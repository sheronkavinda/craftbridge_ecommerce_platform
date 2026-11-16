package payment

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the payment and webhook endpoints onto the /api/v1 router group.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	buyerOrders := rg.Group("/orders")
	buyerOrders.Use(platformhttp.RequireBuyerRole())
	{
		buyerOrders.POST("/:id/payments", handler.InitiatePayment)
	}

	webhooks := rg.Group("/webhooks")
	{
		webhooks.POST("/payments/:provider", handler.HandleWebhook)
	}
}
