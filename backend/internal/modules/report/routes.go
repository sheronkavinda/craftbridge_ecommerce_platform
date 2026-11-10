package report

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the report module endpoints under /api/v1/reports wrapped with strict admin RBAC.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	reportGroup := rg.Group("/reports")
	reportGroup.Use(platformhttp.RequireAdminRole())
	{
		reportGroup.GET("/revenue", handler.GetRevenueSummary)
		reportGroup.GET("/revenue/daily", handler.GetDailyRevenue)
		reportGroup.GET("/sellers", handler.GetSellerRevenue)
		reportGroup.GET("/seller-revenue", handler.GetSellerRevenue)
	}
}
