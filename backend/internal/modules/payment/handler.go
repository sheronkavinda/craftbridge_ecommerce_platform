package payment

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

const maxWebhookBodyBytes = 64 * 1024

// Handler exposes HTTP endpoints for payment operations and provider webhooks.
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

// HandleWebhook handles POST /api/v1/webhooks/payments/:provider.
// This is a public route authenticated via provider cryptographic signature verification.
func (h *Handler) HandleWebhook(c *gin.Context) {
	providerName := strings.TrimSpace(c.Param("provider"))
	_, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)

	if c.Request.Body == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrMalformedNotification.Error()})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBodyBytes)
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrMalformedNotification.Error()})
		return
	}

	parsedValues, err := url.ParseQuery(string(rawBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrMalformedNotification.Error()})
		return
	}

	form := make(map[string]string, len(parsedValues))
	for k, vals := range parsedValues {
		if len(vals) > 0 {
			form[k] = vals[0]
		}
	}

	headers := make(map[string]string, len(c.Request.Header))
	for k, vals := range c.Request.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}

	result, svcErr := h.service.HandleWebhook(ctx, providerName, NotificationInput{
		Headers: headers,
		Form:    form,
		RawBody: rawBody,
	})
	if svcErr != nil {
		handleWebhookError(c, reqID, svcErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
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

func handleWebhookError(c *gin.Context, reqID string, err error) {
	switch {
	case errors.Is(err, ErrMalformedNotification):
		c.JSON(http.StatusBadRequest, gin.H{"error": ErrMalformedNotification.Error()})
	case errors.Is(err, ErrInvalidSignature):
		c.JSON(http.StatusUnauthorized, gin.H{"error": ErrInvalidSignature.Error()})
	case errors.Is(err, ErrUnknownProvider):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrUnknownProvider.Error()})
	case errors.Is(err, ErrAttemptNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": ErrAttemptNotFound.Error()})
	case errors.Is(err, ErrAmountMismatch):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, ErrProviderNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": ErrProviderNotConfigured.Error()})
	default:
		log.Printf("[req_id=%s] failed to process payment webhook: %v", reqID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process payment webhook"})
	}
}
