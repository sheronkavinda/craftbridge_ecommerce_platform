
package cart

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetCart returns current product prices, not browser prices.
func (r *Repository) GetCart(
	ctx context.Context,
	buyerID int64,
) (*Summary, error) {
	if buyerID <= 0 {
		return nil, ErrInvalidCart
	}

	rows, err := r.db.Query(ctx, `
		SELECT ci.id, p.id, s.id, s.name, p.title,
		       ci.quantity, p.price_cents,
		       COALESCE(i.available_qty, 0),
		       (p.status = 'published'
		        AND s.status = 'active'
		        AND cat.is_active = TRUE)
		FROM cart_items ci
		JOIN carts ca ON ca.id = ci.cart_id
		JOIN products p ON p.id = ci.product_id
		JOIN shops s ON s.id = p.shop_id
		JOIN categories cat ON cat.id = p.category_id
		LEFT JOIN inventory i ON i.product_id = p.id
		WHERE ca.buyer_id = $1
		ORDER BY s.id, p.id
	`, buyerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &Summary{
		Items: make([]Item, 0),
		Currency: "USD",
	}

	for rows.Next() {
		var item Item
		var published bool

		err := rows.Scan(
			&item.ID, &item.ProductID,
			&item.ShopID, &item.ShopName,
			&item.Title, &item.Quantity,
			&item.UnitPriceCents, &item.AvailableQty,
			&published,
		)
		if err != nil {
			return nil, err
		}

		item.IsAvailable = published &&
			item.AvailableQty >= item.Quantity
		item.LineTotalCents = item.UnitPriceCents *
			int64(item.Quantity)
		result.SubtotalCents += item.LineTotalCents
		result.Items = append(result.Items, item)
	}

	return result, rows.Err()
}

// Lock or create the buyer's cart inside a transaction.
func lockedCartID(
	ctx context.Context,
	tx pgx.Tx,
	buyerID int64,
) (int64, error) {
	_, err := tx.Exec(ctx, `
		INSERT INTO carts (buyer_id)
		VALUES ($1) ON CONFLICT (buyer_id) DO NOTHING
	`, buyerID)
	if err != nil {
		return 0, err
	}

	var cartID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM carts
		WHERE buyer_id = $1 FOR UPDATE
	`, buyerID).Scan(&cartID)

	return cartID, err
}

// AddItem adds or increases an item in the persistent cart.
func (r *Repository) AddItem(
	ctx context.Context,
	buyerID int64,
	req AddItemRequest,
) error {
	if buyerID <= 0 || req.ProductID <= 0 ||
		req.Quantity < 1 || req.Quantity > 99 {
		return ErrInvalidCart
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	cartID, err := lockedCartID(ctx, tx, buyerID)
	if err != nil {
		return err
	}

	var available int32
	err = tx.QueryRow(ctx, `
		SELECT i.available_qty
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		JOIN categories cat ON cat.id = p.category_id
		JOIN inventory i ON i.product_id = p.id
		WHERE p.id = $1
		  AND p.status = 'published'
		  AND s.status = 'active'
		  AND cat.is_active = TRUE
	`, req.ProductID).Scan(&available)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnavailable
	}
	if err != nil {
		return err
	}

	var quantity int32
	err = tx.QueryRow(ctx, `
		INSERT INTO cart_items (
			cart_id, product_id, quantity
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (cart_id, product_id)
		DO UPDATE SET
			quantity = cart_items.quantity + EXCLUDED.quantity,
			updated_at = NOW()
		WHERE cart_items.quantity + EXCLUDED.quantity <= $4
		RETURNING quantity
	`, cartID, req.ProductID, req.Quantity,
		minInt(available, 99)).Scan(&quantity)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnavailable
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE carts SET updated_at = NOW() WHERE id = $1
	`, cartID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func minInt(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// UpdateItem enforces buyer ownership and stock validation.
func (r *Repository) UpdateItem(
	ctx context.Context,
	buyerID, itemID int64,
	quantity int32,
) error {
	if buyerID <= 0 || itemID <= 0 ||
		quantity < 1 || quantity > 99 {
		return ErrInvalidCart
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	cartID, err := lockedCartID(ctx, tx, buyerID)
	if err != nil {
		return err
	}

	var productID int64
	err = tx.QueryRow(ctx, `
		SELECT product_id FROM cart_items
		WHERE id = $1 AND cart_id = $2
	`, itemID, cartID).Scan(&productID)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCartItemNotFound
	}
	if err != nil {
		return err
	}

	var available int32
	err = tx.QueryRow(ctx, `
		SELECT i.available_qty
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		JOIN categories cat ON cat.id = p.category_id
		JOIN inventory i ON i.product_id = p.id
		WHERE p.id = $1
		  AND p.status = 'published'
		  AND s.status = 'active'
		  AND cat.is_active = TRUE
	`, productID).Scan(&available)

	if errors.Is(err, pgx.ErrNoRows) || quantity > available {
		return ErrUnavailable
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE cart_items
		SET quantity = $3, updated_at = NOW()
		WHERE id = $1 AND cart_id = $2
	`, itemID, cartID, quantity)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RemoveItem removes an item from this buyer's cart only.
func (r *Repository) RemoveItem(
	ctx context.Context,
	buyerID, itemID int64,
) error {
	if buyerID <= 0 || itemID <= 0 {
		return ErrInvalidCart
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	cartID, err := lockedCartID(ctx, tx, buyerID)
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
		DELETE FROM cart_items
		WHERE id = $1 AND cart_id = $2
	`, itemID, cartID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCartItemNotFound
	}

	return tx.Commit(ctx)
}
