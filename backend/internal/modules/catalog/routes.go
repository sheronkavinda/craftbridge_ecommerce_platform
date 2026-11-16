
package catalog

import "github.com/gin-gonic/gin"

// Public catalog and seller product routes.
func RegisterRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("catalog routes require authentication")
	}

	// Public product discovery
	api.GET("/categories", handler.ListCategories)
	api.GET("/products", handler.ListPublicProducts)
	api.GET("/products/:id", handler.GetPublicProduct)

	// Seller product management
	sellerRoutes := api.Group("/seller")
	sellerRoutes.Use(authenticate)

	sellerRoutes.GET("/products", handler.ListMyProducts)
	sellerRoutes.GET("/products/:id", handler.GetMyProduct)
	sellerRoutes.POST("/products", handler.CreateProduct)
	sellerRoutes.PATCH("/products/:id", handler.UpdateProduct)
	sellerRoutes.DELETE("/products/:id", handler.ArchiveProduct)
}

// RegisterInventoryRoutes is defined in inventory_handler.go.
// RegisterImageRoutes is defined in media_handler.go.
//
// Both are registered from the application entry point.
