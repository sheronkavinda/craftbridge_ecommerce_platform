package audit

import (
	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the audit module endpoints under /api/v1/audit and /api/v1/admin/audit-logs wrapped with strict admin RBAC.
func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	auditGroup := rg.Group("/audit")
	auditGroup.Use(platformhttp.RequireAdminRole())
	{
		auditGroup.GET("/logs", handler.ListAuditLogs)
		auditGroup.POST("/logs", handler.CreateAuditLog)
	}

	adminAuditGroup := rg.Group("/admin")
	adminAuditGroup.Use(platformhttp.RequireAdminRole())
	{
		adminAuditGroup.GET("/audit-logs", handler.ListAuditLogs)
	}
}
