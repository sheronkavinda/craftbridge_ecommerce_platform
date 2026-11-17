package refund

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes HTTP endpoints for buyer refund requests, buyer refund views, and admin refund approvals.
type Handler struct {
	service *Service
}

// NewHandler creates a new refund HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RequestRefund handles POST /api/v1/orders/:id/refund-requests.
func (h *Handler) RequestRefund(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	var input CreateRefundRequestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid refund request payload"})
		return
	}
	input.IdempotencyKey = c.GetHeader("Idempotency-Key")

	ref, created, err := h.service.RequestRefund(ctx, actor, orderID, input)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to create refund request")
		return
	}

	if created {
		c.JSON(http.StatusCreated, gin.H{"data": ref})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ref})
}

// ApproveRefund handles POST /api/v1/admin/refunds/:id/approve.
func (h *Handler) ApproveRefund(c *gin.Context) {
	refundID := strings.TrimSpace(c.Param("id"))
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	ref, err := h.service.ApproveRefund(ctx, actor, refundID)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to approve refund")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ref})
}

// ListBuyerRefunds handles GET /api/v1/me/refunds.
func (h *Handler) ListBuyerRefunds(c *gin.Context) {
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	page := 1
	if rawPage := strings.TrimSpace(c.Query("page")); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page parameter"})
			return
		}
		page = parsed
	}

	pageSize := 20
	if rawPageSize := strings.TrimSpace(c.Query("page_size")); rawPageSize != "" {
		parsed, err := strconv.Atoi(rawPageSize)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid page_size parameter"})
			return
		}
		pageSize = parsed
	}

	statusParam := strings.TrimSpace(c.Query("status"))

	views, meta, err := h.service.ListBuyerRefunds(ctx, actor, statusParam, page, pageSize)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to list buyer refunds")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": views,
		"meta": meta,
	})
}

// GetBuyerRefund handles GET /api/v1/me/refunds/:id.
func (h *Handler) GetBuyerRefund(c *gin.Context) {
	refundID := strings.TrimSpace(c.Param("id"))
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	view, err := h.service.GetBuyerRefund(ctx, actor, refundID)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to retrieve refund")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": view})
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
	case errors.Is(err, ErrInvalidRefundInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, ErrRefundNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "refund not found"})
	case errors.Is(err, ErrNoVerifiedPayment):
		c.JSON(http.StatusConflict, gin.H{"error": "order has no verified payment"})
	case errors.Is(err, ErrRefundWindowExpired):
		c.JSON(http.StatusConflict, gin.H{"error": "refund window expired"})
	case errors.Is(err, ErrRefundAlreadyRejected):
		c.JSON(http.StatusConflict, gin.H{"error": "cannot approve a rejected refund"})
	case errors.Is(err, ErrRefundTransitionConflict), errors.Is(err, ErrDuplicateProviderRefundRef):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, ErrAmountExceedsBalance):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "amount exceeds refundable balance"})
	case errors.Is(err, ErrGatewayRefundFailed):
		c.JSON(http.StatusBadGateway, gin.H{"error": "payment provider refund failed"})
	case errors.Is(err, ErrGatewayNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "refund payment provider not configured"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallbackMsg})
	}
}
