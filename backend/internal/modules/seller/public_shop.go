package seller

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type publicShop struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Slug            string  `json:"slug"`
	Description     string  `json:"description"`
	CountryCode     string  `json:"country_code"`
	ProfileImageURL *string `json:"profile_image_url"`
	BannerImageURL  *string `json:"banner_image_url"`
}

func (r *Repository) findPublicShop(ctx context.Context, slug string) (*publicShop, error) {
	const query = `
		SELECT id, name, slug, COALESCE(description, ''), country_code,
		       profile_image_url, banner_image_url
		FROM shops
		WHERE slug = $1 AND status = 'active'
	`

	var shop publicShop
	err := r.db.QueryRow(ctx, query, slug).Scan(
		&shop.ID,
		&shop.Name,
		&shop.Slug,
		&shop.Description,
		&shop.CountryCode,
		&shop.ProfileImageURL,
		&shop.BannerImageURL,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShopNotFound
	}
	if err != nil {
		return nil, err
	}

	return &shop, nil
}

func (h *Handler) GetPublicShop(c *gin.Context) {
	shop, err := h.service.repo.findPublicShop(c.Request.Context(), c.Param("slug"))
	if err != nil {
		respondSellerError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"shop": shop})
}