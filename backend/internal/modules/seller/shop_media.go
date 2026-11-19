
package seller

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type ShopImageStore interface {
	Save(context.Context, []byte, string) (string, error)
	Delete(context.Context, string) error
}

type ShopMediaHandler struct {
	repo  *Repository
	store ShopImageStore
}

func NewShopMediaHandler(
	repo *Repository,
	store ShopImageStore,
) *ShopMediaHandler {
	return &ShopMediaHandler{
		repo:  repo,
		store: store,
	}
}

// POST /api/v1/seller/shop/images/:kind
// kind is "profile" or "banner".
func (h *ShopMediaHandler) UploadShopImage(c *gin.Context) {
	userID, ok := authenticatedUserID(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	kind := c.Param("kind")

	if kind != "profile" && kind != "banner" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Image type must be profile or banner",
		})
		return
	}

	ctx := c.Request.Context()

	previous, err := h.repo.GetShopByUserID(ctx, userID)

	if err != nil || previous.Status != "active" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Active seller shop required",
		})
		return
	}

	const maxBytes = 3 * 1024 * 1024

	c.Request.Body = http.MaxBytesReader(
		c.Writer, c.Request.Body, maxBytes+1024*1024,
	)

	header, err := c.FormFile("image")

	if err != nil || header.Size <= 0 || header.Size > maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "A valid image under 3 MB is required",
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
		io.LimitReader(file, maxBytes+1),
	)

	if err != nil || len(data) == 0 || len(data) > maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid image size",
		})
		return
	}

	sample := data
	if len(sample) > 512 {
		sample = sample[:512]
	}

	var extension string

	switch http.DetectContentType(sample) {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	default:
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error": "Unsupported image format",
		})
		return
	}

	imageURL, err := h.store.Save(ctx, data, extension)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Image storage failed",
		})
		return
	}

	// Only the authenticated shop owner can update the image.
	query := `
		UPDATE shops
		SET
			profile_image_url =
				CASE WHEN $2 = 'profile'
					THEN $3 ELSE profile_image_url END,
			banner_image_url =
				CASE WHEN $2 = 'banner'
					THEN $3 ELSE banner_image_url END,
			updated_at = NOW()
		WHERE owner_user_id = $1
		  AND status = 'active'
		RETURNING ` + shopColumns

	updated, err := scanShop(h.repo.db.QueryRow(
		ctx, query, userID, kind, imageURL,
	))

	if err != nil {
		h.store.Delete(context.Background(), imageURL)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Unable to update shop image",
		})
		return
	}

	// Best-effort cleanup of the previously used image.
	if kind == "profile" &&
		previous.ProfileImageURL != nil {
		h.store.Delete(
			context.Background(), *previous.ProfileImageURL,
		)
	}

	if kind == "banner" &&
		previous.BannerImageURL != nil {
		h.store.Delete(
			context.Background(), *previous.BannerImageURL,
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Shop image updated successfully",
		"shop": updated,
	})
}

func RegisterShopMediaRoutes(
	api *gin.RouterGroup,
	handler *ShopMediaHandler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("shop media requires authentication")
	}

	group := api.Group("/seller")
	group.Use(authenticate)

	group.POST(
		"/shop/images/:kind",
		handler.UploadShopImage,
	)
}
