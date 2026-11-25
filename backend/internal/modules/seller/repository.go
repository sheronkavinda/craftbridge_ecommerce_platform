
package seller

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const applicationColumns = `
	id, user_id, store_name, country_code,
	COALESCE(description, ''), status,
	decision_reason, reviewed_by, reviewed_at,
	created_at, updated_at
`

const shopColumns = `
	id, owner_user_id, application_id,
	name, slug, COALESCE(description, ''),
	country_code, contact_email,
	profile_image_url, banner_image_url,
	status, created_at, updated_at
`

// Convert a database result into a seller application.
func scanApplication(row pgx.Row) (*SellerApplication, error) {
	var a SellerApplication

	err := row.Scan(
		&a.ID,
		&a.UserID,
		&a.StoreName,
		&a.CountryCode,
		&a.Description,
		&a.Status,
		&a.DecisionReason,
		&a.ReviewedBy,
		&a.ReviewedAt,
		&a.CreatedAt,
		&a.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &a, nil
}

// Convert a database result into a shop.
func scanShop(row pgx.Row) (*Shop, error) {
	var s Shop

	err := row.Scan(
		&s.ID,
		&s.OwnerUserID,
		&s.ApplicationID,
		&s.Name,
		&s.Slug,
		&s.Description,
		&s.CountryCode,
		&s.ContactEmail,
		&s.ProfileImageURL,
		&s.BannerImageURL,
		&s.Status,
		&s.CreatedAt,
		&s.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &s, nil
}

// CreateApplication stores a new seller application.
func (r *Repository) CreateApplication(
	ctx context.Context,
	userID int64,
	req CreateSellerApplicationRequest,
) (*SellerApplication, error) {

	query := `
		INSERT INTO seller_applications
			(user_id, store_name, country_code, description)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + applicationColumns

	row := r.db.QueryRow(
		ctx,
		query,
		userID,
		req.StoreName,
		req.CountryCode,
		req.Description,
	)

	return scanApplication(row)
}

// GetApplicationByUserID returns the user's application.
func (r *Repository) GetApplicationByUserID(
	ctx context.Context,
	userID int64,
) (*SellerApplication, error) {

	query := `
		SELECT ` + applicationColumns + `
		FROM seller_applications
		WHERE user_id = $1
	`

	return scanApplication(
		r.db.QueryRow(ctx, query, userID),
	)
}

// GetApplicationByID is used for administration.
func (r *Repository) GetApplicationByID(
	ctx context.Context,
	id int64,
) (*SellerApplication, error) {

	query := `
		SELECT ` + applicationColumns + `
		FROM seller_applications
		WHERE id = $1
	`

	return scanApplication(
		r.db.QueryRow(ctx, query, id),
	)
}

// GetShopByUserID returns only the shop owned by this user.
func (r *Repository) GetShopByUserID(
	ctx context.Context,
	userID int64,
) (*Shop, error) {

	query := `
		SELECT ` + shopColumns + `
		FROM shops
		WHERE owner_user_id = $1
	`

	return scanShop(
		r.db.QueryRow(ctx, query, userID),
	)
}

// UpdateShop modifies the authenticated seller's shop.
func (r *Repository) UpdateShop(
	ctx context.Context,
	userID int64,
	req UpdateShopRequest,
) (*Shop, error) {

	query := `
		UPDATE shops
		SET
			name = $2,
			description = $3,
			contact_email = NULLIF($4, ''),
			updated_at = NOW()
		WHERE owner_user_id = $1
		  AND status = 'active'
		RETURNING ` + shopColumns

	row := r.db.QueryRow(
		ctx,
		query,
		userID,
		req.Name,
		req.Description,
		req.ContactEmail,
	)

	return scanShop(row)
}

var invalidSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// Build a stable, unique shop slug.
func makeShopSlug(name string, applicationID int64) string {
	base := strings.ToLower(strings.TrimSpace(name))
	base = invalidSlugChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")

	if base == "" {
		base = "shop"
	}

	if len(base) > 100 {
		base = strings.Trim(base[:100], "-")
	}

	return base + "-" + strconv.FormatInt(applicationID, 10)
}

// ApproveApplication approves a pending application
// and creates its shop within one database transaction.
func (r *Repository) ApproveApplication(
	ctx context.Context,
	applicationID int64,
	reviewerID int64,
) (*Shop, error) {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin approval transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID int64
	var storeName string
	var countryCode string
	var description string
	var status string

	err = tx.QueryRow(ctx, `
		SELECT
			user_id,
			store_name,
			country_code,
			COALESCE(description, ''),
			status
		FROM seller_applications
		WHERE id = $1
		FOR UPDATE
	`, applicationID).Scan(
		&userID,
		&storeName,
		&countryCode,
		&description,
		&status,
	)

	if err != nil {
		return nil, err
	}

	if status != "pending" {
		return nil, ErrApplicationNotPending
	}

	_, err = tx.Exec(ctx, `
		UPDATE seller_applications
		SET
			status = 'approved',
			reviewed_by = $2,
			reviewed_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`, applicationID, reviewerID)

	if err != nil {
		return nil, err
	}

	slug := makeShopSlug(storeName, applicationID)

	query := `
		INSERT INTO shops (
			owner_user_id,
			application_id,
			name,
			slug,
			description,
			country_code,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'active')
		RETURNING ` + shopColumns

	shop, err := scanShop(tx.QueryRow(
		ctx,
		query,
		userID,
		applicationID,
		storeName,
		slug,
		description,
		countryCode,
	))

	if err != nil {
		return nil, err
	}

    _, err = tx.Exec(ctx, `
        INSERT INTO user_roles (user_id, role_id)
        VALUES ($1, (SELECT id FROM roles WHERE name = 'seller'))
        ON CONFLICT (user_id, role_id) DO NOTHING
    `, userID)
    if err != nil {
        return nil, fmt.Errorf("grant seller role: %w", err)
    }

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return shop, nil
}

// RejectApplication records an administrator's rejection.
func (r *Repository) RejectApplication(
	ctx context.Context,
	applicationID int64,
	reviewerID int64,
	reason string,
) (*SellerApplication, error) {

	query := `
		UPDATE seller_applications
		SET
			status = 'rejected',
			decision_reason = $3,
			reviewed_by = $2,
			reviewed_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
		RETURNING ` + applicationColumns

	return scanApplication(r.db.QueryRow(
		ctx,
		query,
		applicationID,
		reviewerID,
		reason,
	))
}
