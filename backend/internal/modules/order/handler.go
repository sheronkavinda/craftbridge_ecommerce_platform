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
	return &Handler{
		service: service,
	}
}

// Read the authenticated user's ID from trusted middleware.
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

// Read the order ID from the URL.
func orderIDFromPath(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)

	return id, err == nil && id > 0
}

// Return appropriate HTTP errors.
func respondOrderError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidOrder),
		errors.Is(err, ErrInvalidAddress):

		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})

	case errors.Is(err, ErrEmptyCart):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Shopping cart is empty",
		})

	case errors.Is(err, ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Order not found",
		})

	case errors.Is(err, ErrStockUnavailable),
		errors.Is(err, ErrInvalidTransition),
		errors.Is(err, ErrIdempotencyConflict):

		c.JSON(http.StatusConflict, gin.H{
			"error": err.Error(),
		})

	case errors.Is(err, ErrShippingUnavailable):
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "Shipping is unavailable for the selected destination",
		})

	case errors.Is(err, ErrInventoryUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Inventory service is unavailable",
		})

	default:
		log.Printf("Order error: %v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	}
}

// POST /api/v1/checkout
func (h *Handler) Checkout(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	key := c.GetHeader("Idempotency-Key")

	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{
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
		c.Request.Context(),
		buyerID,
		req.AddressID,
		key,
	)

	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Pending order created",
		"order":   result,
	})
}

// GET /api/v1/me/orders
func (h *Handler) ListOrders(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	items, err := h.service.ListOrders(
		c.Request.Context(),
		buyerID,
	)

	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

// GET /api/v1/me/orders/:id
func (h *Handler) GetOrder(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	orderID, valid := orderIDFromPath(c)

	if !valid {
		respondOrderError(c, ErrInvalidOrder)
		return
	}

	result, err := h.service.GetOrder(
		c.Request.Context(),
		buyerID,
		orderID,
	)

	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"order": result,
	})
}

// POST /api/v1/me/orders/:id/cancel
func (h *Handler) CancelOrder(c *gin.Context) {
	buyerID, ok := authenticatedBuyer(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	orderID, valid := orderIDFromPath(c)

	if !valid {
		respondOrderError(c, ErrInvalidOrder)
		return
	}

	err := h.service.CancelOrder(
		c.Request.Context(),
		buyerID,
		orderID,
	)

	if err != nil {
		respondOrderError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Order cancelled",
	})
}
