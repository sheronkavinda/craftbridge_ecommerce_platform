package order

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

// Find an existing checkout using its idempotency key.
func (r *Repository) FindByKey(
	ctx context.Context,
	buyerID int64,
	key string,
) (int64, int64, error) {
	var id, addressID int64

	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			address_id
		FROM orders
		WHERE buyer_id = $1
		  AND idempotency_key = $2
	`, buyerID, key).Scan(
		&id,
		&addressID,
	)

	return id, addressID, err
}

// Retrieve order history for the authenticated buyer.
func (r *Repository) ListBuyerOrders(
	ctx context.Context,
	buyerID int64,
) ([]OrderSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			id,
			grand_total_cents,
			payment_status,
			status,
			created_at
		FROM orders
		WHERE buyer_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 100
	`, buyerID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]OrderSummary, 0)

	for rows.Next() {
		var item OrderSummary

		err := rows.Scan(
			&item.ID,
			&item.GrandTotal,
			&item.PaymentStatus,
			&item.Status,
			&item.CreatedAt,
		)

		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// Get an order belonging to the authenticated buyer.
func (r *Repository) GetBuyerOrder(
	ctx context.Context,
	buyerID, orderID int64,
) (*Order, error) {
	var o Order
	var addressJSON string

	err := r.db.QueryRow(ctx, `
		SELECT
			id,
			buyer_id,
			address_id,
			address_snapshot::text,
			subtotal_cents,
			shipping_total_cents,
			grand_total_cents,
			currency,
			payment_status,
			status,
			expires_at,
			created_at
		FROM orders
		WHERE id = $1
		  AND buyer_id = $2
	`, orderID, buyerID).Scan(
		&o.ID,
		&o.BuyerID,
		&o.AddressID,
		&addressJSON,
		&o.SubtotalCents,
		&o.ShippingTotalCents,
		&o.GrandTotalCents,
		&o.Currency,
		&o.PaymentStatus,
		&o.Status,
		&o.ExpiresAt,
		&o.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOrderNotFound
	}

	if err != nil {
		return nil, err
	}

	o.AddressSnapshot = json.RawMessage(addressJSON)
	o.SellerOrders = make([]SellerOrder, 0)

	// Retrieve seller-specific suborders.
	rows, err := r.db.Query(ctx, `
		SELECT
			so.id,
			so.shop_id,
			sh.name,
			so.subtotal_cents,
			so.shipping_cents,
			so.status
		FROM seller_orders so
		JOIN shops sh ON sh.id = so.shop_id
		WHERE so.order_id = $1
		ORDER BY so.id
	`, orderID)

	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var so SellerOrder

		err := rows.Scan(
			&so.ID,
			&so.ShopID,
			&so.ShopName,
			&so.SubtotalCents,
			&so.ShippingCents,
			&so.Status,
		)

		if err != nil {
			rows.Close()
			return nil, err
		}

		so.Items = make([]OrderItem, 0)

		o.SellerOrders = append(
			o.SellerOrders,
			so,
		)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}

	rows.Close()

	// Retrieve products in each seller suborder.
	for i := range o.SellerOrders {
		so := &o.SellerOrders[i]

		itemRows, err := r.db.Query(ctx, `
			SELECT
				id,
				product_id,
				product_title_snapshot,
				unit_price_cents,
				quantity,
				line_total_cents
			FROM order_items
			WHERE seller_order_id = $1
			ORDER BY id
		`, so.ID)

		if err != nil {
			return nil, err
		}

		for itemRows.Next() {
			var item OrderItem

			err := itemRows.Scan(
				&item.ID,
				&item.ProductID,
				&item.Title,
				&item.UnitPriceCents,
				&item.Quantity,
				&item.LineTotalCents,
			)

			if err != nil {
				itemRows.Close()
				return nil, err
			}

			so.Items = append(
				so.Items,
				item,
			)
		}

		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			return nil, err
		}

		itemRows.Close()
	}

	return &o, nil
}
