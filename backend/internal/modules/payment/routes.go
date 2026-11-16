package payment

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the payment, buyer payment history, admin reconciliation, and webhook endpoints onto the /api/v1 router group.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	buyerOrders := rg.Group("/orders")
	buyerOrders.Use(platformhttp.RequireBuyerRole())
	{
		buyerOrders.POST("/:id/payments", handler.InitiatePayment)
	}

	buyerMe := rg.Group("/me")
	buyerMe.Use(platformhttp.RequireBuyerRole())
	{
		buyerMe.GET("/payments", handler.ListBuyerPayments)
		buyerMe.GET("/payments/:id", handler.GetBuyerPayment)
	}

	adminPayments := rg.Group("/admin/payments")
	adminPayments.Use(platformhttp.RequireAdminRole())
	{
		adminPayments.POST("/reconcile", handler.ReconcilePayments)
	}

	webhooks := rg.Group("/webhooks")
	{
		webhooks.POST("/payments/:provider", handler.HandleWebhook)
	}
}
