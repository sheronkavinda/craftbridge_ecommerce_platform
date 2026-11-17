package refund

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the buyer refund request, buyer refund history, and admin refund approval routes onto /api/v1.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	buyerOrders := rg.Group("/orders")
	buyerOrders.Use(platformhttp.RequireBuyerRole())
	{
		buyerOrders.POST("/:id/refund-requests", handler.RequestRefund)
	}

	buyerMe := rg.Group("/me")
	buyerMe.Use(platformhttp.RequireBuyerRole())
	{
		buyerMe.GET("/refunds", handler.ListBuyerRefunds)
		buyerMe.GET("/refunds/:id", handler.GetBuyerRefund)
	}

	adminRefunds := rg.Group("/admin/refunds")
	adminRefunds.Use(platformhttp.RequireAdminRole())
	{
		adminRefunds.POST("/:id/approve", handler.ApproveRefund)
	}
}
