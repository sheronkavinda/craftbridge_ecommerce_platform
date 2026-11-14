package admin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"craftbridge/backend/internal/modules/report"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound             = errors.New("record not found")
	ErrCategorySlugExists   = errors.New("category slug already exists")
	ErrRefundAlreadyDecided = errors.New("refund has already been decided")
	ErrLastAdmin            = errors.New("cannot remove the last administrator")
)

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
	db            *pgxpool.Pool
	reportService *report.Service
}

// NewRepository creates a new admin repository backed by a pgx connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db:            db,
		reportService: report.NewService(report.NewRepository(db)),
	}
}

// GetRevenueSummary delegates to the report module service so revenue SQL is never duplicated.
func (r *Repository) GetRevenueSummary(ctx context.Context, startStr, endStr string) (*report.RevenueSummary, error) {
	return r.reportService.GetRevenueSummary(ctx, startStr, endStr)
}

// ListUsers retrieves platform users with parameterized role, status, and case-insensitive search filters.
func (r *Repository) ListUsers(ctx context.Context, role, status, search string, limit, offset int) ([]UserSummary, error) {
	const query = `
		SELECT id::text, name, email, role, status, created_at
		FROM users
		WHERE ($1 = '' OR role = $1)
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR name ILIKE '%' || $3 || '%' ESCAPE '\' OR email ILIKE '%' || $3 || '%' ESCAPE '\')
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.db.Query(ctx, query, role, status, search, limit, offset)
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

// UpdateUserRole updates a user's role while atomically guarding against removing the last remaining ADMIN.
func (r *Repository) UpdateUserRole(ctx context.Context, userID, role string) (*UserSummary, error) {
	const query = `
		UPDATE users
		SET role = $2, updated_at = NOW()
		WHERE id::text = $1
		  AND (
		    $2 = 'ADMIN'
		    OR UPPER(role) <> 'ADMIN'
		    OR (SELECT COUNT(*) FROM users WHERE UPPER(role) = 'ADMIN') > 1
		  )
		RETURNING id::text, name, email, role, status, created_at
	`

	var u UserSummary
	err := r.db.QueryRow(ctx, query, userID, role).Scan(
		&u.ID, &u.Name, &u.Email, &u.Role, &u.Status, &u.CreatedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.UpdateUserRole error (user_id=%s): %v", reqIDFromContext(ctx), userID, err)
		if errors.Is(err, pgx.ErrNoRows) {
			var currentRole string
			var adminCount int64
			checkErr := r.db.QueryRow(
				ctx,
				`SELECT UPPER(role), (SELECT COUNT(*)::bigint FROM users WHERE UPPER(role) = 'ADMIN') FROM users WHERE id::text = $1`,
				userID,
			).Scan(&currentRole, &adminCount)
			if errors.Is(checkErr, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			if checkErr == nil && strings.EqualFold(currentRole, "ADMIN") && !strings.EqualFold(role, "ADMIN") && adminCount <= 1 {
				return nil, ErrLastAdmin
			}
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to update user role: %w", err)
	}

	return &u, nil
}

// ListSellerApplications fetches seller applications filtered by approval status.
func (r *Repository) ListSellerApplications(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error) {
	const query = `
		SELECT
			s.id::text,
			s.user_id::text,
			COALESCE(u.name, ''),
			COALESCE(u.email, ''),
			s.store_name,
			s.status,
			COALESCE(s.notes, ''),
			s.created_at,
			s.reviewed_at
		FROM sellers s
		LEFT JOIN users u ON u.id::text = s.user_id::text
		WHERE ($1 = '' OR s.status = $1)
		ORDER BY s.created_at DESC
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
		if err := rows.Scan(&s.ID, &s.UserID, &s.UserName, &s.UserEmail, &s.StoreName, &s.Status, &s.Notes, &s.SubmittedAt, &s.ReviewedAt); err != nil {
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
		UPDATE sellers s
		SET status = $2, notes = $3, reviewed_at = NOW(), updated_at = NOW()
		WHERE s.id::text = $1
		RETURNING
			s.id::text,
			s.user_id::text,
			COALESCE((SELECT u.name FROM users u WHERE u.id::text = s.user_id::text), ''),
			COALESCE((SELECT u.email FROM users u WHERE u.id::text = s.user_id::text), ''),
			s.store_name,
			s.status,
			COALESCE(s.notes, ''),
			s.created_at,
			s.reviewed_at
	`

	var s SellerApplication
	err := r.db.QueryRow(ctx, query, sellerID, status, notes).Scan(
		&s.ID, &s.UserID, &s.UserName, &s.UserEmail, &s.StoreName, &s.Status, &s.Notes, &s.SubmittedAt, &s.ReviewedAt,
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

// UpdateRefundStatus approves or rejects a PENDING refund request using parameterized queries.
func (r *Repository) UpdateRefundStatus(ctx context.Context, refundID, status, reason string) (*RefundRecord, error) {
	const query = `
		UPDATE refunds
		SET status = $2, reason = COALESCE(NULLIF($3, ''), reason), reviewed_at = NOW(), updated_at = NOW()
		WHERE id::text = $1 AND status = 'PENDING'
		RETURNING id::text, order_id::text, amount::float8, status, COALESCE(reason, ''), created_at, reviewed_at
	`

	var rec RefundRecord
	err := r.db.QueryRow(ctx, query, refundID, status, reason).Scan(
		&rec.ID, &rec.OrderID, &rec.Amount, &rec.Status, &rec.Reason, &rec.CreatedAt, &rec.ReviewedAt,
	)
	if err != nil {
		log.Printf("[req_id=%s] repository.UpdateRefundStatus error (refund_id=%s): %v", reqIDFromContext(ctx), refundID, err)
		if errors.Is(err, pgx.ErrNoRows) {
			var exists bool
			checkErr := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM refunds WHERE id::text = $1)`, refundID).Scan(&exists)
			if checkErr == nil && exists {
				return nil, ErrRefundAlreadyDecided
			}
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrCategorySlugExists
		}
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrCategorySlugExists
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

// GetDashboardCounts executes aggregate COUNT/SUM FILTER queries for users, sellers, refunds, and categories.
func (r *Repository) GetDashboardCounts(ctx context.Context) (*DashboardCounts, error) {
	const usersQuery = `
		SELECT
			COUNT(*)::bigint AS total,
			COUNT(*) FILTER (WHERE UPPER(status) = 'ACTIVE')::bigint AS active,
			COUNT(*) FILTER (WHERE UPPER(status) = 'SUSPENDED')::bigint AS suspended,
			COUNT(*) FILTER (WHERE UPPER(role) = 'BUYER')::bigint AS buyer_count,
			COUNT(*) FILTER (WHERE UPPER(role) = 'SELLER')::bigint AS seller_count,
			COUNT(*) FILTER (WHERE UPPER(role) = 'ADMIN')::bigint AS admin_count
		FROM users
	`
	const sellersQuery = `
		SELECT
			COUNT(*) FILTER (WHERE UPPER(status) = 'PENDING')::bigint AS pending,
			COUNT(*) FILTER (WHERE UPPER(status) = 'APPROVED')::bigint AS approved,
			COUNT(*) FILTER (WHERE UPPER(status) = 'REJECTED')::bigint AS rejected,
			COUNT(*) FILTER (WHERE UPPER(status) = 'SUSPENDED')::bigint AS suspended
		FROM sellers
	`
	const refundsQuery = `
		SELECT
			COUNT(*) FILTER (WHERE UPPER(status) = 'PENDING')::bigint AS pending_count,
			COALESCE(SUM(amount) FILTER (WHERE UPPER(status) = 'PENDING'), 0)::float8 AS pending_amount
		FROM refunds
	`
	const categoriesQuery = `
		SELECT
			COUNT(*)::bigint AS total,
			COUNT(*) FILTER (WHERE is_active = TRUE)::bigint AS active
		FROM categories
	`

	var counts DashboardCounts

	if err := r.db.QueryRow(ctx, usersQuery).Scan(
		&counts.Users.Total,
		&counts.Users.Active,
		&counts.Users.Suspended,
		&counts.Users.ByRole.Buyer,
		&counts.Users.ByRole.Seller,
		&counts.Users.ByRole.Admin,
	); err != nil {
		log.Printf("[req_id=%s] repository.GetDashboardCounts users query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to aggregate user stats: %w", err)
	}

	if err := r.db.QueryRow(ctx, sellersQuery).Scan(
		&counts.Sellers.Pending,
		&counts.Sellers.Approved,
		&counts.Sellers.Rejected,
		&counts.Sellers.Suspended,
	); err != nil {
		log.Printf("[req_id=%s] repository.GetDashboardCounts sellers query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to aggregate seller stats: %w", err)
	}

	if err := r.db.QueryRow(ctx, refundsQuery).Scan(
		&counts.Refunds.PendingCount,
		&counts.Refunds.PendingAmount,
	); err != nil {
		log.Printf("[req_id=%s] repository.GetDashboardCounts refunds query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to aggregate refund stats: %w", err)
	}

	if err := r.db.QueryRow(ctx, categoriesQuery).Scan(
		&counts.Categories.Total,
		&counts.Categories.Active,
	); err != nil {
		log.Printf("[req_id=%s] repository.GetDashboardCounts categories query error: %v", reqIDFromContext(ctx), err)
		return nil, fmt.Errorf("failed to aggregate category stats: %w", err)
	}

	return &counts, nil
}
