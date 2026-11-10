package admin

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("record not found")

type ctxKeyRequestID struct{}

// WithRequestID attaches a request ID to a context for repository error logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID{}, requestID)
}

func reqIDFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(ctxKeyRequestID{}).(string); ok && val != "" {
		return val
	}
	return "unknown"
}

// Repository executes 100% parameterized SQL queries using pgx to prevent SQL injection.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new admin repository backed by a pgx connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// ListUsers retrieves platform users with parameterized role and status filters.
func (r *Repository) ListUsers(ctx context.Context, role, status string, limit, offset int) ([]UserSummary, error) {
	const query = `
		SELECT id::text, name, email, role, status, created_at
		FROM users
		WHERE ($1 = '' OR role = $1)
		  AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.db.Query(ctx, query, role, status, limit, offset)
	if err != nil {
		log.Printf("[req_id=%s] repository.ListUsers query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	users := make([]UserSummary, 0)
	for rows.Next() {
		var u UserSummary
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Status, &u.CreatedAt); err != nil {
			log.Printf("[req_id=%s] repository.ListUsers scan error: %v", reqIDFromContext(ctx), err)
			return nil, fmt.Errorf("failed to scan user row: %w", err)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[req_id=%s] repository.ListUsers rows iteration error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("error iterating user rows: %w", err)
	}

	return users, nil
}

// UpdateUserStatus updates a user's account status using parameterized queries.
func (r *Repository) UpdateUserStatus(ctx context.Context, userID, status string) (*UserSummary, error) {
	const query = `
		UPDATE users
		SET status = $2, updated_at = NOW()
		WHERE id::text = $1
		RETURNING id::text, name, email, role, status, created_at
	`

	var u UserSummary
	err := r.db.QueryRow(ctx, query, userID, status).Scan(
		&u.ID, &u.Name, &u.Email, &u.Role, &u.Status, &u.CreatedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.UpdateUserStatus error (user_id=%s): %v", reqIDFromContext(ctx), userID, err)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update user status: %w", err)
	}

	return &u, nil
}

// ListSellerApplications fetches seller applications filtered by approval status.
func (r *Repository) ListSellerApplications(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error) {
	const query = `
		SELECT id::text, user_id::text, store_name, status, COALESCE(notes, ''), created_at, reviewed_at
		FROM sellers
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, status, limit, offset)
	if err != nil {
		log.Printf("[req_id=%s] repository.ListSellerApplications query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to list seller applications: %w", err)
	}
	defer rows.Close()

	sellers := make([]SellerApplication, 0)
	for rows.Next() {
		var s SellerApplication
		if err := rows.Scan(&s.ID, &s.UserID, &s.StoreName, &s.Status, &s.Notes, &s.SubmittedAt, &s.ReviewedAt); err != nil {
			log.Printf("[req_id=%s] repository.ListSellerApplications scan error: %v", reqIDFromContext(ctx), err)
			return nil, fmt.Errorf("failed to scan seller application row: %w", err)
		}
		sellers = append(sellers, s)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[req_id=%s] repository.ListSellerApplications rows iteration error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("error iterating seller application rows: %w", err)
	}

	return sellers, nil
}

// UpdateSellerStatus transitions a seller application status (APPROVED, REJECTED, SUSPENDED).
func (r *Repository) UpdateSellerStatus(ctx context.Context, sellerID, status, notes string) (*SellerApplication, error) {
	const query = `
		UPDATE sellers
		SET status = $2, notes = $3, reviewed_at = NOW(), updated_at = NOW()
		WHERE id::text = $1
		RETURNING id::text, user_id::text, store_name, status, COALESCE(notes, ''), created_at, reviewed_at
	`

	var s SellerApplication
	err := r.db.QueryRow(ctx, query, sellerID, status, notes).Scan(
		&s.ID, &s.UserID, &s.StoreName, &s.Status, &s.Notes, &s.SubmittedAt, &s.ReviewedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.UpdateSellerStatus error (seller_id=%s): %v", reqIDFromContext(ctx), sellerID, err)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update seller status: %w", err)
	}

	return &s, nil
}

// ListRefunds retrieves refund requests filtered by status using parameterized queries.
func (r *Repository) ListRefunds(ctx context.Context, status string, limit, offset int) ([]RefundRecord, error) {
	const query = `
		SELECT id::text, order_id::text, amount::float8, status, COALESCE(reason, ''), created_at, reviewed_at
		FROM refunds
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, status, limit, offset)
	if err != nil {
		log.Printf("[req_id=%s] repository.ListRefunds query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to list refunds: %w", err)
	}
	defer rows.Close()

	refunds := make([]RefundRecord, 0)
	for rows.Next() {
		var rec RefundRecord
		if err := rows.Scan(&rec.ID, &rec.OrderID, &rec.Amount, &rec.Status, &rec.Reason, &rec.CreatedAt, &rec.ReviewedAt); err != nil {
			log.Printf("[req_id=%s] repository.ListRefunds scan error: %v", reqIDFromContext(ctx), err)
			return nil, fmt.Errorf("failed to scan refund row: %w", err)
		}
		refunds = append(refunds, rec)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[req_id=%s] repository.ListRefunds rows iteration error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("error iterating refund rows: %w", err)
	}

	return refunds, nil
}

// UpdateRefundStatus approves or rejects a refund request using parameterized queries.
func (r *Repository) UpdateRefundStatus(ctx context.Context, refundID, status, reason string) (*RefundRecord, error) {
	const query = `
		UPDATE refunds
		SET status = $2, reason = COALESCE(NULLIF($3, ''), reason), reviewed_at = NOW(), updated_at = NOW()
		WHERE id::text = $1
		RETURNING id::text, order_id::text, amount::float8, status, COALESCE(reason, ''), created_at, reviewed_at
	`

	var rec RefundRecord
	err := r.db.QueryRow(ctx, query, refundID, status, reason).Scan(
		&rec.ID, &rec.OrderID, &rec.Amount, &rec.Status, &rec.Reason, &rec.CreatedAt, &rec.ReviewedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.UpdateRefundStatus error (refund_id=%s): %v", reqIDFromContext(ctx), refundID, err)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update refund status: %w", err)
	}

	return &rec, nil
}

// ListCategories retrieves all catalog categories ordered by name.
func (r *Repository) ListCategories(ctx context.Context) ([]Category, error) {
	const query = `
		SELECT id::text, name, slug, COALESCE(description, ''), is_active, created_at
		FROM categories
		ORDER BY name ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		log.Printf("[req_id=%s] repository.ListCategories query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	defer rows.Close()

	categories := make([]Category, 0)
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsActive, &c.CreatedAt); err != nil {
			log.Printf("[req_id=%s] repository.ListCategories scan error: %v", reqIDFromContext(ctx), err)
			return nil, fmt.Errorf("failed to scan category row: %w", err)
		}
		categories = append(categories, c)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[req_id=%s] repository.ListCategories rows iteration error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("error iterating category rows: %w", err)
	}

	return categories, nil
}

// CreateCategory inserts a new category (with an explicit generated id) into the categories table.
func (r *Repository) CreateCategory(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error) {
	const query = `
		INSERT INTO categories (id, name, slug, description, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, name, slug, COALESCE(description, ''), is_active, created_at
	`

	var c Category
	err := r.db.QueryRow(ctx, query, categoryID, name, slug, description, isActive).Scan(
		&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsActive, &c.CreatedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.CreateCategory error (id=%s, slug=%s): %v", reqIDFromContext(ctx), categoryID, slug, err)
		return nil, fmt.Errorf("failed to create category: %w", err)
	}

	return &c, nil
}

// UpdateCategory updates an existing category by ID.
func (r *Repository) UpdateCategory(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error) {
	const query = `
		UPDATE categories
		SET name = $2, slug = $3, description = $4, is_active = $5, updated_at = NOW()
		WHERE id::text = $1
		RETURNING id::text, name, slug, COALESCE(description, ''), is_active, created_at
	`

	var c Category
	err := r.db.QueryRow(ctx, query, categoryID, name, slug, description, isActive).Scan(
		&c.ID, &c.Name, &c.Slug, &c.Description, &c.IsActive, &c.CreatedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.UpdateCategory error (category_id=%s): %v", reqIDFromContext(ctx), categoryID, err)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update category: %w", err)
	}

	return &c, nil
}

// DeleteCategory removes a category by ID.
func (r *Repository) DeleteCategory(ctx context.Context, categoryID string) error {
	const query = `DELETE FROM categories WHERE id::text = $1`
	tag, err := r.db.Exec(ctx, query, categoryID)
	if err != nil {
		log.Printf("[req_id=%s] repository.DeleteCategory error (category_id=%s): %v", reqIDFromContext(ctx), categoryID, err)
		return fmt.Errorf("failed to delete category: %w", err)
	}
	if tag.RowsAffected() == 0 {
		log.Printf("[req_id=%s] repository.DeleteCategory not found (category_id=%s)", reqIDFromContext(ctx), categoryID)
		return ErrNotFound
	}
	return nil
}
