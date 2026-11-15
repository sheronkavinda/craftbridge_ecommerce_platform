package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrOrderNotFound indicates that the requested order does not exist or does not belong to the caller.
	ErrOrderNotFound = errors.New("order not found")
	// ErrAttemptNotFound indicates that the requested payment attempt does not exist.
	ErrAttemptNotFound = errors.New("payment attempt not found")
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

// Repository executes parameterized SQL queries for payment attempts, payment events, and order lookup.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new payment repository backed by a pgx connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetOrderForPayment loads an order's payment-relevant fields from the orders table.
// Because Member 04 owns the orders schema (migration 030) while local_dev_bootstrap.sql defines
// a minimal placeholder orders table, this query inspects to_jsonb(o) so it works seamlessly with both
// full orders schemas (user_id/buyer_id, total_cents/total_amount/amount, currency, status, order_number)
// and the local bootstrap schema.
func (r *Repository) GetOrderForPayment(ctx context.Context, orderID string) (OrderForPayment, error) {
	const query = `
		SELECT
			o.id::text,
			to_jsonb(o) AS order_json,
			COALESCE(
				(SELECT p.amount::float8 FROM payments p WHERE p.order_id::text = o.id::text ORDER BY p.created_at DESC LIMIT 1),
				0
			) AS legacy_payment_amount,
			COALESCE(
				(SELECT p.status FROM payments p WHERE p.order_id::text = o.id::text ORDER BY p.created_at DESC LIMIT 1),
				''
			) AS legacy_payment_status,
			COALESCE(
				(SELECT u.id::text FROM users u WHERE UPPER(u.role) = 'BUYER' ORDER BY u.created_at ASC LIMIT 1),
				'buyer-001'
			) AS fallback_buyer_id
		FROM orders o
		WHERE o.id::text = $1
	`

	var (
		resolvedID          string
		rawJSON             []byte
		legacyPaymentAmount float64
		legacyPaymentStatus string
		fallbackBuyerID     string
	)

	err := r.db.QueryRow(ctx, query, orderID).Scan(
		&resolvedID,
		&rawJSON,
		&legacyPaymentAmount,
		&legacyPaymentStatus,
		&fallbackBuyerID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderForPayment{}, ErrOrderNotFound
		}
		log.Printf("[req_id=%s] payment.Repository.GetOrderForPayment error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return OrderForPayment{}, fmt.Errorf("failed to query order for payment: %w", err)
	}

	var fields map[string]any
	if len(rawJSON) > 0 {
		_ = json.Unmarshal(rawJSON, &fields)
	}

	userID := extractStringField(fields, "user_id", "buyer_id", "customer_id")
	if userID == "" {
		userID = fallbackBuyerID
	}

	orderNumber := extractStringField(fields, "order_number", "number")
	if orderNumber == "" {
		orderNumber = resolvedID
	}

	currency := strings.ToUpper(extractStringField(fields, "currency"))
	if currency == "" {
		currency = "USD"
	}

	status := strings.ToUpper(extractStringField(fields, "status", "payment_status"))
	if status == "" {
		if legacyPaymentStatus != "" {
			status = strings.ToUpper(strings.TrimSpace(legacyPaymentStatus))
		} else {
			status = "PENDING"
		}
	}

	amountCents := extractAmountCents(fields, legacyPaymentAmount)

	order := OrderForPayment{
		OrderID:     resolvedID,
		OrderNumber: orderNumber,
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    currency,
		Status:      status,
	}

	if userID != "" {
		var buyerName, buyerEmail string
		userErr := r.db.QueryRow(
			ctx,
			`SELECT COALESCE(name, ''), COALESCE(email, '') FROM users WHERE id::text = $1`,
			userID,
		).Scan(&buyerName, &buyerEmail)
		if userErr == nil {
			order.BuyerName = buyerName
			order.BuyerEmail = buyerEmail
		}
	}

	return order, nil
}

// GetLatestPendingAttemptByOrder returns the most recent PENDING payment attempt for the given order, or nil if none exists.
func (r *Repository) GetLatestPendingAttemptByOrder(ctx context.Context, orderID string) (*PaymentAttempt, error) {
	const query = `
		SELECT
			id::text,
			order_id::text,
			user_id::text,
			provider,
			provider_reference,
			amount_cents,
			currency,
			status,
			failure_reason,
			created_at,
			updated_at,
			completed_at
		FROM payment_attempts
		WHERE order_id::text = $1
		  AND status = 'PENDING'
		ORDER BY created_at DESC
		LIMIT 1
	`

	var attempt PaymentAttempt
	err := r.db.QueryRow(ctx, query, orderID).Scan(
		&attempt.ID,
		&attempt.OrderID,
		&attempt.UserID,
		&attempt.Provider,
		&attempt.ProviderReference,
		&attempt.AmountCents,
		&attempt.Currency,
		&attempt.Status,
		&attempt.FailureReason,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
		&attempt.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		log.Printf("[req_id=%s] payment.Repository.GetLatestPendingAttemptByOrder error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return nil, fmt.Errorf("failed to query pending payment attempt: %w", err)
	}

	return &attempt, nil
}

// CreateAttempt inserts a new payment_attempts record and returns the stored row.
func (r *Repository) CreateAttempt(ctx context.Context, attempt PaymentAttempt) (*PaymentAttempt, error) {
	const query = `
		INSERT INTO payment_attempts (
			id,
			order_id,
			user_id,
			provider,
			provider_reference,
			amount_cents,
			currency,
			status,
			failure_reason
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING
			id::text,
			order_id::text,
			user_id::text,
			provider,
			provider_reference,
			amount_cents,
			currency,
			status,
			failure_reason,
			created_at,
			updated_at,
			completed_at
	`

	var created PaymentAttempt
	err := r.db.QueryRow(
		ctx,
		query,
		attempt.ID,
		attempt.OrderID,
		attempt.UserID,
		attempt.Provider,
		attempt.ProviderReference,
		attempt.AmountCents,
		attempt.Currency,
		string(attempt.Status),
		attempt.FailureReason,
	).Scan(
		&created.ID,
		&created.OrderID,
		&created.UserID,
		&created.Provider,
		&created.ProviderReference,
		&created.AmountCents,
		&created.Currency,
		&created.Status,
		&created.FailureReason,
		&created.CreatedAt,
		&created.UpdatedAt,
		&created.CompletedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] payment.Repository.CreateAttempt error (id=%s, order_id=%s): %v", reqIDFromContext(ctx), attempt.ID, attempt.OrderID, err)
		return nil, fmt.Errorf("failed to create payment attempt: %w", err)
	}

	return &created, nil
}

func extractStringField(fields map[string]any, keys ...string) string {
	for _, k := range keys {
		if raw, ok := fields[k]; ok && raw != nil {
			switch v := raw.(type) {
			case string:
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func extractAmountCents(fields map[string]any, fallbackDecimalAmount float64) int64 {
	for _, centKey := range []string{"total_cents", "amount_cents", "grand_total_cents"} {
		if raw, ok := fields[centKey]; ok && raw != nil {
			if cents, ok := toInt64(raw); ok && cents > 0 {
				return cents
			}
		}
	}
	for _, decKey := range []string{"total_amount", "amount", "total", "grand_total"} {
		if raw, ok := fields[decKey]; ok && raw != nil {
			if dec, ok := toFloat64(raw); ok && dec > 0 {
				return int64(math.Round(dec * 100))
			}
		}
	}
	if fallbackDecimalAmount > 0 {
		return int64(math.Round(fallbackDecimalAmount * 100))
	}
	return 0
}

func toInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(math.Round(val)), true
	case int64:
		return val, true
	case int:
		return int64(val), true
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(val), 10, 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case int:
		return float64(val), true
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
