package receipt

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the owner-restricted buyer receipt view and download routes onto /api/v1.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	buyerMe := rg.Group("/me")
	buyerMe.Use(platformhttp.RequireBuyerRole())
	{
		buyerMe.GET("/orders/:id/receipt", handler.GetOrderReceipt)
		buyerMe.GET("/orders/:id/receipt/download", handler.DownloadOrderReceipt)
	}
}
