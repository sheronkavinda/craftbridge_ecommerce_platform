
package catalog

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const maxImageBytes = 5 * 1024 * 1024

type ImageHandler struct {
	repo  *Repository
	store ImageStore
}

func NewImageHandler(
	repo *Repository,
	store ImageStore,
) *ImageHandler {
	return &ImageHandler{
		repo: repo,
		store: store,
	}
}

// POST /api/v1/seller/products/:id/images
func (h *ImageHandler) UploadProductImage(c *gin.Context) {
	userID, authenticated := catalogUserID(c)
	productID, valid := parseProductID(c)

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product ID",
		})
		return
	}

	altText := strings.TrimSpace(c.Query("alt_text"))

	if utf8.RuneCountInString(altText) > 250 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Alt text exceeds 250 characters",
		})
		return
	}

	// Restrict the full multipart request size.
	c.Request.Body = http.MaxBytesReader(
		c.Writer, c.Request.Body, maxImageBytes+1024*1024,
	)

	header, err := c.FormFile("image")

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Image file is required or request is too large",
		})
		return
	}

	if header.Size > maxImageBytes || header.Size <= 0 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "Image must be between 1 byte and 5 MB",
		})
		return
	}

	file, err := header.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Cannot open image",
		})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(
		io.LimitReader(file, maxImageBytes+1),
	)

	if err != nil || len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Cannot read image",
		})
		return
	}

	if len(data) > maxImageBytes {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error": "Image exceeds 5 MB",
		})
		return
	}

	sample := data

	if len(sample) > 512 {
		sample = sample[:512]
	}

	detected := http.DetectContentType(sample)

	var extension string

	switch detected {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": "Only JPEG, PNG and WebP are supported",
		})
		return
	}

	// Verify product ownership before writing the file.
	// AddProductImage performs a second ownership check.
	var ownedID int64

	err = h.repo.db.QueryRow(c.Request.Context(), `
		SELECT p.id
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		WHERE p.id = $1
		  AND s.owner_user_id = $2
		  AND s.status = 'active'
		  AND p.status <> 'archived'
	`, productID, userID).Scan(&ownedID)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Product unavailable",
		})
		return
	}

	imageURL, err := h.store.Save(
		c.Request.Context(), data, extension,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Image storage failed",
		})
		return
	}

	image, err := h.repo.AddProductImage(
		c.Request.Context(),
		userID, productID, imageURL, altText,
	)

	if err != nil {
		// Remove the uploaded file if database insert fails.
		h.store.Delete(c.Request.Context(), imageURL)

		if err == ErrImageLimitReached {
			c.JSON(http.StatusConflict, gin.H{
				"error": "Maximum 8 images per product",
			})
			return
		}

		catalogError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Product image uploaded",
		"image": image,
	})
}

// GET /api/v1/products/:id/images
func (h *ImageHandler) ListProductImages(c *gin.Context) {
	productID, valid := parseProductID(c)

	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid product ID",
		})
		return
	}

	images, err := h.repo.ListPublicProductImages(
		c.Request.Context(), productID,
	)

	if err != nil {
		catalogError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": images,
	})
}

// DELETE /api/v1/seller/product-images/:imageID
func (h *ImageHandler) DeleteProductImage(c *gin.Context) {
	userID, authenticated := catalogUserID(c)

	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	imageID, err := strconv.ParseInt(
		c.Param("imageID"), 10, 64,
	)

	if err != nil || imageID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid image ID",
		})
		return
	}

	imageURL, err := h.repo.RemoveProductImage(
		c.Request.Context(), userID, imageID,
	)

	if err != nil {
		if err == ErrImageNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Image not found",
			})
			return
		}

		catalogError(c, err)
		return
	}

	// File cleanup follows successful metadata deletion.
	// Failed cleanup can be retried by maintenance tooling.
	cleanupErr := h.store.Delete(
		c.Request.Context(), imageURL,
	)

	if cleanupErr != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "Image record deleted; file cleanup pending",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Image deleted successfully",
	})
}

// Register product-image API routes.
func RegisterImageRoutes(
	api *gin.RouterGroup,
	handler *ImageHandler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("image routes require authentication")
	}

	api.GET("/products/:id/images", handler.ListProductImages)

	protected := api.Group("/seller")
	protected.Use(authenticate)

	protected.POST(
		"/products/:id/images",
		handler.UploadProductImage,
	)

	protected.DELETE(
		"/product-images/:imageID",
		handler.DeleteProductImage,
	)
}
