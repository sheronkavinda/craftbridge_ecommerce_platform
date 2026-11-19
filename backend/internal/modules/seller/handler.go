
package seller

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

// Get the verified user ID from authentication middleware.
// The ID must be set by the server, not by request JSON.
func authenticatedUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get("user_id")

	if !exists {
		return 0, false
	}

	switch id := value.(type) {
	case int64:
		return id, id > 0
	case int:
		return int64(id), id > 0
	default:
		return 0, false
	}
}

// Shared error response handling.
func respondSellerError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request data",
		})

	case errors.Is(err, ErrApplicationAlreadyExists):
		c.JSON(http.StatusConflict, gin.H{
			"error": "You have already submitted a seller application",
		})

	case errors.Is(err, ErrApplicationNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Seller application not found",
		})

	case errors.Is(err, ErrApplicationNotPending):
		c.JSON(http.StatusConflict, gin.H{
			"error": "Application has already been reviewed",
		})

	case errors.Is(err, ErrShopNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Shop not found or unavailable",
		})

	default:
		log.Printf("Seller operation failed: %v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	}
}

// POST /api/v1/seller/applications
func (h *Handler) CreateApplication(c *gin.Context) {
	userID, ok := authenticatedUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	var req CreateSellerApplicationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid seller application data",
		})
		return
	}

	application, err := h.service.ApplyForSeller(
		c.Request.Context(),
		userID,
		req,
	)

	if err != nil {
		respondSellerError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Seller application submitted successfully",
		"application": application,
	})
}

// GET /api/v1/seller/application
func (h *Handler) GetMyApplication(c *gin.Context) {
	userID, ok := authenticatedUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	application, err := h.service.GetMyApplication(
		c.Request.Context(),
		userID,
	)

	if err != nil {
		respondSellerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"application": application,
	})
}

// GET /api/v1/seller/shop
func (h *Handler) GetMyShop(c *gin.Context) {
	userID, ok := authenticatedUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	shop, err := h.service.GetMyShop(
		c.Request.Context(),
		userID,
	)

	if err != nil {
		respondSellerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"shop": shop,
	})
}

// PATCH /api/v1/seller/shop
func (h *Handler) UpdateMyShop(c *gin.Context) {
	userID, ok := authenticatedUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	var req UpdateShopRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid shop information",
		})
		return
	}

	shop, err := h.service.UpdateMyShop(
		c.Request.Context(),
		userID,
		req,
	)

	if err != nil {
		respondSellerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Shop updated successfully",
		"shop": shop,
	})
}

// POST /api/v1/admin/seller-applications/:id/decision
// This must be registered behind admin authorization.
func (h *Handler) DecideApplication(c *gin.Context) {
	adminUserID, ok := authenticatedUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	applicationID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)

	if err != nil || applicationID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid application ID",
		})
		return
	}

	var req struct {
		Decision string `json:"decision" binding:"required,oneof=approve reject"`
		Reason   string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Decision must be approve or reject",
		})
		return
	}

	switch req.Decision {
	case "approve":
		shop, err := h.service.ApproveSellerApplication(
			c.Request.Context(),
			applicationID,
			adminUserID,
		)

		if err != nil {
			respondSellerError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Seller approved and shop created",
			"shop": shop,
		})

	case "reject":
		application, err := h.service.RejectSellerApplication(
			c.Request.Context(),
			applicationID,
			adminUserID,
			req.Reason,
		)

		if err != nil {
			respondSellerError(c, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Seller application rejected",
			"application": application,
		})
	}
}
