package report

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler exposes HTTP endpoints for sales and revenue reports.
type Handler struct {
	service *Service
}

// NewHandler creates a new report HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetRevenueSummary handles GET /api/v1/reports/revenue.
func (h *Handler) GetRevenueSummary(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	summary, err := h.service.GetRevenueSummary(c.Request.Context(), startDate, endDate)
	if err != nil {
		if errors.Is(err, ErrInvalidDateRange) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate revenue summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": summary})
}

// GetDailyRevenue handles GET /api/v1/reports/revenue/daily.
func (h *Handler) GetDailyRevenue(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))

	points, err := h.service.GetDailyRevenueBreakdown(c.Request.Context(), startDate, endDate, limit)
	if err != nil {
		if errors.Is(err, ErrInvalidDateRange) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate daily revenue report"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": points})
}

// GetSellerRevenue handles GET /api/v1/reports/sellers.
func (h *Handler) GetSellerRevenue(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	sellers, err := h.service.GetSellerRevenueReport(c.Request.Context(), startDate, endDate, limit)
	if err != nil {
		if errors.Is(err, ErrInvalidDateRange) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate seller revenue report"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sellers})
}
