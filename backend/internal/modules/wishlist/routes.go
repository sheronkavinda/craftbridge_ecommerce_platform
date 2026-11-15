package wishlist

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RegisterRoutes registers authenticated wishlist endpoints.
func RegisterRoutes(rg *gin.RouterGroup, db *pgxpool.Pool) {
	repo := NewRepository(db)
	service := NewService(repo)
	handler := NewHandler(service)

	// All wishlist endpoints require authentication.
	wishlistGroup := rg.Group("/me/wishlist")
	wishlistGroup.Use(platformhttp.RequireAuthenticatedUser())

	wishlistGroup.GET("", handler.List)
	wishlistGroup.POST("", handler.Add)
	wishlistGroup.DELETE("/:productId", handler.Remove)
}
