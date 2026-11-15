package payment

import (
	"errors"
	"log"
	"net/http"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes HTTP endpoints for payment operations.
type Handler struct {
	service *Service
}

// NewHandler creates a new payment HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// InitiatePayment handles POST /api/v1/orders/:id/payments.
// Any amount or currency supplied in the request body is intentionally ignored;
// payment amounts and currencies are always sourced from the stored order record.
func (h *Handler) InitiatePayment(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	result, err := h.service.InitiatePayment(ctx, actor, orderID)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to initiate payment")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": result})
}

func extractVerifiedActor(c *gin.Context) (Actor, string) {
	var actor Actor
	var reqID string

	if val, exists := c.Get(platformhttp.ContextKeyActorID); exists {
		if str, ok := val.(string); ok {
			actor.ID = str
		}
	}
	if val, exists := c.Get(platformhttp.ContextKeyActorName); exists {
		if str, ok := val.(string); ok {
			actor.Name = str
		}
	}
	if val, exists := c.Get(platformhttp.ContextKeyActorRole); exists {
		if str, ok := val.(string); ok {
			actor.Role = str
		}
	}
	if val, exists := c.Get(platformhttp.ContextKeyRequestID); exists {
		if str, ok := val.(string); ok {
			reqID = str
		}
	}

	return actor, reqID
}

func handleServiceError(c *gin.Context, reqID string, err error, fallbackMsg string) {
	log.Printf("[req_id=%s] %s: %v", reqID, fallbackMsg, err)
	switch {
	case errors.Is(err, ErrInvalidPaymentInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrOrderNotFound), errors.Is(err, ErrAttemptNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, ErrOrderNotPayable):
		c.JSON(http.StatusConflict, gin.H{"error": "order is not pending payment"})
	case errors.Is(err, ErrProviderNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payment provider not configured"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallbackMsg})
	}
}
