
package catalog

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

// Read the verified user ID from authentication middleware.
func catalogUserID(c *gin.Context) (int64, bool) {
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

func parseProductID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	return id, err == nil && id > 0
}

// Handle known catalog errors.
func catalogError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidProductInput):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product or filter data",
		})

	case errors.Is(err, ErrCategoryUnavailable):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Category does not exist or is inactive",
		})

	case errors.Is(err, ErrShopUnavailable):
		c.JSON(http.StatusForbidden, gin.H{
			"error": "An active, approved seller shop is required",
		})

	case errors.Is(err, ErrProductNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Product not found",
		})

	default:
		log.Printf("Catalog error: %v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	}
}

// POST /api/v1/seller/products
func (h *Handler) CreateProduct(c *gin.Context) {
	userID, ok := catalogUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	var req CreateProductRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product JSON",
		})
		return
	}

	product, err := h.service.CreateProduct(
		c.Request.Context(),
		userID,
		req,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Product created as draft",
		"product": product,
	})
}

// GET /api/v1/seller/products
func (h *Handler) ListMyProducts(c *gin.Context) {
	userID, ok := catalogUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	items, err := h.service.ListMyProducts(
		c.Request.Context(),
		userID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

// GET /api/v1/seller/products/:id
func (h *Handler) GetMyProduct(c *gin.Context) {
	userID, ok := catalogUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	productID, valid := parseProductID(c)

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product ID",
		})
		return
	}

	product, err := h.service.GetMyProduct(
		c.Request.Context(),
		userID,
		productID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"product": product,
	})
}

// PATCH /api/v1/seller/products/:id
func (h *Handler) UpdateProduct(c *gin.Context) {
	userID, ok := catalogUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	productID, valid := parseProductID(c)

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product ID",
		})
		return
	}

	var req UpdateProductRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid update JSON",
		})
		return
	}

	product, err := h.service.UpdateProduct(
		c.Request.Context(),
		userID,
		productID,
		req,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Product updated",
		"product": product,
	})
}

// DELETE /api/v1/seller/products/:id
// Safely archive a product instead of deleting it.
func (h *Handler) ArchiveProduct(c *gin.Context) {
	userID, ok := catalogUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	productID, valid := parseProductID(c)

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product ID",
		})
		return
	}

	product, err := h.service.ArchiveProduct(
		c.Request.Context(),
		userID,
		productID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Product archived",
		"product": product,
	})
}

// GET /api/v1/categories
func (h *Handler) ListCategories(c *gin.Context) {
	items, err := h.service.ListCategories(
		c.Request.Context(),
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

// GET /api/v1/products/:id
func (h *Handler) GetPublicProduct(c *gin.Context) {
	productID, valid := parseProductID(c)

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product ID",
		})
		return
	}

	product, err := h.service.GetPublicProduct(
		c.Request.Context(),
		productID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"product": product,
	})
}

// Parse an optional integer query parameter.
func queryInt64(c *gin.Context, key string) (*int64, error) {
	raw := c.Query(key)

	if raw == "" {
		return nil, nil
	}

	value, err := strconv.ParseInt(raw, 10, 64)

	if err != nil {
		return nil, err
	}

	return &value, nil
}

// GET /api/v1/products
// Supports search, category, price, sorting and pagination.
func (h *Handler) ListPublicProducts(c *gin.Context) {
	page, pageSize := 1, 12

	var err error

	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)

		if err != nil {
			catalogError(c, ErrInvalidProductInput)
			return
		}
	}

	if raw := c.Query("page_size"); raw != "" {
		pageSize, err = strconv.Atoi(raw)

		if err != nil {
			catalogError(c, ErrInvalidProductInput)
			return
		}
	}

	categoryID, err := queryInt64(c, "category_id")

	if err != nil {
		catalogError(c, ErrInvalidProductInput)
		return
	}

	minPrice, err := queryInt64(c, "min_price_cents")

	if err != nil {
		catalogError(c, ErrInvalidProductInput)
		return
	}

	maxPrice, err := queryInt64(c, "max_price_cents")

	if err != nil {
		catalogError(c, ErrInvalidProductInput)
		return
	}

	sort := c.DefaultQuery("sort", "newest")

	result, err := h.service.ListPublicProducts(
		c.Request.Context(),
		ListOptions{
			Search:        c.Query("search"),
			CategoryID:    categoryID,
			MinPriceCents: minPrice,
			MaxPriceCents: maxPrice,
			Sort:          sort,
			Page:          page,
			PageSize:      pageSize,
		},
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, result)
}
