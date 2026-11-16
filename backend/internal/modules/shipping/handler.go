
package shipping

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

func authenticatedUser(c *gin.Context) (int64, bool) {
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

func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	return id, err == nil && id > 0
}

func shippingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidShipment):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid shipment request",
		})
	case errors.Is(err, ErrShipmentNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Shipment or order not found",
		})
	case errors.Is(err, ErrFulfillmentState):
		c.JSON(http.StatusConflict, gin.H{
			"error": "Invalid fulfillment transition",
		})
	default:
		log.Printf("shipping error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	}
}

// GET /api/v1/seller/orders
func (h *Handler) ListSellerOrders(c *gin.Context) {
	userID, ok := authenticatedUser(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	result, err := h.service.ListSellerOrders(
		c.Request.Context(), userID,
	)
	if err != nil {
		shippingError(c, err)
		return
	}

	c.JSON(200, gin.H{"items": result})
}

// PATCH /api/v1/seller/orders/:id/fulfillment
func (h *Handler) UpdateFulfillment(c *gin.Context) {
	userID, ok := authenticatedUser(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	sellerOrderID, valid := pathID(c)
	if !valid {
		shippingError(c, ErrInvalidShipment)
		return
	}

	var req FulfillmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		shippingError(c, ErrInvalidShipment)
		return
	}

	if err := h.service.UpdateFulfillment(
		c.Request.Context(), userID, sellerOrderID, req,
	); err != nil {
		shippingError(c, err)
		return
	}

	c.JSON(200, gin.H{"message": "Fulfillment updated"})
}

// GET /api/v1/me/orders/:id/tracking
func (h *Handler) GetBuyerTracking(c *gin.Context) {
	userID, ok := authenticatedUser(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	orderID, valid := pathID(c)
	if !valid {
		shippingError(c, ErrInvalidShipment)
		return
	}

	result, err := h.service.GetBuyerTracking(
		c.Request.Context(), userID, orderID,
	)
	if err != nil {
		shippingError(c, err)
		return
	}

	c.JSON(200, gin.H{"shipments": result})
}

// Register shipping APIs behind authenticated routes.
func RegisterRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("shipping routes require authentication")
	}

	sellerRoutes := api.Group("/seller/orders")
	sellerRoutes.Use(authenticate)
	sellerRoutes.GET("", handler.ListSellerOrders)
	sellerRoutes.PATCH(
		"/:id/fulfillment",
		handler.UpdateFulfillment,
	)

	buyerRoutes := api.Group("/me/orders")
	buyerRoutes.Use(authenticate)
	buyerRoutes.GET("/:id/tracking", handler.GetBuyerTracking)
}
