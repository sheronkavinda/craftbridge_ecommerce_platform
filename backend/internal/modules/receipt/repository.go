package receipt

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ctxKeyRequestID struct{}

// WithRequestID attaches a request ID to a context for structured repository error logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, requestID)
}

func reqIDFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(ctxKeyRequestID{}).(string); ok && val != "" {
		return val
	}
	return "unknown"
}

// Repository executes parameterized SQL queries for the receipts table and completed refund aggregation.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new receipt repository backed by a pgx connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetOrCreateByOrder lazily and idempotently issues a receipt for an order using
// INSERT ... ON CONFLICT (order_id) DO NOTHING followed by a SELECT on order_id::text = $1.
func (r *Repository) GetOrCreateByOrder(ctx context.Context, candidate Receipt) (*Receipt, error) {
	const insertSQL = `
		INSERT INTO receipts (
			id,
			order_id,
			payment_attempt_id,
			user_id,
			receipt_number,
			amount_cents,
			currency,
			issued_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (order_id) DO NOTHING
	`

	_, err := r.db.Exec(
		ctx,
		insertSQL,
		candidate.ID,
		candidate.OrderID,
		candidate.PaymentAttemptID,
		candidate.UserID,
		candidate.ReceiptNumber,
		candidate.AmountCents,
		candidate.Currency,
		candidate.IssuedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] receipt.Repository.GetOrCreateByOrder insert error (order_id=%s): %v", reqIDFromContext(ctx), candidate.OrderID, err)
		return nil, fmt.Errorf("failed to insert receipt: %w", err)
	}

	const selectSQL = `
		SELECT
			id::text,
			order_id::text,
			payment_attempt_id::text,
			user_id::text,
			receipt_number,
			amount_cents,
			currency,
			issued_at,
			created_at
		FROM receipts
		WHERE order_id::text = $1
	`

	var stored Receipt
	err = r.db.QueryRow(ctx, selectSQL, candidate.OrderID).Scan(
		&stored.ID,
		&stored.OrderID,
		&stored.PaymentAttemptID,
		&stored.UserID,
		&stored.ReceiptNumber,
		&stored.AmountCents,
		&stored.Currency,
		&stored.IssuedAt,
		&stored.CreatedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] receipt.Repository.GetOrCreateByOrder select error (order_id=%s): %v", reqIDFromContext(ctx), candidate.OrderID, err)
		return nil, fmt.Errorf("failed to select receipt: %w", err)
	}

	return &stored, nil
}

// SumCompletedRefundsForOrder returns the total amount_cents of COMPLETED refunds for the given order.
func (r *Repository) SumCompletedRefundsForOrder(ctx context.Context, orderID string) (int64, error) {
	const query = `
		SELECT COALESCE(
			SUM(COALESCE(NULLIF(amount_cents, 0), ROUND(COALESCE(amount, 0) * 100)::bigint)),
			0
		)::bigint
		FROM refunds
		WHERE order_id::text = $1
		  AND status = 'COMPLETED'
	`

	var refundedCents int64
	if err := r.db.QueryRow(ctx, query, orderID).Scan(&refundedCents); err != nil {
		log.Printf("[req_id=%s] receipt.Repository.SumCompletedRefundsForOrder error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return 0, fmt.Errorf("failed to sum completed refunds for receipt: %w", err)
	}

	return refundedCents, nil
}
