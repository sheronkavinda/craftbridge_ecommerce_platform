package audit

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes HTTP endpoints for the audit module.
type Handler struct {
	service *Service
}

// NewHandler creates a new HTTP handler for audit logs.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ListAuditLogs handles GET /api/v1/audit/logs.
func (h *Handler) ListAuditLogs(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		offset = 0
	}

	filter := ListAuditLogsFilter{
		ActorID: strings.TrimSpace(c.Query("actor_id")),
		Action:  strings.TrimSpace(c.Query("action")),
		Outcome: strings.TrimSpace(c.Query("outcome")),
		Limit:   limit,
		Offset:  offset,
	}

	logs, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve audit logs",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": logs,
	})
}

// CreateAuditLog handles POST /api/v1/audit/logs.
// To prevent identity spoofing (IDOR), actor identity and request_id are bound from the verified admin context.
func (h *Handler) CreateAuditLog(c *gin.Context) {
	var input CreateAuditLogInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request payload",
		})
		return
	}

	if actorID, exists := c.Get(platformhttp.ContextKeyActorID); exists {
		if str, ok := actorID.(string); ok && str != "" {
			input.ActorID = str
		}
	}
	if actorName, exists := c.Get(platformhttp.ContextKeyActorName); exists {
		if str, ok := actorName.(string); ok && str != "" {
			input.ActorName = str
		}
	}
	if reqID, exists := c.Get(platformhttp.ContextKeyRequestID); exists {
		if str, ok := reqID.(string); ok && str != "" {
			input.RequestID = str
		}
	}

	entry, err := h.service.LogAction(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidAuditInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create audit log",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": entry,
	})
}
