package order

import "github.com/gin-gonic/gin"

func RegisterRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("order routes require authentication")
	}

	checkout := api.Group("/checkout")
	checkout.Use(authenticate)
	checkout.POST("", handler.Checkout)

	buyer := api.Group("/me/orders")
	buyer.Use(authenticate)

	buyer.GET("", handler.ListOrders)
	buyer.GET("/:id", handler.GetOrder)
	buyer.POST("/:id/cancel", handler.CancelOrder)
}
