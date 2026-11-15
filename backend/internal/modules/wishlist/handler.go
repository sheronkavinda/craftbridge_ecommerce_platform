package wishlist

import (
	"errors"
	"net/http"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// Handler handles wishlist HTTP requests.
type Handler struct {
	service *Service
}

// NewHandler creates a wishlist handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// getUserID retrieves the authenticated user's ID.
func getUserID(c *gin.Context) string {
	value, exists := c.Get(platformhttp.ContextKeyActorID)
	if !exists {
		return ""
	}

	userID, ok := value.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(userID)
}

// List handles GET /api/v1/me/wishlist.
func (h *Handler) List(c *gin.Context) {
	userID := getUserID(c)

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	result, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to load wishlist",
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Add handles POST /api/v1/me/wishlist.
func (h *Handler) Add(c *gin.Context) {
	userID := getUserID(c)

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	var input AddWishlistRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "valid productId is required",
		})
		return
	}

	item, err := h.service.Add(
		c.Request.Context(),
		userID,
		input.ProductID,
	)

	if errors.Is(err, ErrInvalidProductID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "product ID is required",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to add product to wishlist",
		})
		return
	}

	c.JSON(http.StatusCreated, item)
}

// Remove handles DELETE /api/v1/me/wishlist/:productId.
func (h *Handler) Remove(c *gin.Context) {
	userID := getUserID(c)

	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "authentication required",
		})
		return
	}

	productID := strings.TrimSpace(c.Param("productId"))

	err := h.service.Remove(
		c.Request.Context(),
		userID,
		productID,
	)

	if errors.Is(err, ErrInvalidProductID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "product ID is required",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to remove product from wishlist",
		})
		return
	}

	c.Status(http.StatusNoContent)
}
