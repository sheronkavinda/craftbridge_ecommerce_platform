package auth

import (
	"errors"
	"log"
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
	auth := rg.Group("/auth")
	auth.POST("/register", handler.Register)
	auth.POST("/login", handler.Login)
	auth.POST("/logout", platformhttp.RequireAuthenticatedUser(), handler.Logout)
	auth.POST("/forgot-password", handler.ForgotPassword)
	auth.POST("/reset-password", handler.ResetPassword)
	auth.GET("/me", platformhttp.RequireAuthenticatedUser(), handler.Me)
}

func (h *Handler) Register(c *gin.Context) {
	var input RegisterInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	response, err := h.service.Register(c.Request.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		case errors.Is(err, ErrInvalidEmail), errors.Is(err, ErrInvalidName),
			errors.Is(err, ErrWeakPassword), errors.Is(err, ErrPasswordTooLong):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create account"})
		}
		return
	}
	c.JSON(http.StatusCreated, response)
}

func (h *Handler) Login(c *gin.Context) {
	var input LoginInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	response, err := h.service.Login(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": ErrInvalidCredentials.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not sign in"})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *Handler) Logout(c *gin.Context) {
	userID, err := strconv.ParseInt(c.GetString(platformhttp.ContextKeyActorID), 10, 64)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	if err := h.service.Logout(c.Request.Context(), userID, c.GetString(platformhttp.ContextKeySessionID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not revoke session"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) ForgotPassword(c *gin.Context) {
	var input ForgotPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.service.ForgotPassword(c.Request.Context(), input); err != nil {
		if errors.Is(err, ErrPasswordResetDeliveryFailed) {
			log.Printf("password reset email delivery failed: %v", err)
			c.JSON(http.StatusOK, gin.H{"message": "If the account exists, reset instructions have been sent"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not process reset request"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "If the account exists, reset instructions have been sent"})
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var input ResetPasswordInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if err := h.service.ResetPassword(c.Request.Context(), input); err != nil {
		if errors.Is(err, ErrWeakPassword) || errors.Is(err, ErrPasswordTooLong) ||
			errors.Is(err, ErrInvalidResetToken) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset password"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}

func (h *Handler) Me(c *gin.Context) {
	userID, err := strconv.ParseInt(c.GetString("actor_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authenticated user"})
		return
	}
	user, err := h.service.Me(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": user.ID, "name": user.Name, "email": user.Email,
		"roles": user.Roles, "status": user.Status,
	})
}
