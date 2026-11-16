package shipping

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var ErrShippingRateNotFound = errors.New(
	"seller shop or shipping rate not found",
)

// ShippingRate stores a seller's delivery pricing rule.
type ShippingRate struct {
	ID                     int64  `json:"id"`
	ShopID                 int64  `json:"shop_id"`
	DestinationCountry     string `json:"destination_country"`
	BaseFeeCents           int64  `json:"base_fee_cents"`
	AdditionalItemFeeCents int64  `json:"additional_item_fee_cents"`
	IsActive               bool   `json:"is_active"`
}

// Request body for creating/updating a shipping rate.
type UpsertShippingRateRequest struct {
	BaseFeeCents           int64 `json:"base_fee_cents"`
	AdditionalItemFeeCents int64 `json:"additional_item_fee_cents"`
}

// Initial CraftBridge MVP: shipping to the USA.
func validShippingCountry(country string) bool {
	return country == "US"
}

func scanShippingRate(row pgx.Row) (*ShippingRate, error) {
	var rate ShippingRate

	err := row.Scan(
		&rate.ID,
		&rate.ShopID,
		&rate.DestinationCountry,
		&rate.BaseFeeCents,
		&rate.AdditionalItemFeeCents,
		&rate.IsActive,
	)
	if err != nil {
		return nil, err
	}

	return &rate, nil
}

// GET shipping rate for the authenticated seller's shop.
func (s *Service) GetSellerShippingRate(
	ctx context.Context,
	userID int64,
	country string,
) (*ShippingRate, error) {
	if userID <= 0 || !validShippingCountry(country) {
		return nil, ErrInvalidShipment
	}

	rate, err := scanShippingRate(s.db.QueryRow(ctx, `
		SELECT
			r.id,
			r.shop_id,
			r.destination_country,
			r.base_fee_cents,
			r.additional_item_fee_cents,
			r.is_active
		FROM seller_shipping_rates r
		JOIN shops sh ON sh.id = r.shop_id
		WHERE sh.owner_user_id = $1
		  AND sh.status = 'active'
		  AND r.destination_country = $2
	`, userID, country))

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShippingRateNotFound
	}

	return rate, err
}

// Create or update the seller's shipping price.
func (s *Service) UpsertSellerShippingRate(
	ctx context.Context,
	userID int64,
	country string,
	req UpsertShippingRateRequest,
) (*ShippingRate, error) {
	if userID <= 0 ||
		!validShippingCountry(country) ||
		req.BaseFeeCents < 0 ||
		req.BaseFeeCents > 1000000 ||
		req.AdditionalItemFeeCents < 0 ||
		req.AdditionalItemFeeCents > 100000 {
		return nil, ErrInvalidShipment
	}

	rate, err := scanShippingRate(s.db.QueryRow(ctx, `
		INSERT INTO seller_shipping_rates (
			shop_id,
			destination_country,
			base_fee_cents,
			additional_item_fee_cents
		)
		SELECT
			sh.id,
			$2,
			$3,
			$4
		FROM shops sh
		WHERE sh.owner_user_id = $1
		  AND sh.status = 'active'
		ON CONFLICT (shop_id, destination_country)
		DO UPDATE SET
			base_fee_cents = EXCLUDED.base_fee_cents,
			additional_item_fee_cents =
				EXCLUDED.additional_item_fee_cents,
			is_active = TRUE,
			updated_at = NOW()
		RETURNING
			id,
			shop_id,
			destination_country,
			base_fee_cents,
			additional_item_fee_cents,
			is_active
	`,
		userID,
		country,
		req.BaseFeeCents,
		req.AdditionalItemFeeCents,
	))

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShippingRateNotFound
	}

	return rate, err
}

// GET /api/v1/seller/shipping-rates/:country
func (h *Handler) GetShippingRate(c *gin.Context) {
	userID, ok := authenticatedUser(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	country := strings.ToUpper(
		strings.TrimSpace(c.Param("country")),
	)

	rate, err := h.service.GetSellerShippingRate(
		c.Request.Context(),
		userID,
		country,
	)

	if err != nil {
		respondShippingRateError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"rate": rate,
	})
}

// PUT /api/v1/seller/shipping-rates/:country
func (h *Handler) PutShippingRate(c *gin.Context) {
	userID, ok := authenticatedUser(c)

	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Authentication required",
		})
		return
	}

	country := strings.ToUpper(
		strings.TrimSpace(c.Param("country")),
	)

	var req UpsertShippingRateRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid shipping rate request",
		})
		return
	}

	rate, err := h.service.UpsertSellerShippingRate(
		c.Request.Context(),
		userID,
		country,
		req,
	)

	if err != nil {
		respondShippingRateError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Shipping rate saved successfully",
		"rate":    rate,
	})
}

func respondShippingRateError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidShipment):
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid shipping rate",
		})

	case errors.Is(err, ErrShippingRateNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Active seller shop or rate not found",
		})

	default:
		log.Printf("Shipping rate error: %v", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Internal server error",
		})
	}
}

// Register the authenticated seller shipping-rate APIs.
func RegisterShippingRateRoutes(
	api *gin.RouterGroup,
	handler *Handler,
	authenticate gin.HandlerFunc,
) {
	if authenticate == nil {
		panic("Shipping rate routes require authentication")
	}

	group := api.Group("/seller/shipping-rates")
	group.Use(authenticate)

	group.GET("/:country", handler.GetShippingRate)
	group.PUT("/:country", handler.PutShippingRate)
}
