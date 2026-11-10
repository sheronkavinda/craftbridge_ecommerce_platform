package admin

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the admin endpoints onto the /api/v1/admin group wrapped with strict admin RBAC.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	adminGroup := rg.Group("/admin")
	adminGroup.Use(platformhttp.RequireAdminRole())
	{
		adminGroup.GET("/users", handler.ListUsers)
		adminGroup.PATCH("/users/:id/status", handler.UpdateUserStatus)
		adminGroup.PUT("/users/:id/status", handler.UpdateUserStatus)

		adminGroup.GET("/sellers", handler.ListSellerApplications)
		adminGroup.GET("/sellers/applications", handler.ListSellerApplications)
		adminGroup.PATCH("/sellers/:id/status", handler.UpdateSellerStatus)
		adminGroup.PUT("/sellers/:id/status", handler.UpdateSellerStatus)

		adminGroup.GET("/refunds", handler.ListRefunds)
		adminGroup.PATCH("/refunds/:id/status", handler.UpdateRefundStatus)
		adminGroup.PUT("/refunds/:id/status", handler.UpdateRefundStatus)

		adminGroup.GET("/categories", handler.ListCategories)
		adminGroup.POST("/categories", handler.CreateCategory)
		adminGroup.PUT("/categories/:id", handler.UpdateCategory)
		adminGroup.DELETE("/categories/:id", handler.DeleteCategory)
	}
}
