package receipt

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes HTTP endpoints for viewing and downloading owner-restricted digital payment receipts.
type Handler struct {
	service *Service
}

// NewHandler creates a new receipt HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetOrderReceipt handles GET /api/v1/me/orders/:id/receipt.
func (h *Handler) GetOrderReceipt(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	view, err := h.service.GetOrderReceipt(ctx, actor, orderID)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to retrieve receipt")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": view})
}

// DownloadOrderReceipt handles GET /api/v1/me/orders/:id/receipt/download.
func (h *Handler) DownloadOrderReceipt(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	actor, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	view, err := h.service.GetOrderReceipt(ctx, actor, orderID)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to download receipt")
		return
	}

	htmlBytes, err := h.service.RenderReceiptHTML(view)
	if err != nil {
		log.Printf("[req_id=%s] failed to render receipt HTML: %v", reqID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to render receipt document"})
		return
	}

	filename := fmt.Sprintf("receipt-%s.html", view.ReceiptNumber)
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Data(http.StatusOK, "text/html; charset=utf-8", htmlBytes)
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
	case errors.Is(err, ErrInvalidReceiptInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
	case errors.Is(err, ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "order not found"})
	case errors.Is(err, ErrNoVerifiedPayment):
		c.JSON(http.StatusConflict, gin.H{"error": "no verified payment for this order"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallbackMsg})
	}
}
