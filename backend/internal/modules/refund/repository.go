package refund

import (
	"context"
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
	// ErrRefundNotFound indicates that the requested refund record does not exist or does not belong to the caller.
	ErrRefundNotFound = errors.New("refund not found")
	// ErrDuplicateIdempotencyKey indicates that a refund with the same idempotency_key already exists.
	ErrDuplicateIdempotencyKey = errors.New("duplicate refund idempotency key")
	// ErrDuplicateProviderRefundRef indicates that another refund already recorded the same provider_refund_reference.
	ErrDuplicateProviderRefundRef = errors.New("duplicate provider refund reference")
	// ErrRefundTransitionConflict indicates that the refund status changed concurrently and could not be transitioned.
	ErrRefundTransitionConflict = errors.New("refund status transition conflict")
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

// Repository executes parameterized SQL queries for the refunds table.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new refund repository backed by a pgx connection pool.
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
		log.Printf("[req_id=%s] refund.Repository.RunInTx begin error: %v", reqIDFromContext(ctx), err)
		return fmt.Errorf("failed to begin refund transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	txCtx := context.WithValue(ctx, ctxKeyPgxTx{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("[req_id=%s] refund.Repository.RunInTx commit error: %v", reqIDFromContext(ctx), err)
		return fmt.Errorf("failed to commit refund transaction: %w", err)
	}
	return nil
}

const refundSelectColumns = `
	id::text,
	order_id::text,
	payment_attempt_id::text,
	COALESCE(requested_by, ''),
	seller_id::text,
	COALESCE(reason, ''),
	COALESCE(details, ''),
	COALESCE(amount, 0)::float8,
	COALESCE(amount_cents, ROUND(COALESCE(amount, 0) * 100)::bigint),
	COALESCE(NULLIF(currency, ''), 'USD'),
	status,
	provider_refund_reference,
	idempotency_key,
	COALESCE(requested_at, created_at),
	reviewed_at,
	COALESCE(decided_at, reviewed_at),
	decided_by,
	completed_at,
	created_at,
	updated_at
`

func scanRefundRow(row pgx.Row) (*Refund, error) {
	var ref Refund
	err := row.Scan(
		&ref.ID,
		&ref.OrderID,
		&ref.PaymentAttemptID,
		&ref.RequestedBy,
		&ref.SellerID,
		&ref.Reason,
		&ref.Details,
		&ref.Amount,
		&ref.AmountCents,
		&ref.Currency,
		&ref.Status,
		&ref.ProviderRefundReference,
		&ref.IdempotencyKey,
		&ref.RequestedAt,
		&ref.ReviewedAt,
		&ref.DecidedAt,
		&ref.DecidedBy,
		&ref.CompletedAt,
		&ref.CreatedAt,
		&ref.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ref, nil
}

// GetByID retrieves a single refund record by ID.
func (r *Repository) GetByID(ctx context.Context, refundID string) (*Refund, error) {
	query := `SELECT ` + refundSelectColumns + ` FROM refunds WHERE id::text = $1`
	ref, err := scanRefundRow(r.querier(ctx).QueryRow(ctx, query, refundID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundNotFound
		}
		log.Printf("[req_id=%s] refund.Repository.GetByID error (id=%s): %v", reqIDFromContext(ctx), refundID, err)
		return nil, fmt.Errorf("failed to query refund: %w", err)
	}
	return ref, nil
}

// GetByIdempotencyKey retrieves an existing refund by idempotency_key, or nil if none exists.
func (r *Repository) GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*Refund, error) {
	trimmed := strings.TrimSpace(idempotencyKey)
	if trimmed == "" {
		return nil, nil
	}
	query := `SELECT ` + refundSelectColumns + ` FROM refunds WHERE idempotency_key = $1`
	ref, err := scanRefundRow(r.querier(ctx).QueryRow(ctx, query, trimmed))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		log.Printf("[req_id=%s] refund.Repository.GetByIdempotencyKey error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to query refund by idempotency key: %w", err)
	}
	return ref, nil
}

// FindIdenticalOpenRefund finds an existing PENDING or APPROVED refund with the same order, buyer, and amount_cents.
func (r *Repository) FindIdenticalOpenRefund(ctx context.Context, orderID, buyerID string, amountCents int64) (*Refund, error) {
	query := `
		SELECT ` + refundSelectColumns + `
		FROM refunds
		WHERE order_id::text = $1
		  AND requested_by = $2
		  AND amount_cents = $3
		  AND status IN ('PENDING', 'APPROVED')
		ORDER BY created_at DESC
		LIMIT 1
	`
	ref, err := scanRefundRow(r.querier(ctx).QueryRow(ctx, query, orderID, buyerID, amountCents))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		log.Printf("[req_id=%s] refund.Repository.FindIdenticalOpenRefund error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return nil, fmt.Errorf("failed to query open refund: %w", err)
	}
	return ref, nil
}

// SumActiveRefundsForOrder sums amount_cents across all refunds for the given order whose status is not REJECTED.
func (r *Repository) SumActiveRefundsForOrder(ctx context.Context, orderID string) (int64, error) {
	const query = `
		SELECT COALESCE(
			SUM(COALESCE(NULLIF(amount_cents, 0), ROUND(COALESCE(amount, 0) * 100)::bigint)),
			0
		)::bigint
		FROM refunds
		WHERE order_id::text = $1
		  AND status <> 'REJECTED'
	`
	var sum int64
	if err := r.querier(ctx).QueryRow(ctx, query, orderID).Scan(&sum); err != nil {
		log.Printf("[req_id=%s] refund.Repository.SumActiveRefundsForOrder error (order_id=%s): %v", reqIDFromContext(ctx), orderID, err)
		return 0, fmt.Errorf("failed to sum active refunds for order: %w", err)
	}
	return sum, nil
}

// Create inserts a new refund record, writing both amount (NUMERIC) and amount_cents (BIGINT) in sync.
func (r *Repository) Create(ctx context.Context, ref Refund) (*Refund, error) {
	amountDecimalStr := FormatCentsDecimal(ref.AmountCents)
	query := `
		INSERT INTO refunds (
			id,
			order_id,
			payment_attempt_id,
			requested_by,
			seller_id,
			reason,
			details,
			amount,
			amount_cents,
			currency,
			status,
			idempotency_key,
			requested_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::numeric(12,2), $9, $10, $11, $12, $13)
		RETURNING ` + refundSelectColumns

	created, err := scanRefundRow(r.querier(ctx).QueryRow(
		ctx,
		query,
		ref.ID,
		ref.OrderID,
		ref.PaymentAttemptID,
		ref.RequestedBy,
		ref.SellerID,
		ref.Reason,
		ref.Details,
		amountDecimalStr,
		ref.AmountCents,
		ref.Currency,
		string(ref.Status),
		ref.IdempotencyKey,
		ref.RequestedAt,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateIdempotencyKey
		}
		log.Printf("[req_id=%s] refund.Repository.Create error (id=%s, order_id=%s): %v", reqIDFromContext(ctx), ref.ID, ref.OrderID, err)
		return nil, fmt.Errorf("failed to create refund: %w", err)
	}
	return created, nil
}

// TransitionPendingToApproved atomically transitions a refund from PENDING to APPROVED,
// setting decided_by, decided_at, and reviewed_at in sync.
// It returns (updatedRefund, true, nil) when a PENDING row was updated, or (nil, false, nil) if no PENDING row matched.
func (r *Repository) TransitionPendingToApproved(
	ctx context.Context,
	refundID string,
	adminID string,
	decidedAt time.Time,
) (*Refund, bool, error) {
	query := `
		UPDATE refunds
		SET
			status = 'APPROVED',
			decided_by = $2,
			decided_at = $3,
			reviewed_at = $3,
			updated_at = $3
		WHERE id::text = $1
		  AND status = 'PENDING'
		RETURNING ` + refundSelectColumns

	updated, err := scanRefundRow(r.querier(ctx).QueryRow(ctx, query, refundID, adminID, decidedAt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		log.Printf("[req_id=%s] refund.Repository.TransitionPendingToApproved error (id=%s): %v", reqIDFromContext(ctx), refundID, err)
		return nil, false, fmt.Errorf("failed to approve pending refund: %w", err)
	}
	return updated, true, nil
}

// UpdateAfterProviderRefund transitions an APPROVED refund to COMPLETED or PROCESSED after a successful provider refund call,
// enforcing uniqueness of provider_refund_reference across all refund records.
func (r *Repository) UpdateAfterProviderRefund(
	ctx context.Context,
	refundID string,
	status RefundStatus,
	providerRef string,
	completedAt *time.Time,
) (*Refund, error) {
	q := r.querier(ctx)
	trimmedRef := strings.TrimSpace(providerRef)

	if trimmedRef != "" {
		var duplicateExists bool
		checkErr := q.QueryRow(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM refunds WHERE provider_refund_reference = $1 AND id::text <> $2)`,
			trimmedRef,
			refundID,
		).Scan(&duplicateExists)
		if checkErr != nil {
			return nil, fmt.Errorf("failed to check provider refund reference uniqueness: %w", checkErr)
		}
		if duplicateExists {
			return nil, ErrDuplicateProviderRefundRef
		}
	}

	var refPtr *string
	if trimmedRef != "" {
		refPtr = &trimmedRef
	}

	query := `
		UPDATE refunds
		SET
			status = $2,
			provider_refund_reference = COALESCE($3, provider_refund_reference),
			completed_at = COALESCE($4, completed_at),
			updated_at = NOW()
		WHERE id::text = $1
		  AND status = 'APPROVED'
		RETURNING ` + refundSelectColumns

	updated, err := scanRefundRow(q.QueryRow(ctx, query, refundID, string(status), refPtr, completedAt))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRefundTransitionConflict
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrDuplicateProviderRefundRef
		}
		log.Printf("[req_id=%s] refund.Repository.UpdateAfterProviderRefund error (id=%s): %v", reqIDFromContext(ctx), refundID, err)
		return nil, fmt.Errorf("failed to finalize refund after provider call: %w", err)
	}
	return updated, nil
}

// ListByBuyer returns a paginated list of a buyer's refunds ordered newest first (by requested_at DESC, id DESC) and total count.
func (r *Repository) ListByBuyer(ctx context.Context, filter ListBuyerRefundsFilter) ([]Refund, int64, error) {
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
		countSQL = `SELECT COUNT(*) FROM refunds WHERE requested_by = $1 AND status = $2`
		countArg = []any{filter.UserID, string(filter.Status)}

		listSQL = `
			SELECT ` + refundSelectColumns + `
			FROM refunds
			WHERE requested_by = $1
			  AND status = $2
			ORDER BY COALESCE(requested_at, created_at) DESC, id DESC
			LIMIT $3 OFFSET $4
		`
		listArg = []any{filter.UserID, string(filter.Status), pageSize, offset}
	} else {
		countSQL = `SELECT COUNT(*) FROM refunds WHERE requested_by = $1`
		countArg = []any{filter.UserID}

		listSQL = `
			SELECT ` + refundSelectColumns + `
			FROM refunds
			WHERE requested_by = $1
			ORDER BY COALESCE(requested_at, created_at) DESC, id DESC
			LIMIT $2 OFFSET $3
		`
		listArg = []any{filter.UserID, pageSize, offset}
	}

	if err := q.QueryRow(ctx, countSQL, countArg...).Scan(&total); err != nil {
		log.Printf("[req_id=%s] refund.Repository.ListByBuyer count error (user_id=%s): %v", reqIDFromContext(ctx), filter.UserID, err)
		return nil, 0, fmt.Errorf("failed to count buyer refunds: %w", err)
	}

	rows, err := q.Query(ctx, listSQL, listArg...)
	if err != nil {
		log.Printf("[req_id=%s] refund.Repository.ListByBuyer query error (user_id=%s): %v", reqIDFromContext(ctx), filter.UserID, err)
		return nil, 0, fmt.Errorf("failed to list buyer refunds: %w", err)
	}
	defer rows.Close()

	out := make([]Refund, 0)
	for rows.Next() {
		ref, scanErr := scanRefundRow(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("failed to scan buyer refund row: %w", scanErr)
		}
		out = append(out, *ref)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating buyer refunds: %w", err)
	}

	return out, total, nil
}
