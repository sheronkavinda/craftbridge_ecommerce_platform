package wishlist

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles wishlist database operations.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a wishlist repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Add saves a product to the user's wishlist.
// If the product already exists, return the existing item.
func (r *Repository) Add(
	ctx context.Context,
	userID string,
	productID string,
) (*WishlistItem, error) {
	const query = `
		INSERT INTO wishlists (user_id, product_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, product_id)
		DO UPDATE SET product_id = EXCLUDED.product_id
		RETURNING id, user_id, product_id, created_at
	`

	var item WishlistItem

	err := r.db.QueryRow(
		ctx,
		query,
		userID,
		productID,
	).Scan(
		&item.ID,
		&item.UserID,
		&item.ProductID,
		&item.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to add wishlist item: %w", err)
	}

	return &item, nil
}

// List returns all saved products for a user.
func (r *Repository) List(
	ctx context.Context,
	userID string,
) ([]WishlistItem, error) {
	const query = `
		SELECT id, user_id, product_id, created_at
		FROM wishlists
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get wishlist: %w", err)
	}
	defer rows.Close()

	items := make([]WishlistItem, 0)

	for rows.Next() {
		var item WishlistItem

		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.ProductID,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan wishlist item: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read wishlist rows: %w", err)
	}

	return items, nil
}

// Remove deletes a saved product from the user's wishlist.
func (r *Repository) Remove(
	ctx context.Context,
	userID string,
	productID string,
) error {
	const query = `
		DELETE FROM wishlists
		WHERE user_id = $1 AND product_id = $2
	`

	_, err := r.db.Exec(ctx, query, userID, productID)
	if err != nil {
		return fmt.Errorf("failed to remove wishlist item: %w", err)
	}

	return nil
}
