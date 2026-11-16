
package catalog

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/seller/products/:id/inventory
func (h *Handler) GetProductInventory(c *gin.Context) {
	userID, authenticated := catalogUserID(c)
	productID, valid := parseProductID(c)

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	if !valid {
		catalogError(c, ErrInvalidProductInput)
		return
	}

	inventory, err := h.service.GetInventory(
		c.Request.Context(), userID, productID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inventory": inventory,
	})
}

// POST /api/v1/seller/products/:id/inventory-adjustments
func (h *Handler) AdjustProductInventory(c *gin.Context) {
	userID, authenticated := catalogUserID(c)
	productID, valid := parseProductID(c)

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	if !valid {
		catalogError(c, ErrInvalidProductInput)
		return
	}

	var req InventoryAdjustmentRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid stock adjustment",
		})
		return
	}

	inventory, err := h.service.AdjustInventory(
		c.Request.Context(), userID, productID, req,
	)

	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidStock):
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid stock adjustment",
			})
		case errors.Is(err, ErrInsufficientStock):
			c.JSON(http.StatusConflict, gin.H{
				"error": "Insufficient available stock",
			})
		default:
			catalogError(c, err)
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Inventory updated successfully",
		"inventory": inventory,
	})
}

// GET /api/v1/seller/products/:id/inventory-movements
func (h *Handler) ListInventoryHistory(c *gin.Context) {
	userID, authenticated := catalogUserID(c)
	productID, valid := parseProductID(c)

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	if !valid {
		catalogError(c, ErrInvalidProductInput)
		return
	}

	movements, err := h.service.ListInventoryMovements(
		c.Request.Context(), userID, productID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": movements,
	})
}

// Additional authenticated inventory routes.
// These must use Adeesha's authentication middleware.
func RegisterInventoryRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("inventory routes require authentication")
	}

	group := api.Group("/seller/products")
	group.Use(authenticate)

	group.GET("/:id/inventory", handler.GetProductInventory)
	group.POST("/:id/inventory-adjustments", handler.AdjustProductInventory)
	group.GET("/:id/inventory-movements", handler.ListInventoryHistory)
}
