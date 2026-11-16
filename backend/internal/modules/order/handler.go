
package order

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func authenticatedBuyer(c *gin.Context) (int64, bool) {
	value, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}

	switch id := value.(type) {
	case int64:
		return id, id > 0
	case int:
		return int64(id), id > 0
	}

	return 0, false
}

func orderIDFromPath(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	return id, err == nil && id > 0
}

func respondOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidOrder),
		errors.Is(err, ErrInvalidAddress):
		c.JSON(400, gin.H{"error": err.Error()})

	case errors.Is(err, ErrEmptyCart):
		c.JSON(400, gin.H{"error": "Shopping cart is empty"})

	case errors.Is(err, ErrOrderNotFound):
		c.JSON(404, gin.H{"error": "Order not found"})

	case errors.Is(err, ErrStockUnavailable),
		errors.Is(err, ErrInvalidTransition),
		errors.Is(err, ErrIdempotencyConflict):
		c.JSON(409, gin.H{"error": err.Error()})

	default:
		log.Printf("order error: %v", err)
		c.JSON(500, gin.H{"error": "Internal server error"})
	}
}

// POST /api/v1/checkout
func (h *Handler) Checkout(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		c.JSON(400, gin.H{
			"error": "Idempotency-Key header is required",
		})
		return
	}

	var req CheckoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondOrderError(c, ErrInvalidOrder)
		return
	}

	result, err := h.service.Checkout(
		c.Request.Context(), buyerID, req.AddressID, key,
	)
	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Pending order created",
		"order": result,
	})
}

// GET /api/v1/me/orders
func (h *Handler) ListOrders(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	items, err := h.service.ListOrders(
		c.Request.Context(), buyerID,
	)
	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(200, gin.H{"items": items})
}

// GET /api/v1/me/orders/:id
func (h *Handler) GetOrder(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	orderID, valid := orderIDFromPath(c)
	if !valid {
		respondOrderError(c, ErrInvalidOrder)
		return
	}

	result, err := h.service.GetOrder(
		c.Request.Context(), buyerID, orderID,
	)
	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(200, gin.H{"order": result})
}

// POST /api/v1/me/orders/:id/cancel
func (h *Handler) CancelOrder(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	orderID, valid := orderIDFromPath(c)
	if !valid {
		respondOrderError(c, ErrInvalidOrder)
		return
	}

	if err := h.service.CancelOrder(
		c.Request.Context(), buyerID, orderID,
	); err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(200, gin.H{"message": "Order cancelled"})
}
