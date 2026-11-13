package admin

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes HTTP endpoints for administrative operations.
type Handler struct {
	service *Service
}

// NewHandler creates a new admin HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ListUsers handles GET /api/v1/admin/users.
func (h *Handler) ListUsers(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	role := c.Query("role")
	status := c.Query("status")

	_, _, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)
	users, err := h.service.ListUsers(ctx, role, status, limit, offset)
	if err != nil {
		log.Printf("[req_id=%s] handler.ListUsers error: %v", reqID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": users})
}

// UpdateUserStatus handles PATCH/PUT /api/v1/admin/users/:id/status.
func (h *Handler) UpdateUserStatus(c *gin.Context) {
	userID := strings.TrimSpace(c.Param("id"))
	var input UpdateUserStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	actorID, actorName, reqID := extractVerifiedActor(c)
	user, err := h.service.UpdateUserStatus(c.Request.Context(), actorID, actorName, reqID, userID, input)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to update user status")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": user})
}

// ListSellerApplications handles GET /api/v1/admin/sellers and GET /api/v1/admin/sellers/applications.
func (h *Handler) ListSellerApplications(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := c.Query("status")

	_, _, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)
	sellers, err := h.service.ListSellerApplications(ctx, status, limit, offset)
	if err != nil {
		log.Printf("[req_id=%s] handler.ListSellerApplications error: %v", reqID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list seller applications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": sellers})
}

// UpdateSellerStatus handles PUT/PATCH /api/v1/admin/sellers/:id/status.
func (h *Handler) UpdateSellerStatus(c *gin.Context) {
	sellerID := strings.TrimSpace(c.Param("id"))
	var input UpdateSellerStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	actorID, actorName, reqID := extractVerifiedActor(c)
	seller, err := h.service.UpdateSellerStatus(c.Request.Context(), actorID, actorName, reqID, sellerID, input)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to update seller status")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": seller})
}

// ListRefunds handles GET /api/v1/admin/refunds.
func (h *Handler) ListRefunds(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := c.Query("status")

	_, _, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)
	refunds, err := h.service.ListRefunds(ctx, status, limit, offset)
	if err != nil {
		log.Printf("[req_id=%s] handler.ListRefunds error: %v", reqID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list refunds"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": refunds})
}

// UpdateRefundStatus handles PUT/PATCH /api/v1/admin/refunds/:id/status.
func (h *Handler) UpdateRefundStatus(c *gin.Context) {
	refundID := strings.TrimSpace(c.Param("id"))
	var input UpdateRefundStatusInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	actorID, actorName, reqID := extractVerifiedActor(c)
	refund, err := h.service.UpdateRefundStatus(c.Request.Context(), actorID, actorName, reqID, refundID, input)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to update refund status")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": refund})
}

// ListCategories handles GET /api/v1/admin/categories.
func (h *Handler) ListCategories(c *gin.Context) {
	_, _, reqID := extractVerifiedActor(c)
	ctx := WithRequestID(c.Request.Context(), reqID)
	categories, err := h.service.ListCategories(ctx)
	if err != nil {
		log.Printf("[req_id=%s] handler.ListCategories error: %v", reqID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list categories"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": categories})
}

// CreateCategory handles POST /api/v1/admin/categories.
func (h *Handler) CreateCategory(c *gin.Context) {
	var input CategoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	actorID, actorName, reqID := extractVerifiedActor(c)
	category, err := h.service.CreateCategory(c.Request.Context(), actorID, actorName, reqID, input)
	if err != nil {
		log.Printf("[req_id=%s] handler.CreateCategory error: %v", reqID, err)
		handleServiceError(c, reqID, err, "failed to create category")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": category})
}

// UpdateCategory handles PUT /api/v1/admin/categories/:id.
func (h *Handler) UpdateCategory(c *gin.Context) {
	categoryID := strings.TrimSpace(c.Param("id"))
	var input CategoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload"})
		return
	}

	actorID, actorName, reqID := extractVerifiedActor(c)
	category, err := h.service.UpdateCategory(c.Request.Context(), actorID, actorName, reqID, categoryID, input)
	if err != nil {
		handleServiceError(c, reqID, err, "failed to update category")
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": category})
}

// DeleteCategory handles DELETE /api/v1/admin/categories/:id.
func (h *Handler) DeleteCategory(c *gin.Context) {
	categoryID := strings.TrimSpace(c.Param("id"))
	actorID, actorName, reqID := extractVerifiedActor(c)

	if err := h.service.DeleteCategory(c.Request.Context(), actorID, actorName, reqID, categoryID); err != nil {
		handleServiceError(c, reqID, err, "failed to delete category")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "category deleted"})
}

func extractVerifiedActor(c *gin.Context) (actorID, actorName, requestID string) {
	if val, exists := c.Get(platformhttp.ContextKeyActorID); exists {
		if str, ok := val.(string); ok {
			actorID = str
		}
	}
	if val, exists := c.Get(platformhttp.ContextKeyActorName); exists {
		if str, ok := val.(string); ok {
			actorName = str
		}
	}
	if val, exists := c.Get(platformhttp.ContextKeyRequestID); exists {
		if str, ok := val.(string); ok {
			requestID = str
		}
	}
	return actorID, actorName, requestID
}

func handleServiceError(c *gin.Context, reqID string, err error, fallbackMsg string) {
	log.Printf("[req_id=%s] %s: %v", reqID, fallbackMsg, err)
	switch {
	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrForbiddenIDOR):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, ErrCategorySlugExists):
		c.JSON(http.StatusConflict, gin.H{"error": "A category with this slug already exists"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallbackMsg})
	}
}
