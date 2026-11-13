package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"craftbridge/backend/internal/platform/config"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ContextKeyRequestID = "request_id"
	ContextKeyActorID   = "actor_id"
	ContextKeyActorName = "actor_name"
	ContextKeyActorRole = "actor_role"
)

var activeConfig atomic.Pointer[config.Config]

// RouteRegistrar defines a function signature for registering module routes onto the /api/v1 group.
type RouteRegistrar func(rg *gin.RouterGroup)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Sub  string `json:"sub"`
	Name string `json:"name"`
	Role string `json:"role"`
	Exp  int64  `json:"exp"`
}

// NewRouter initializes the Gin engine, standard middleware, health check, Swagger endpoints, and /api/v1 group.
func NewRouter(cfg *config.Config, db *pgxpool.Pool, registrars ...RouteRegistrar) *gin.Engine {
	if cfg != nil {
		activeConfig.Store(cfg)
	}

	if cfg != nil && strings.EqualFold(cfg.AppEnv, "production") {
		gin.SetMode(gin.ReleaseMode)
	}

	var corsOrigins []string
	if cfg != nil {
		corsOrigins = cfg.CORSOrigins
	}

	engine := gin.New()
	engine.Use(gin.Logger())
	engine.Use(gin.Recovery())
	engine.Use(SecurityHeadersMiddleware())
	engine.Use(RequestIDMiddleware())
	engine.Use(CORSMiddleware(corsOrigins))

	// Root health check endpoint
	engine.GET("/health", healthHandler(db))

	// Swagger / OpenAPI documentation endpoints
	registerSwaggerRoutes(engine)

	// API v1 route group
	v1 := engine.Group("/api/v1")
	{
		v1.GET("/health", healthHandler(db))
		for _, register := range registrars {
			if register != nil {
				register(v1)
			}
		}
	}

	return engine
}

// SecurityHeadersMiddleware sets defensive HTTP headers to mitigate XSS, clickjacking, and MIME sniffing.
func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("X-Content-Type-Options", "nosniff")
		c.Writer.Header().Set("X-Frame-Options", "DENY")
		c.Writer.Header().Set("X-XSS-Protection", "1; mode=block")
		c.Writer.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

// RequireAdminRole enforces strict Role-Based Access Control (RBAC).
// It blocks any caller that is not authenticated with the 'admin' role.
func RequireAdminRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		actorID, actorName, actorRole, err := extractVerifiedIdentity(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized: valid authentication credentials required",
			})
			return
		}

		if !strings.EqualFold(actorRole, "admin") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: strict admin role required",
			})
			return
		}

		if actorID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized: missing actor identity",
			})
			return
		}

		c.Set(ContextKeyActorID, actorID)
		c.Set(ContextKeyActorName, actorName)
		c.Set(ContextKeyActorRole, "admin")
		c.Next()
	}
}

func extractVerifiedIdentity(c *gin.Context) (actorID, actorName, actorRole string, err error) {
	cfg := activeConfig.Load()
	appEnv := "development"
	jwtSecret := ""
	if cfg != nil {
		appEnv = strings.TrimSpace(cfg.AppEnv)
		jwtSecret = strings.TrimSpace(cfg.JWTSecret)
	} else {
		if env := strings.TrimSpace(os.Getenv("APP_ENV")); env != "" {
			appEnv = env
		}
		jwtSecret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if jwtSecret == "" {
		jwtSecret = "your-super-secret-jwt-key-change-in-production"
	}

	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		token := strings.TrimSpace(authHeader[7:])
		claims, verifyErr := verifyHS256JWT(token, jwtSecret)
		if verifyErr != nil {
			return "", "", "", verifyErr
		}
		name := strings.TrimSpace(claims.Name)
		if name == "" {
			name = "Admin"
		}
		return strings.TrimSpace(claims.Sub), name, strings.TrimSpace(claims.Role), nil
	}

	// Strictly restrict header-based identity fallback to development environments
	if !strings.EqualFold(appEnv, "development") {
		return "", "", "", errors.New("bearer token authentication is required")
	}

	roleHeader := strings.TrimSpace(c.GetHeader("X-Actor-Role"))
	if roleHeader == "" {
		roleHeader = strings.TrimSpace(c.GetHeader("X-User-Role"))
	}
	idHeader := strings.TrimSpace(c.GetHeader("X-Actor-ID"))
	if idHeader == "" {
		idHeader = strings.TrimSpace(c.GetHeader("X-User-ID"))
	}
	nameHeader := strings.TrimSpace(c.GetHeader("X-Actor-Name"))
	if nameHeader == "" {
		nameHeader = "Admin"
	}

	if roleHeader == "" || idHeader == "" {
		return "", "", "", errors.New("missing authentication credentials")
	}

	return idHeader, nameHeader, roleHeader, nil
}

func verifyHS256JWT(tokenStr, secret string) (*jwtClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid token header encoding")
	}
	var hdr jwtHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil || !strings.EqualFold(hdr.Alg, "HS256") {
		return nil, errors.New("unsupported or invalid token algorithm")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errors.New("invalid token signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid token payload encoding")
	}

	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("invalid token claims")
	}

	if claims.Exp <= time.Now().Unix() {
		return nil, errors.New("token expired or missing expiration")
	}

	return &claims, nil
}

func healthHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "up"
		if db != nil {
			if err := db.Ping(c.Request.Context()); err != nil {
				dbStatus = "down"
			}
		} else {
			dbStatus = "unconfigured"
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"database":  dbStatus,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// RequestIDMiddleware injects or propagates a validated X-Request-ID header on every HTTP request.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if reqID == "" || len(reqID) > 64 {
			reqID = generateRequestID()
		}
		c.Set(ContextKeyRequestID, reqID)
		c.Writer.Header().Set("X-Request-ID", reqID)
		c.Next()
	}
}

// CORSMiddleware configures Cross-Origin Resource Sharing for the specified allowed origins.
func CORSMiddleware(allowedOrigins []string) gin.HandlerFunc {
	originMap := make(map[string]struct{}, len(allowedOrigins))
	allowAll := false
	for _, origin := range allowedOrigins {
		if origin == "*" {
			allowAll = true
		}
		originMap[origin] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowAll {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else if _, ok := originMap[origin]; ok {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Vary", "Origin")
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization, X-Request-ID, X-Actor-ID, X-Actor-Name, X-Actor-Role")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func registerSwaggerRoutes(engine *gin.Engine) {
	engine.GET("/swagger/doc.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(openAPISpecJSON))
	})

	engine.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	engine.GET("/swagger/index.html", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerUIHTML))
	})
}

func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "req-default"
	}
	return "req-" + hex.EncodeToString(b)
}

const openAPISpecJSON = `{
  "openapi": "3.0.3",
  "info": {
    "title": "CraftBridge E-Commerce Platform API",
    "description": "Backend REST API for CraftBridge (Member 06 - Admin, Reporting & Integration Module). In development mode, supply X-Actor-Role: admin and X-Actor-ID headers or a Bearer JWT token.",
    "version": "1.0.0"
  },
  "servers": [
    {
      "url": "/",
      "description": "CraftBridge Local API Server"
    }
  ],
  "tags": [
    { "name": "Health", "description": "Service and database health checks" },
    { "name": "Admin - Users", "description": "Administrative user management and status control" },
    { "name": "Admin - Sellers", "description": "Seller onboarding applications and approval triggers" },
    { "name": "Admin - Categories", "description": "Product catalog category management" },
    { "name": "Admin - Refunds", "description": "Administrative refund review and status transitions" },
    { "name": "Admin - Audit Logs", "description": "Immutable append-only administrative audit trail" },
    { "name": "Reports", "description": "Platform revenue and seller sales aggregation reports" }
  ],
  "security": [
    { "BearerAuth": [] },
    { "ActorRoleHeader": [], "ActorIDHeader": [] }
  ],
  "components": {
    "securitySchemes": {
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT",
        "description": "HS256 JWT token with role='admin'"
      },
      "ActorRoleHeader": {
        "type": "apiKey",
        "in": "header",
        "name": "X-Actor-Role",
        "description": "Development mode role header (set to 'admin')"
      },
      "ActorIDHeader": {
        "type": "apiKey",
        "in": "header",
        "name": "X-Actor-ID",
        "description": "Development mode actor identifier (e.g. 'admin-001')"
      }
    },
    "parameters": {
      "XActorRole": {
        "name": "X-Actor-Role",
        "in": "header",
        "required": false,
        "description": "Role of the caller in development mode (must be 'admin')",
        "schema": { "type": "string", "default": "admin" }
      },
      "XActorID": {
        "name": "X-Actor-ID",
        "in": "header",
        "required": false,
        "description": "Unique identifier of the acting administrator",
        "schema": { "type": "string", "default": "admin-001" }
      },
      "XActorName": {
        "name": "X-Actor-Name",
        "in": "header",
        "required": false,
        "description": "Display name of the acting administrator",
        "schema": { "type": "string", "default": "System Admin" }
      }
    },
    "schemas": {
      "UserSummary": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "example": "usr-101" },
          "name": { "type": "string", "example": "Jane Doe" },
          "email": { "type": "string", "example": "jane@example.com" },
          "role": { "type": "string", "example": "buyer" },
          "status": { "type": "string", "enum": ["ACTIVE", "SUSPENDED", "BANNED"], "example": "ACTIVE" },
          "created_at": { "type": "string", "format": "date-time" }
        }
      },
      "UpdateUserStatusInput": {
        "type": "object",
        "required": ["status"],
        "properties": {
          "status": { "type": "string", "enum": ["ACTIVE", "SUSPENDED", "BANNED"], "example": "SUSPENDED" },
          "reason": { "type": "string", "example": "Policy violation review" }
        }
      },
      "SellerApplication": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "example": "sel-201" },
          "user_id": { "type": "string", "example": "usr-201" },
          "store_name": { "type": "string", "example": "Artisan Clay Studio" },
          "status": { "type": "string", "enum": ["PENDING", "APPROVED", "REJECTED", "SUSPENDED"], "example": "PENDING" },
          "notes": { "type": "string", "example": "Verified business registration" },
          "submitted_at": { "type": "string", "format": "date-time" },
          "reviewed_at": { "type": "string", "format": "date-time" }
        }
      },
      "UpdateSellerStatusInput": {
        "type": "object",
        "required": ["status"],
        "properties": {
          "status": { "type": "string", "enum": ["PENDING", "APPROVED", "REJECTED", "SUSPENDED"], "example": "APPROVED" },
          "notes": { "type": "string", "example": "Approved after portfolio verification" }
        }
      },
      "Category": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "example": "cat-01" },
          "name": { "type": "string", "example": "Handcrafted Ceramics" },
          "slug": { "type": "string", "example": "handcrafted-ceramics" },
          "description": { "type": "string", "example": "Artisanal pottery and stoneware" },
          "is_active": { "type": "boolean", "example": true },
          "created_at": { "type": "string", "format": "date-time" }
        }
      },
      "CategoryInput": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": { "type": "string", "example": "Handcrafted Ceramics" },
          "slug": { "type": "string", "example": "handcrafted-ceramics" },
          "description": { "type": "string", "example": "Artisanal pottery and stoneware" },
          "is_active": { "type": "boolean", "example": true }
        }
      },
      "RefundRecord": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "example": "ref-301" },
          "order_id": { "type": "string", "example": "ord-901" },
          "amount": { "type": "number", "format": "double", "example": 45.50 },
          "status": { "type": "string", "enum": ["PENDING", "APPROVED", "REJECTED", "PROCESSED"], "example": "APPROVED" },
          "reason": { "type": "string", "example": "Damaged in transit" },
          "created_at": { "type": "string", "format": "date-time" },
          "reviewed_at": { "type": "string", "format": "date-time" }
        }
      },
      "UpdateRefundStatusInput": {
        "type": "object",
        "required": ["status"],
        "properties": {
          "status": { "type": "string", "enum": ["APPROVED", "REJECTED", "PROCESSED"], "example": "APPROVED" },
          "reason": { "type": "string", "example": "Verified buyer photo evidence" }
        }
      },
      "AuditLog": {
        "type": "object",
        "properties": {
          "id": { "type": "integer", "format": "int64", "example": 1 },
          "actor_id": { "type": "string", "example": "admin-001" },
          "actor_name": { "type": "string", "example": "System Admin" },
          "action": { "type": "string", "example": "SELLER_STATUS_UPDATE" },
          "target": { "type": "string", "example": "seller:sel-201" },
          "outcome": { "type": "string", "enum": ["SUCCESS", "FAILURE"], "example": "SUCCESS" },
          "request_id": { "type": "string", "example": "req-a1b2c3d4" },
          "details": { "type": "string", "example": "seller status transitioned to APPROVED" },
          "created_at": { "type": "string", "format": "date-time" }
        }
      },
      "CreateAuditLogInput": {
        "type": "object",
        "required": ["action", "target"],
        "properties": {
          "action": { "type": "string", "example": "MANUAL_AUDIT_NOTE" },
          "target": { "type": "string", "example": "order:ord-901" },
          "outcome": { "type": "string", "enum": ["SUCCESS", "FAILURE"], "example": "SUCCESS" },
          "details": { "type": "string", "example": "Reviewed order dispute documentation" }
        }
      },
      "RevenueSummary": {
        "type": "object",
        "properties": {
          "gross_verified_payments": { "type": "number", "format": "double", "example": 12500.00 },
          "total_refunds": { "type": "number", "format": "double", "example": 450.00 },
          "net_revenue": { "type": "number", "format": "double", "example": 12050.00 },
          "verified_payment_count": { "type": "integer", "format": "int64", "example": 140 },
          "refund_count": { "type": "integer", "format": "int64", "example": 5 },
          "start_date": { "type": "string", "example": "2026-01-01" },
          "end_date": { "type": "string", "example": "2026-12-31" },
          "generated_at": { "type": "string", "format": "date-time" }
        }
      },
      "SellerSalesReport": {
        "type": "object",
        "properties": {
          "seller_id": { "type": "string", "example": "sel-201" },
          "store_name": { "type": "string", "example": "Artisan Clay Studio" },
          "order_count": { "type": "integer", "format": "int64", "example": 42 },
          "verified_payments": { "type": "number", "format": "double", "example": 4200.00 },
          "refunds": { "type": "number", "format": "double", "example": 120.00 },
          "net_revenue": { "type": "number", "format": "double", "example": 4080.00 }
        }
      }
    }
  },
  "paths": {
    "/api/v1/health": {
      "get": {
        "tags": ["Health"],
        "summary": "API v1 health and database connectivity check",
        "security": [],
        "responses": {
          "200": { "description": "Service and database health status" }
        }
      }
    },
    "/api/v1/admin/users": {
      "get": {
        "tags": ["Admin - Users"],
        "summary": "List platform users",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "role", "in": "query", "schema": { "type": "string" }, "description": "Filter by user role (e.g. buyer, seller, admin)" },
          { "name": "status", "in": "query", "schema": { "type": "string", "enum": ["ACTIVE", "SUSPENDED", "BANNED"] }, "description": "Filter by account status" },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "offset", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": {
          "200": { "description": "List of users" },
          "401": { "description": "Unauthorized" },
          "403": { "description": "Forbidden (admin role required)" }
        }
      }
    },
    "/api/v1/admin/users/{id}/status": {
      "patch": {
        "tags": ["Admin - Users"],
        "summary": "Update user account status (ACTIVE, SUSPENDED, BANNED)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" }, "description": "Target user ID" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateUserStatusInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Updated user record" },
          "400": { "description": "Invalid status input" },
          "403": { "description": "Forbidden / Self-lockout prevented" },
          "404": { "description": "User not found" }
        }
      },
      "put": {
        "tags": ["Admin - Users"],
        "summary": "Update user account status (PUT alias)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" }, "description": "Target user ID" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateUserStatusInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Updated user record" }
        }
      }
    },
    "/api/v1/admin/sellers": {
      "get": {
        "tags": ["Admin - Sellers"],
        "summary": "List seller onboarding applications",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "status", "in": "query", "schema": { "type": "string", "enum": ["PENDING", "APPROVED", "REJECTED", "SUSPENDED"] } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "offset", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": {
          "200": { "description": "List of seller applications" }
        }
      }
    },
    "/api/v1/admin/sellers/applications": {
      "get": {
        "tags": ["Admin - Sellers"],
        "summary": "List seller applications filtered by review status",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "status", "in": "query", "schema": { "type": "string", "enum": ["PENDING", "APPROVED", "REJECTED", "SUSPENDED"] } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "offset", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": {
          "200": { "description": "List of seller applications" }
        }
      }
    },
    "/api/v1/admin/sellers/{id}/status": {
      "put": {
        "tags": ["Admin - Sellers"],
        "summary": "Review and update seller application status (APPROVED, REJECTED, SUSPENDED)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" }, "description": "Seller ID" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateSellerStatusInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Seller status updated and audit logged" },
          "400": { "description": "Invalid status" },
          "404": { "description": "Seller not found" }
        }
      },
      "patch": {
        "tags": ["Admin - Sellers"],
        "summary": "Review and update seller application status (PATCH)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" }, "description": "Seller ID" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateSellerStatusInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Seller status updated and audit logged" }
        }
      }
    },
    "/api/v1/admin/categories": {
      "get": {
        "tags": ["Admin - Categories"],
        "summary": "List all product catalog categories",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" }
        ],
        "responses": {
          "200": { "description": "Array of categories" }
        }
      },
      "post": {
        "tags": ["Admin - Categories"],
        "summary": "Create a new product category",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/CategoryInput" }
            }
          }
        },
        "responses": {
          "201": { "description": "Category created and audit logged" },
          "400": { "description": "Validation error" },
          "409": { "description": "A category with this slug already exists" }
        }
      }
    },
    "/api/v1/admin/categories/{id}": {
      "put": {
        "tags": ["Admin - Categories"],
        "summary": "Update an existing product category",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/CategoryInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Category updated" },
          "404": { "description": "Category not found" },
          "409": { "description": "A category with this slug already exists" }
        }
      },
      "delete": {
        "tags": ["Admin - Categories"],
        "summary": "Delete a product category by ID",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": { "description": "Category deleted" },
          "404": { "description": "Category not found" }
        }
      }
    },
    "/api/v1/admin/refunds": {
      "get": {
        "tags": ["Admin - Refunds"],
        "summary": "List refund requests for administrative review",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "status", "in": "query", "schema": { "type": "string", "enum": ["PENDING", "APPROVED", "REJECTED", "PROCESSED"] } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "offset", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": {
          "200": { "description": "List of refund records" }
        }
      }
    },
    "/api/v1/admin/refunds/{id}/status": {
      "put": {
        "tags": ["Admin - Refunds"],
        "summary": "Approve or reject a refund request (PUT)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" }, "description": "Refund ID" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateRefundStatusInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Refund status updated and audit logged" },
          "404": { "description": "Refund not found" }
        }
      },
      "patch": {
        "tags": ["Admin - Refunds"],
        "summary": "Approve or reject a refund request (PATCH)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" }, "description": "Refund ID" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/UpdateRefundStatusInput" }
            }
          }
        },
        "responses": {
          "200": { "description": "Refund status updated and audit logged" }
        }
      }
    },
    "/api/v1/admin/audit-logs": {
      "get": {
        "tags": ["Admin - Audit Logs"],
        "summary": "Retrieve immutable administrative audit logs",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "actor_id", "in": "query", "schema": { "type": "string" } },
          { "name": "action", "in": "query", "schema": { "type": "string" } },
          { "name": "outcome", "in": "query", "schema": { "type": "string", "enum": ["SUCCESS", "FAILURE"] } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "offset", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": {
          "200": { "description": "List of audit logs ordered by created_at DESC" }
        }
      }
    },
    "/api/v1/audit/logs": {
      "get": {
        "tags": ["Admin - Audit Logs"],
        "summary": "Retrieve append-only audit logs",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "actor_id", "in": "query", "schema": { "type": "string" } },
          { "name": "action", "in": "query", "schema": { "type": "string" } },
          { "name": "outcome", "in": "query", "schema": { "type": "string", "enum": ["SUCCESS", "FAILURE"] } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "offset", "in": "query", "schema": { "type": "integer", "default": 0 } }
        ],
        "responses": {
          "200": { "description": "List of audit logs" }
        }
      },
      "post": {
        "tags": ["Admin - Audit Logs"],
        "summary": "Record a new append-only audit log entry",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": { "$ref": "#/components/schemas/CreateAuditLogInput" }
            }
          }
        },
        "responses": {
          "201": { "description": "Created audit log entry" }
        }
      }
    },
    "/api/v1/reports/seller-revenue": {
      "get": {
        "tags": ["Reports"],
        "summary": "Aggregated seller revenue report (verified payments minus approved refunds)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "start_date", "in": "query", "schema": { "type": "string", "example": "2026-01-01" }, "description": "Start date (YYYY-MM-DD)" },
          { "name": "end_date", "in": "query", "schema": { "type": "string", "example": "2026-12-31" }, "description": "End date (YYYY-MM-DD)" },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } }
        ],
        "responses": {
          "200": { "description": "Seller revenue breakdown" }
        }
      }
    },
    "/api/v1/reports/revenue": {
      "get": {
        "tags": ["Reports"],
        "summary": "Platform net revenue summary (gross verified payments minus refunds)",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "start_date", "in": "query", "schema": { "type": "string", "example": "2026-01-01" } },
          { "name": "end_date", "in": "query", "schema": { "type": "string", "example": "2026-12-31" } }
        ],
        "responses": {
          "200": { "description": "Net revenue summary" }
        }
      }
    },
    "/api/v1/reports/revenue/daily": {
      "get": {
        "tags": ["Reports"],
        "summary": "Daily time-series breakdown of verified payments, refunds, and net revenue",
        "parameters": [
          { "$ref": "#/components/parameters/XActorRole" },
          { "$ref": "#/components/parameters/XActorID" },
          { "$ref": "#/components/parameters/XActorName" },
          { "name": "start_date", "in": "query", "schema": { "type": "string", "example": "2026-01-01" } },
          { "name": "end_date", "in": "query", "schema": { "type": "string", "example": "2026-12-31" } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 30 } }
        ],
        "responses": {
          "200": { "description": "Daily revenue breakdown" }
        }
      }
    }
  }
}`

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <title>CraftBridge API - Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '/swagger/doc.json',
        dom_id: '#swagger-ui',
        presets: [SwaggerUIBundle.presets.apis],
        layout: 'BaseLayout'
      });
    };
  </script>
</body>
</html>`
