
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrInvalidStock = errors.New("invalid stock quantity")
	ErrReservationUnavailable = errors.New("stock reservation unavailable")
)

type InventoryAdjustmentRequest struct {
	Delta  int32  `json:"delta" binding:"required"`
	Reason string `json:"reason" binding:"required"`
}

// GetInventory retrieves stock for a seller-owned product.
func (r *Repository) GetInventory(
	ctx context.Context,
	userID, productID int64,
) (*Inventory, error) {

	var inv Inventory

	err := r.db.QueryRow(ctx, `
		SELECT i.product_id, i.available_qty,
		       i.reserved_qty, i.updated_at
		FROM inventory i
		JOIN products p ON p.id = i.product_id
		JOIN shops s ON s.id = p.shop_id
		WHERE i.product_id = $1
		  AND s.owner_user_id = $2
	`, productID, userID).Scan(
		&inv.ProductID,
		&inv.AvailableQty,
		&inv.ReservedQty,
		&inv.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}

	return &inv, err
}

// AdjustInventory updates available stock and records
// the movement in the same PostgreSQL transaction.
func (r *Repository) AdjustInventory(
	ctx context.Context,
	userID, productID int64,
	req InventoryAdjustmentRequest,
) (*Inventory, error) {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var current int32

	err = tx.QueryRow(ctx, `
		SELECT i.available_qty
		FROM inventory i
		JOIN products p ON p.id = i.product_id
		JOIN shops s ON s.id = p.shop_id
		WHERE i.product_id = $1
		  AND s.owner_user_id = $2
		  AND s.status = 'active'
		  AND p.status <> 'archived'
		FOR UPDATE OF i
	`, productID, userID).Scan(&current)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}

	newQuantity := int64(current) + int64(req.Delta)

	if newQuantity < 0 || newQuantity > 2147483647 {
		return nil, ErrInsufficientStock
	}

	var inv Inventory

	err = tx.QueryRow(ctx, `
		UPDATE inventory
		SET available_qty = $2,
		    updated_at = NOW()
		WHERE product_id = $1
		RETURNING product_id, available_qty,
		          reserved_qty, updated_at
	`, productID, newQuantity).Scan(
		&inv.ProductID,
		&inv.AvailableQty,
		&inv.ReservedQty,
		&inv.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements (
			product_id, available_delta,
			reserved_delta, reason, actor_user_id
		)
		VALUES ($1, $2, 0, $3, $4)
	`, productID, req.Delta, req.Reason, userID)

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &inv, nil
}

// ListInventoryMovements returns the last 50
// stock movements for the seller's product.
func (r *Repository) ListInventoryMovements(
	ctx context.Context,
	userID, productID int64,
) ([]InventoryMovement, error) {

	_, err := r.GetInventory(ctx, userID, productID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, product_id, available_delta,
		       reserved_delta, reason, actor_user_id,
		       created_at
		FROM inventory_movements
		WHERE product_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 50
	`, productID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]InventoryMovement, 0)

	for rows.Next() {
		var m InventoryMovement

		err := rows.Scan(
			&m.ID, &m.ProductID,
			&m.AvailableDelta, &m.ReservedDelta,
			&m.Reason, &m.ActorUserID, &m.CreatedAt,
		)

		if err != nil {
			return nil, err
		}

		result = append(result, m)
	}

	return result, rows.Err()
}

// ReserveStock is called by the Orders module INSIDE
// its existing checkout transaction.
//
// Available stock decreases; reserved stock increases.
func (r *Repository) ReserveStock(
	ctx context.Context,
	tx pgx.Tx,
	productID int64,
	quantity int32,
) error {

	if tx == nil || productID <= 0 || quantity <= 0 {
		return ErrInvalidStock
	}

	var id int64

	err := tx.QueryRow(ctx, `
		UPDATE inventory AS i
		SET available_qty = i.available_qty - $2,
		    reserved_qty = i.reserved_qty + $2,
		    updated_at = NOW()
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		WHERE i.product_id = $1
		  AND p.id = i.product_id
		  AND p.status = 'published'
		  AND s.status = 'active'
		  AND i.available_qty >= $2
		RETURNING i.product_id
	`, productID, quantity).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInsufficientStock
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements (
			product_id, available_delta,
			reserved_delta, reason
		)
		VALUES ($1, $2, $3, 'checkout_reserve')
	`, productID, -quantity, quantity)

	return err
}

// ReleaseStock releases reserved stock when an order
// expires or is cancelled before payment.
func (r *Repository) ReleaseStock(
	ctx context.Context,
	tx pgx.Tx,
	productID int64,
	quantity int32,
) error {

	if tx == nil || productID <= 0 || quantity <= 0 {
		return ErrInvalidStock
	}

	var id int64

	err := tx.QueryRow(ctx, `
		UPDATE inventory
		SET available_qty = available_qty + $2,
		    reserved_qty = reserved_qty - $2,
		    updated_at = NOW()
		WHERE product_id = $1
		  AND reserved_qty >= $2
		RETURNING product_id
	`, productID, quantity).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReservationUnavailable
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements (
			product_id, available_delta,
			reserved_delta, reason
		)
		VALUES ($1, $2, $3, 'checkout_release')
	`, productID, quantity, -quantity)

	return err
}

// CommitReservedStock finalizes a reservation after
// the payment module verifies successful payment.
func (r *Repository) CommitReservedStock(
	ctx context.Context,
	tx pgx.Tx,
	productID int64,
	quantity int32,
) error {

	if tx == nil || productID <= 0 || quantity <= 0 {
		return ErrInvalidStock
	}

	var id int64

	err := tx.QueryRow(ctx, `
		UPDATE inventory
		SET reserved_qty = reserved_qty - $2,
		    updated_at = NOW()
		WHERE product_id = $1
		  AND reserved_qty >= $2
		RETURNING product_id
	`, productID, quantity).Scan(&id)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrReservationUnavailable
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory_movements (
			product_id, available_delta,
			reserved_delta, reason
		)
		VALUES ($1, 0, $2, 'order_paid')
	`, productID, -quantity)

	return err
}

// Service methods for seller inventory management.

func (s *Service) GetInventory(
	ctx context.Context,
	userID, productID int64,
) (*Inventory, error) {

	if userID <= 0 || productID <= 0 {
		return nil, ErrInvalidStock
	}

	return s.repo.GetInventory(ctx, userID, productID)
}

func (s *Service) AdjustInventory(
	ctx context.Context,
	userID, productID int64,
	req InventoryAdjustmentRequest,
) (*Inventory, error) {

	req.Reason = strings.TrimSpace(req.Reason)

	if userID <= 0 || productID <= 0 ||
		req.Delta == 0 ||
		req.Delta > 100000 || req.Delta < -100000 ||
		utf8.RuneCountInString(req.Reason) < 3 ||
		utf8.RuneCountInString(req.Reason) > 200 {
		return nil, ErrInvalidStock
	}

	return s.repo.AdjustInventory(
		ctx, userID, productID, req,
	)
}

func (s *Service) ListInventoryMovements(
	ctx context.Context,
	userID, productID int64,
) ([]InventoryMovement, error) {

	if userID <= 0 || productID <= 0 {
		return nil, ErrInvalidStock
	}

	return s.repo.ListInventoryMovements(
		ctx, userID, productID,
	)
}

// Trusted internal methods for Shalinda's Orders module.
// The caller MUST control the transaction and idempotency.

func (s *Service) ReserveForCheckout(
	ctx context.Context,
	tx pgx.Tx,
	productID int64,
	quantity int32,
) error {
	if err := s.repo.ReserveStock(ctx, tx, productID, quantity); err != nil {
		return fmt.Errorf("reserve product %d: %w", productID, err)
	}
	return nil
}

func (s *Service) ReleaseCheckoutReservation(
	ctx context.Context,
	tx pgx.Tx,
	productID int64,
	quantity int32,
) error {
	return s.repo.ReleaseStock(ctx, tx, productID, quantity)
}

func (s *Service) ConfirmCheckoutReservation(
	ctx context.Context,
	tx pgx.Tx,
	productID int64,
	quantity int32,
) error {
	return s.repo.CommitReservedStock(ctx, tx, productID, quantity)
}
