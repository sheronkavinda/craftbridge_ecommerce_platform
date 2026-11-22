package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"craftbridge/backend/internal/platform/config"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCORSMiddleware_AllowsIdempotencyKeyHeader(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		AppEnv:      "development",
		CORSOrigins: []string{"http://localhost:4200"},
	}
	router := NewRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/orders/order-005/refund-requests", nil)
	req.Header.Set("Origin", "http://localhost:4200")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Idempotency-Key")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 204, got %d", rec.Code)
	}
	if allowOrigin := rec.Header().Get("Access-Control-Allow-Origin"); allowOrigin != "http://localhost:4200" {
		t.Errorf("got Access-Control-Allow-Origin %q, want http://localhost:4200", allowOrigin)
	}
	if allowHeaders := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(allowHeaders, "Idempotency-Key") {
		t.Errorf("expected Access-Control-Allow-Headers to include Idempotency-Key, got %q", allowHeaders)
	}
	if strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), "X-Actor-ID") {
		t.Error("CORS must not advertise development actor headers")
	}
}

func TestOpenAPISpecIsValidAndDoesNotAdvertiseActorHeaders(t *testing.T) {
	if !json.Valid([]byte(openAPISpecJSON)) {
		t.Fatal("OpenAPI specification is not valid JSON")
	}
	if strings.Contains(openAPISpecJSON, "X-Actor-") || strings.Contains(openAPISpecJSON, "XActor") {
		t.Fatal("OpenAPI specification must not advertise development actor headers")
	}
}

func TestRequireBuyerRoleAcceptsBuyerRoleInMultiRoleSession(t *testing.T) {
	const secret = "member01-router-test-secret"
	cfg := &config.Config{AppEnv: "test", JWTSecret: secret}
	router := NewRouter(cfg, nil, func(v1 *gin.RouterGroup) {
		v1.GET("/buyer-only", RequireBuyerRole(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/buyer-only", nil)
	request.Header.Set("Authorization", "Bearer "+multiRoleTestJWT(secret, 42))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected buyer role from multi-role session to be accepted, got %d", response.Code)
	}
}

func TestRequireAdminRoleRejectsDevelopmentActorHeadersByDefault(t *testing.T) {
	t.Setenv("ENABLE_LOCAL_DEV_AUTH", "")
	cfg := &config.Config{AppEnv: "development", JWTSecret: "router-test-secret"}
	router := NewRouter(cfg, nil, func(v1 *gin.RouterGroup) {
		v1.GET("/protected", RequireAdminRole(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.Header.Set("X-Actor-ID", "admin-001")
	request.Header.Set("X-Actor-Role", "admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected actor headers to be rejected, got %d", response.Code)
	}
}

func TestRequireAdminRoleAllowsExplicitLocalDevelopmentOptIn(t *testing.T) {
	t.Setenv("ENABLE_LOCAL_DEV_AUTH", "1")
	cfg := &config.Config{AppEnv: "development", JWTSecret: "router-test-secret"}
	router := NewRouter(cfg, nil, func(v1 *gin.RouterGroup) {
		v1.GET("/protected", RequireAdminRole(), func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.Header.Set("X-Actor-ID", "admin-001")
	request.Header.Set("X-Actor-Role", "admin")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected explicitly enabled local actor headers to work, got %d", response.Code)
	}
}

func multiRoleTestJWT(secret string, userID int64) string {
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"sub":   strconv.FormatInt(userID, 10),
		"name":  "Seller and buyer",
		"role":  "seller",
		"roles": []string{"buyer", "seller"},
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
