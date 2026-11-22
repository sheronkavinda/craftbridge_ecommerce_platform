package account

import (
	"errors"
	"net/http"
	"strconv"

	platformhttp "craftbridge/backend/internal/platform/http"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(rg *gin.RouterGroup, handler *Handler) {
	me := rg.Group("/me")
	me.Use(platformhttp.RequireAuthenticatedUser())
	me.GET("", handler.GetProfile)
	me.PATCH("", handler.UpdateProfile)
	me.GET("/addresses", handler.ListAddresses)
	me.POST("/addresses", handler.CreateAddress)
	me.PATCH("/addresses/:id", handler.UpdateAddress)
	me.DELETE("/addresses/:id", handler.DeleteAddress)
}

func (h *Handler) currentUserID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.GetString(platformhttp.ContextKeyActorID), 10, 64)
	return id, err == nil && id > 0
}

func (h *Handler) GetProfile(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	profile, err := h.service.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load profile"})
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (h *Handler) UpdateProfile(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	var input UpdateProfileInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.service.UpdateProfile(c.Request.Context(), userID, input); err != nil {
		if errors.Is(err, ErrInvalidName) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update profile"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Profile updated"})
}

func (h *Handler) ListAddresses(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	addresses, err := h.service.ListAddresses(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load addresses"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"addresses": addresses})
}

func (h *Handler) CreateAddress(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	var address Address
	if err := c.ShouldBindJSON(&address); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	created, err := h.service.CreateAddress(c.Request.Context(), userID, address)
	if err != nil {
		if errors.Is(err, ErrInvalidAddress) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create address"})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h *Handler) UpdateAddress(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	addressID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address id"})
		return
	}
	var address Address
	if err := c.ShouldBindJSON(&address); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	updated, err := h.service.UpdateAddress(c.Request.Context(), userID, addressID, address)
	if err != nil {
		if errors.Is(err, ErrInvalidAddress) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update address"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

func (h *Handler) DeleteAddress(c *gin.Context) {
	userID, ok := h.currentUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	addressID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid address id"})
		return
	}
	if err := h.service.DeleteAddress(c.Request.Context(), userID, addressID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "address not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete address"})
		return
	}
	c.Status(http.StatusNoContent)
}
