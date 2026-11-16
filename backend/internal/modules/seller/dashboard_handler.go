
package seller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GET /api/v1/seller/dashboard
func (h *Handler) GetDashboard(c *gin.Context) {
	userID, authenticated := authenticatedUserID(c)

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	dashboard, err := h.service.GetDashboard(
		c.Request.Context(), userID,
	)

	if err != nil {
		respondSellerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"dashboard": dashboard,
	})
}
