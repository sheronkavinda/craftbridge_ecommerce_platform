package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

// GetOrderForPayment loads an order's payment-relevant fields from Member 04's orders table
// (and buyer metadata from users).
//
// Multi-seller SellerID rule:
//   - If the order has exactly one seller_order, SellerID is set to that shop's owner_user_id::text
//     so single-seller refunds can attribute the refund to that seller.
//   - If the order has 0 or >1 seller_orders (a multi-seller basket), SellerID is left empty ("")
//     so refunds do not misattribute a multi-seller order to a single seller.
func (r *Repository) GetOrderForPayment(ctx context.Context, orderID string) (OrderForPayment, error) {
	const query = `
		SELECT
			o.id::text,
			o.buyer_id::text,
			o.grand_total_cents,
			UPPER(TRIM(o.currency)),
			o.status,
			o.payment_status,
			o.expires_at,
			COALESCE(u.name, ''),
			COALESCE(u.email, ''),
			CASE
				WHEN (SELECT COUNT(*) FROM seller_orders so WHERE so.order_id = o.id) = 1
				THEN COALESCE((
					SELECT sh.owner_user_id::text
					FROM seller_orders so
					JOIN shops sh ON sh.id = so.shop_id
					WHERE so.order_id = o.id
					LIMIT 1
				), '')
				ELSE ''
			END AS seller_id
		FROM orders o
		LEFT JOIN users u ON u.id = o.buyer_id
		WHERE o.id::text = $1
	`

	var (
		resolvedID    string
		buyerID       string
		amountCents   int64
		currency      string
		status        string
		paymentStatus string
		expiresAt     time.Time
		buyerName     string
		buyerEmail    string
		sellerID      string
	)

	err := r.querier(ctx).QueryRow(ctx, query, orderID).Scan(
		&resolvedID,
		&buyerID,
		&amountCents,
		&currency,
		&status,
		&paymentStatus,
		&expiresAt,
		&buyerName,
		&buyerEmail,
		&sellerID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return OrderForPayment{}, ErrOrderNotFound
		}
		log.Printf("[req_id=%s] payment.Repository.GetOrderForPayment error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return OrderForPayment{}, fmt.Errorf("failed to query order for payment: %w", err)
	}

	if currency == "" {
		currency = "USD"
	}

	expiresAtCopy := expiresAt
	return OrderForPayment{
		OrderID:       resolvedID,
		OrderNumber:   resolvedID,
		UserID:        buyerID,
		SellerID:      sellerID,
		AmountCents:   amountCents,
		Currency:      currency,
		Status:        strings.ToUpper(strings.TrimSpace(status)),
		PaymentStatus: strings.ToUpper(strings.TrimSpace(paymentStatus)),
		ExpiresAt:     &expiresAtCopy,
		BuyerName:     buyerName,
		BuyerEmail:    buyerEmail,
	}, nil
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

// GetLatestSuccessfulAttemptByOrder returns the most recent SUCCESS payment attempt for the given order, or nil if none exists.
func (r *Repository) GetLatestSuccessfulAttemptByOrder(ctx context.Context, orderID string) (*PaymentAttempt, error) {
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
		  AND status = 'SUCCESS'
		ORDER BY completed_at DESC NULLS LAST, created_at DESC
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
		log.Printf("[req_id=%s] payment.Repository.GetLatestSuccessfulAttemptByOrder error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return nil, fmt.Errorf("failed to query successful payment attempt: %w", err)
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
// When transitioning to SUCCESS, failure_reason is explicitly cleared (set to NULL).
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
			failure_reason = CASE WHEN $2 = 'SUCCESS' THEN NULL ELSE COALESCE($4, failure_reason) END,
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

// ClaimPendingAttemptsForReconcile selects PENDING payment attempts created at or before olderThan
// using SELECT ... FOR UPDATE SKIP LOCKED inside a transaction so concurrent workers never claim the same rows.
func (r *Repository) ClaimPendingAttemptsForReconcile(ctx context.Context, olderThan time.Time, limit int) ([]PaymentAttempt, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}

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
		WHERE status = 'PENDING'
		  AND created_at <= $1
		ORDER BY created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, err := r.querier(ctx).Query(ctx, query, olderThan, limit)
	if err != nil {
		log.Printf("[req_id=%s] payment.Repository.ClaimPendingAttemptsForReconcile error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to query pending attempts for reconciliation: %w", err)
	}
	defer rows.Close()

	var attempts []PaymentAttempt
	for rows.Next() {
		var att PaymentAttempt
		if scanErr := rows.Scan(
			&att.ID,
			&att.OrderID,
			&att.UserID,
			&att.Provider,
			&att.ProviderReference,
			&att.AmountCents,
			&att.Currency,
			&att.Status,
			&att.FailureReason,
			&att.CreatedAt,
			&att.UpdatedAt,
			&att.CompletedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan pending attempt for reconciliation: %w", scanErr)
		}
		attempts = append(attempts, att)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pending attempts for reconciliation: %w", err)
	}

	return attempts, nil
}

// ListAttemptsByUser returns a buyer's payment attempts ordered newest first along with the total matching count.
func (r *Repository) ListAttemptsByUser(ctx context.Context, filter ListBuyerPaymentsFilter) ([]PaymentAttempt, int64, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	q := r.querier(ctx)

	var (
		total    int64
		countSQL string
		listSQL  string
		countArg []any
		listArg  []any
	)

	if filter.Status != "" {
		countSQL = `SELECT COUNT(*) FROM payment_attempts WHERE user_id::text = $1 AND status = $2`
		countArg = []any{filter.UserID, string(filter.Status)}

		listSQL = `
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
			WHERE user_id::text = $1
			  AND status = $2
			ORDER BY created_at DESC, id DESC
			LIMIT $3 OFFSET $4
		`
		listArg = []any{filter.UserID, string(filter.Status), pageSize, offset}
	} else {
		countSQL = `SELECT COUNT(*) FROM payment_attempts WHERE user_id::text = $1`
		countArg = []any{filter.UserID}

		listSQL = `
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
			WHERE user_id::text = $1
			ORDER BY created_at DESC, id DESC
			LIMIT $2 OFFSET $3
		`
		listArg = []any{filter.UserID, pageSize, offset}
	}

	if err := q.QueryRow(ctx, countSQL, countArg...).Scan(&total); err != nil {
		log.Printf("[req_id=%s] payment.Repository.ListAttemptsByUser count error (user_id=%s): %v", reqIDFromContext(ctx), filter.UserID, err)
		return nil, 0, fmt.Errorf("failed to count buyer payment attempts: %w", err)
	}

	rows, err := q.Query(ctx, listSQL, listArg...)
	if err != nil {
		log.Printf("[req_id=%s] payment.Repository.ListAttemptsByUser query error (user_id=%s): %v", reqIDFromContext(ctx), filter.UserID, err)
		return nil, 0, fmt.Errorf("failed to list buyer payment attempts: %w", err)
	}
	defer rows.Close()

	attempts := make([]PaymentAttempt, 0)
	for rows.Next() {
		var att PaymentAttempt
		if scanErr := rows.Scan(
			&att.ID,
			&att.OrderID,
			&att.UserID,
			&att.Provider,
			&att.ProviderReference,
			&att.AmountCents,
			&att.Currency,
			&att.Status,
			&att.FailureReason,
			&att.CreatedAt,
			&att.UpdatedAt,
			&att.CompletedAt,
		); scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan buyer payment attempt: %w", scanErr)
		}
		attempts = append(attempts, att)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating buyer payment attempts: %w", err)
	}

	return attempts, total, nil
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
