
package seller

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Dashboard struct {
	Shop              *Shop `json:"shop"`
	TotalProducts     int64 `json:"total_products"`
	PublishedProducts int64 `json:"published_products"`
	DraftProducts     int64 `json:"draft_products"`
	ArchivedProducts  int64 `json:"archived_products"`
	LowStockProducts  int64 `json:"low_stock_products"`
	AvailableUnits    int64 `json:"available_units"`
}

// GetDashboard calculates seller statistics
// using the canonical products and inventory tables.
func (r *Repository) GetDashboard(
	ctx context.Context,
	userID int64,
) (*Dashboard, error) {

	shop, err := r.GetShopByUserID(ctx, userID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShopNotFound
	}
	if err != nil {
		return nil, err
	}

	dashboard := &Dashboard{
		Shop: shop,
	}

	err = r.db.QueryRow(ctx, `
		SELECT
			COUNT(*),

			COUNT(*) FILTER (
				WHERE p.status = 'published'
			),

			COUNT(*) FILTER (
				WHERE p.status = 'draft'
			),

			COUNT(*) FILTER (
				WHERE p.status = 'archived'
			),

			COUNT(*) FILTER (
				WHERE p.status = 'published'
				  AND COALESCE(i.available_qty, 0) <= 5
			),

			COALESCE(SUM(i.available_qty) FILTER (
				WHERE p.status <> 'archived'
			), 0)

		FROM products p
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE p.shop_id = $1
	`, shop.ID).Scan(
		&dashboard.TotalProducts,
		&dashboard.PublishedProducts,
		&dashboard.DraftProducts,
		&dashboard.ArchivedProducts,
		&dashboard.LowStockProducts,
		&dashboard.AvailableUnits,
	)

	if err != nil {
		return nil, err
	}

	return dashboard, nil
}

// Service method.
func (s *Service) GetDashboard(
	ctx context.Context,
	userID int64,
) (*Dashboard, error) {

	if userID <= 0 {
		return nil, ErrInvalidInput
	}

	dashboard, err := s.repo.GetDashboard(ctx, userID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShopNotFound
	}

	return dashboard, err
}
