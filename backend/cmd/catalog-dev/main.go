
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"craftbridge/backend/internal/modules/catalog"
	"craftbridge/backend/internal/modules/seller"
)

// Local development authentication only.
// Never register this middleware in production.
func localAuth(c *gin.Context) {
	rawID := c.GetHeader("X-Dev-User-ID")

	userID, err := strconv.ParseInt(rawID, 10, 64)

	if err != nil || userID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": "Local development user ID required",
		})
		return
	}

	c.Set("user_id", userID)
	c.Next()
}

// Local administrator authorization.
// For isolated development only.
func localAdmin(c *gin.Context) {
	if c.GetHeader("X-Dev-Role") != "admin" {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "Admin role required",
		})
		return
	}

	c.Next()
}

// Allow the local Angular development server.
func localCORS(c *gin.Context) {
	origin := c.GetHeader("Origin")

	if origin == "http://localhost:4200" ||
		origin == "http://127.0.0.1:4200" {

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
		c.Header(
			"Access-Control-Allow-Methods",
			"GET, POST, PATCH, DELETE, OPTIONS",
		)
		c.Header(
			"Access-Control-Allow-Headers",
			"Content-Type, X-Dev-User-ID, X-Dev-Role",
		)
	}

	if c.Request.Method == http.MethodOptions {
		c.AbortWithStatus(http.StatusNoContent)
		return
	}

	c.Next()
}

func main() {
	// Disable accidental deployment of local demo authentication.
	if os.Getenv("APP_ENV") != "local" ||
		os.Getenv("ENABLE_LOCAL_DEV_AUTH") != "1" {
		log.Fatal("catalog-dev requires explicitly enabled local mode")
	}

	databaseURL := os.Getenv("DATABASE_URL")

	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	// Connect PostgreSQL.
	ctx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatal("PostgreSQL pool error:", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatal("PostgreSQL connection error:", err)
	}

	// Initialize seller module.
	sellerRepository := seller.NewRepository(db)
	sellerService := seller.NewService(sellerRepository)
	sellerHandler := seller.NewHandler(sellerService)

	// Initialize catalog module.
	catalogRepository := catalog.NewRepository(db)
	catalogService := catalog.NewService(catalogRepository)
	catalogHandler := catalog.NewHandler(catalogService)

	// Local image storage.
	imageStore, err := catalog.NewLocalImageStore(
		"uploads/catalog",
	)
	if err != nil {
		log.Fatal("Image storage initialization error:", err)
	}

	imageHandler := catalog.NewImageHandler(
		catalogRepository, imageStore,
	)

	// Initialize Gin.
	router := gin.Default()

	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}

	router.Use(localCORS)

	// Public uploaded images for the local demo.
	router.StaticFS(
		"/media/catalog",
		gin.Dir("uploads/catalog", false),
	)

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"service": "craftbridge-catalog-dev",
		})
	})

	router.GET("/health/db", func(c *gin.Context) {
		pingCtx, done := context.WithTimeout(
			c.Request.Context(), 3*time.Second,
		)
		defer done()

		if err := db.Ping(pingCtx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "database unavailable",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "database connected",
		})
	})

	// API version 1.
	api := router.Group("/api/v1")

	// Register seller and shop APIs.
	seller.RegisterRoutes(
		api, sellerHandler, localAuth,
	)
	seller.RegisterShopMediaRoutes(
    api,
    seller.NewShopMediaHandler(sellerRepository, imageStore),
    localAuth,
)

	// Local-only admin decision endpoint.
	seller.RegisterAdminRoutes(
		api, sellerHandler, localAuth, localAdmin,
	)

	// Register product catalog APIs.
	catalog.RegisterRoutes(
		api, catalogHandler, localAuth,
	)

	// Register inventory management APIs.
	catalog.RegisterInventoryRoutes(
		api, catalogHandler, localAuth,
	)

	// Register image APIs.
	catalog.RegisterImageRoutes(
		api, imageHandler, localAuth,
	)

	log.Println("CraftBridge Seller & Catalog API initialized")
	log.Println("Listening on http://127.0.0.1:8081")

	if err := router.Run("127.0.0.1:8081"); err != nil {
		log.Fatal(err)
	}
}

