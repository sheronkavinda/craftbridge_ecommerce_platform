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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrOrderNotFound indicates that the requested order does not exist or does not belong to the caller.
	ErrOrderNotFound = errors.New("order not found")
	// ErrAttemptNotFound indicates that the requested payment attempt does not exist.
	ErrAttemptNotFound = errors.New("payment attempt not found")
)

type ctxKeyRequestID struct{}
type ctxKeyPgxTx struct{}

// TxRunner executes a function inside a single database transaction.
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

type dbQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

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

func (r *Repository) querier(ctx context.Context) dbQuerier {
	if tx, ok := ctx.Value(ctxKeyPgxTx{}).(pgx.Tx); ok && tx != nil {
		return tx
	}
	return r.db
}

// RunInTx starts a pgx transaction, attaches it to txCtx, and commits if fn returns nil.
func (r *Repository) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		log.Printf("[req_id=%s] payment.Repository.RunInTx begin error: %v", reqIDFromContext(ctx), err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txCtx := context.WithValue(ctx, ctxKeyPgxTx{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("[req_id=%s] payment.Repository.RunInTx commit error: %v", reqIDFromContext(ctx), err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
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

	q := r.querier(ctx)
	err := q.QueryRow(ctx, query, orderID).Scan(
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
		userErr := q.QueryRow(
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

// GetAttemptByID loads a single payment_attempts record by its ID.
func (r *Repository) GetAttemptByID(ctx context.Context, attemptID string) (*PaymentAttempt, error) {
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
		WHERE id::text = $1
	`

	var attempt PaymentAttempt
	err := r.querier(ctx).QueryRow(ctx, query, attemptID).Scan(
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
			return nil, ErrAttemptNotFound
		}
		log.Printf("[req_id=%s] payment.Repository.GetAttemptByID error (attempt_id=%s): %v", reqIDFromContext(ctx), attemptID, err)
		return nil, fmt.Errorf("failed to query payment attempt: %w", err)
	}

	return &attempt, nil
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
	err := r.querier(ctx).QueryRow(ctx, query, orderID).Scan(
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
	err := r.querier(ctx).QueryRow(
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

// UpdateAttemptStatus updates a payment attempt's status, provider_reference, failure_reason, and completed_at inside a transaction.
func (r *Repository) UpdateAttemptStatus(
	ctx context.Context,
	attemptID string,
	status PaymentStatus,
	providerRef *string,
	failureReason *string,
	completedAt *time.Time,
) (*PaymentAttempt, error) {
	const query = `
		UPDATE payment_attempts
		SET
			status = $2,
			provider_reference = COALESCE($3, provider_reference),
			failure_reason = COALESCE($4, failure_reason),
			completed_at = COALESCE($5, completed_at),
			updated_at = NOW()
		WHERE id::text = $1
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

	var updated PaymentAttempt
	err := r.querier(ctx).QueryRow(
		ctx,
		query,
		attemptID,
		string(status),
		providerRef,
		failureReason,
		completedAt,
	).Scan(
		&updated.ID,
		&updated.OrderID,
		&updated.UserID,
		&updated.Provider,
		&updated.ProviderReference,
		&updated.AmountCents,
		&updated.Currency,
		&updated.Status,
		&updated.FailureReason,
		&updated.CreatedAt,
		&updated.UpdatedAt,
		&updated.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttemptNotFound
		}
		log.Printf("[req_id=%s] payment.Repository.UpdateAttemptStatus error (attempt_id=%s): %v", reqIDFromContext(ctx), attemptID, err)
		return nil, fmt.Errorf("failed to update payment attempt status: %w", err)
	}

	return &updated, nil
}

// InsertEventIfNew inserts a payment_events record using ON CONFLICT (provider, provider_event_id) DO NOTHING.
// If the event is new, it returns inserted=true and existingProcessedAt=nil.
// If the event already exists, it returns inserted=false and the existing row's processed_at value.
func (r *Repository) InsertEventIfNew(ctx context.Context, event PaymentEvent) (bool, *time.Time, error) {
	payload := event.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	const insertQuery = `
		INSERT INTO payment_events (
			id,
			provider,
			provider_event_id,
			attempt_id,
			event_type,
			payload,
			signature_valid,
			processed_at
		)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
		RETURNING id::text
	`

	q := r.querier(ctx)
	var insertedID string
	err := q.QueryRow(
		ctx,
		insertQuery,
		event.ID,
		event.Provider,
		event.ProviderEventID,
		event.AttemptID,
		event.EventType,
		string(payload),
		event.SignatureValid,
		event.ProcessedAt,
	).Scan(&insertedID)
	if err == nil {
		return true, nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("[req_id=%s] payment.Repository.InsertEventIfNew insert error (provider=%s, event_id=%s): %v", reqIDFromContext(ctx), event.Provider, event.ProviderEventID, err)
		return false, nil, fmt.Errorf("failed to insert payment event: %w", err)
	}

	const selectExistingQuery = `
		SELECT processed_at
		FROM payment_events
		WHERE provider = $1 AND provider_event_id = $2
	`
	var existingProcessedAt *time.Time
	selErr := q.QueryRow(ctx, selectExistingQuery, event.Provider, event.ProviderEventID).Scan(&existingProcessedAt)
	if selErr != nil {
		log.Printf("[req_id=%s] payment.Repository.InsertEventIfNew select existing error (provider=%s, event_id=%s): %v", reqIDFromContext(ctx), event.Provider, event.ProviderEventID, selErr)
		return false, nil, fmt.Errorf("failed to query existing payment event: %w", selErr)
	}

	return false, existingProcessedAt, nil
}

// MarkEventProcessed sets processed_at on a stored payment_events record.
func (r *Repository) MarkEventProcessed(ctx context.Context, provider, providerEventID string, processedAt time.Time) error {
	const query = `
		UPDATE payment_events
		SET processed_at = $3
		WHERE provider = $1 AND provider_event_id = $2
	`
	_, err := r.querier(ctx).Exec(ctx, query, provider, providerEventID, processedAt)
	if err != nil {
		log.Printf("[req_id=%s] payment.Repository.MarkEventProcessed error (provider=%s, event_id=%s): %v", reqIDFromContext(ctx), provider, providerEventID, err)
		return fmt.Errorf("failed to mark payment event processed: %w", err)
	}
	return nil
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
