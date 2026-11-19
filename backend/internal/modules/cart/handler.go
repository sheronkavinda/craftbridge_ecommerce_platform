package cart

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func buyerID(c *gin.Context) (int64, bool) {
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

func cartError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidCart):
		c.JSON(400, gin.H{"error": "Invalid cart request"})
	case errors.Is(err, ErrUnavailable):
		c.JSON(409, gin.H{"error": "Product unavailable or insufficient stock"})
	case errors.Is(err, ErrCartItemNotFound):
		c.JSON(404, gin.H{"error": "Cart item not found"})
	default:
		log.Printf("cart error: %v", err)
		c.JSON(500, gin.H{"error": "Internal server error"})
	}
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := buyerID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	result, err := h.repo.GetCart(c.Request.Context(), id)
	if err != nil {
		cartError(c, err)
		return
	}
	c.JSON(200, result)
}

func (h *Handler) Add(c *gin.Context) {
	id, ok := buyerID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	var req AddItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cartError(c, ErrInvalidCart)
		return
	}

	if err := h.repo.AddItem(c.Request.Context(), id, req); err != nil {
		cartError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "Item added"})
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := buyerID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	itemID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || itemID <= 0 {
		cartError(c, ErrInvalidCart)
		return
	}

	var req UpdateItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cartError(c, ErrInvalidCart)
		return
	}

	if err := h.repo.UpdateItem(
		c.Request.Context(), id, itemID, req.Quantity,
	); err != nil {
		cartError(c, err)
		return
	}

	c.JSON(200, gin.H{"message": "Quantity updated"})
}

func (h *Handler) Remove(c *gin.Context) {
	id, ok := buyerID(c)
	if !ok {
		c.JSON(401, gin.H{"error": "Authentication required"})
		return
	}

	itemID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || itemID <= 0 {
		cartError(c, ErrInvalidCart)
		return
	}

	if err := h.repo.RemoveItem(
		c.Request.Context(), id, itemID,
	); err != nil {
		cartError(c, err)
		return
	}

	c.JSON(200, gin.H{"message": "Item removed"})
}
