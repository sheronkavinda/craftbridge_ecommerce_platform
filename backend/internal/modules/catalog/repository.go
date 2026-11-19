
package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type scanner interface {
	Scan(dest ...any) error
}

const productFields = `
	p.id, p.shop_id, p.category_id, p.title,
	p.description, p.price_cents, p.currency,
	p.status, p.created_at, p.updated_at
`

const productViewFields = productFields + `,
	s.name, c.name,
	COALESCE(i.available_qty, 0),
	COALESCE(i.reserved_qty, 0)
`

const productJoins = `
	FROM products p
	JOIN shops s ON s.id = p.shop_id
	JOIN categories c ON c.id = p.category_id
	LEFT JOIN inventory i ON i.product_id = p.id
`

func scanProduct(row scanner) (*Product, error) {
	var p Product

	err := row.Scan(
		&p.ID, &p.ShopID, &p.CategoryID,
		&p.Title, &p.Description, &p.PriceCents,
		&p.Currency, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &p, nil
}

func scanProductView(row scanner) (*ProductView, error) {
	var v ProductView

	err := row.Scan(
		&v.ID, &v.ShopID, &v.CategoryID,
		&v.Title, &v.Description, &v.PriceCents,
		&v.Currency, &v.Status, &v.CreatedAt, &v.UpdatedAt,
		&v.ShopName, &v.CategoryName,
		&v.AvailableQty, &v.ReservedQty,
	)

	if err != nil {
		return nil, err
	}

	return &v, nil
}

// Create a product and opening inventory in one transaction.
func (r *Repository) CreateProduct(
	ctx context.Context,
	userID int64,
	req CreateProductRequest,
) (*Product, error) {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var shopID int64

	err = tx.QueryRow(ctx, `
		SELECT id FROM shops
		WHERE owner_user_id = $1 AND status = 'active'
		FOR UPDATE
	`, userID).Scan(&shopID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShopUnavailable
	}
	if err != nil {
		return nil, err
	}

	var categoryActive bool

	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM categories
			WHERE id = $1 AND is_active = TRUE
		)
	`, req.CategoryID).Scan(&categoryActive)

	if err != nil {
		return nil, err
	}
	if !categoryActive {
		return nil, ErrCategoryUnavailable
	}

	p, err := scanProduct(tx.QueryRow(ctx, `
		INSERT INTO products AS p (
			shop_id, category_id, title, description,
			price_cents, currency, status
		)
		VALUES ($1, $2, $3, $4, $5, 'USD', 'draft')
		RETURNING `+productFields,
		shopID, req.CategoryID, req.Title,
		req.Description, req.PriceCents,
	))

	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO inventory (
			product_id, available_qty, reserved_qty
		)
		VALUES ($1, $2, 0)
	`, p.ID, req.StockQty)

	if err != nil {
		return nil, err
	}

	if req.StockQty > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO inventory_movements (
				product_id, available_delta,
				reserved_delta, reason, actor_user_id
			)
			VALUES ($1, $2, 0, 'initial_stock', $3)
		`, p.ID, req.StockQty, userID)

		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

// List products owned by the authenticated seller.
func (r *Repository) ListMyProducts(
	ctx context.Context,
	userID int64,
) ([]ProductView, error) {

	query := `SELECT ` + productViewFields + productJoins + `
		WHERE s.owner_user_id = $1
		ORDER BY p.created_at DESC, p.id DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]ProductView, 0)

	for rows.Next() {
		item, err := scanProductView(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *item)
	}

	return result, rows.Err()
}

// Get one product belonging to this seller.
func (r *Repository) GetMyProduct(
	ctx context.Context,
	userID, productID int64,
) (*ProductView, error) {

	query := `SELECT ` + productViewFields + productJoins + `
		WHERE p.id = $1 AND s.owner_user_id = $2
	`

	return scanProductView(
		r.db.QueryRow(ctx, query, productID, userID),
	)
}

// Update a product with server-side ownership checks.
func (r *Repository) UpdateProduct(
	ctx context.Context,
	userID, productID int64,
	req UpdateProductRequest,
) (*Product, error) {

	if req.CategoryID != nil {
		var active bool

		err := r.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM categories
				WHERE id = $1 AND is_active = TRUE
			)
		`, *req.CategoryID).Scan(&active)

		if err != nil {
			return nil, err
		}
		if !active {
			return nil, ErrCategoryUnavailable
		}
	}

	query := `
		UPDATE products AS p
		SET
			category_id = COALESCE($3::bigint, p.category_id),
			title = COALESCE($4::text, p.title),
			description = COALESCE($5::text, p.description),
			price_cents = COALESCE($6::bigint, p.price_cents),
			status = COALESCE($7::text, p.status),
			updated_at = NOW()
		FROM shops AS s
		WHERE p.id = $1
		  AND p.shop_id = s.id
		  AND s.owner_user_id = $2
		  AND s.status = 'active'
		  AND p.status <> 'archived'
		RETURNING ` + productFields

	return scanProduct(r.db.QueryRow(
		ctx,
		query,
		productID,
		userID,
		req.CategoryID,
		req.Title,
		req.Description,
		req.PriceCents,
		req.Status,
	))
}

// Archive instead of permanently deleting product records.
func (r *Repository) ArchiveProduct(
	ctx context.Context,
	userID, productID int64,
) (*Product, error) {

	query := `
		UPDATE products AS p
		SET status = 'archived', updated_at = NOW()
		FROM shops AS s
		WHERE p.id = $1
		  AND p.shop_id = s.id
		  AND s.owner_user_id = $2
		  AND s.status = 'active'
		  AND p.status <> 'archived'
		RETURNING ` + productFields

	return scanProduct(
		r.db.QueryRow(ctx, query, productID, userID),
	)
}

// Retrieve available product categories.
func (r *Repository) ListCategories(
	ctx context.Context,
) ([]Category, error) {

	rows, err := r.db.Query(ctx, `
		SELECT
			id, name, slug, COALESCE(description, ''),
			is_active, created_at
		FROM categories
		WHERE is_active = TRUE
		ORDER BY name ASC
	`)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]Category, 0)

	for rows.Next() {
		var c Category

		err := rows.Scan(
			&c.ID,
			&c.Name,
			&c.Slug,
			&c.Description,
			&c.IsActive,
			&c.CreatedAt,
		)

		if err != nil {
			return nil, err
		}

		result = append(result, c)
	}

	return result, rows.Err()
}

// Retrieve one published product for the public shop.
func (r *Repository) GetPublicProduct(
	ctx context.Context,
	productID int64,
) (*ProductView, error) {

	query := `SELECT ` + productViewFields + productJoins + `
		WHERE p.id = $1
		  AND p.status = 'published'
		  AND s.status = 'active'
		  AND c.is_active = TRUE
	`

	return scanProductView(
		r.db.QueryRow(ctx, query, productID),
	)
}

// Public search, filters, sorting and pagination.
func (r *Repository) ListPublicProducts(
	ctx context.Context,
	opts ListOptions,
) (*ListResult, error) {

	filters := `
		WHERE p.status = 'published'
		  AND s.status = 'active'
		  AND c.is_active = TRUE
		  AND (
		      $1::text = ''
		      OR p.title ILIKE '%' || $1 || '%'
		      OR p.description ILIKE '%' || $1 || '%'
		      OR s.name ILIKE '%' || $1 || '%'
		  )
		  AND ($2::bigint IS NULL OR p.category_id = $2)
		  AND ($3::bigint IS NULL OR p.price_cents >= $3)
		  AND ($4::bigint IS NULL OR p.price_cents <= $4)
	`

	args := []any{
		opts.Search,
		opts.CategoryID,
		opts.MinPriceCents,
		opts.MaxPriceCents,
	}

	var total int64

	err := r.db.QueryRow(
		ctx,
		`SELECT COUNT(*) `+productJoins+filters,
		args...,
	).Scan(&total)

	if err != nil {
		return nil, err
	}

	orderBy := "p.created_at DESC, p.id DESC"

	switch opts.Sort {
	case "price_asc":
		orderBy = "p.price_cents ASC, p.id DESC"
	case "price_desc":
		orderBy = "p.price_cents DESC, p.id DESC"
	case "oldest":
		orderBy = "p.created_at ASC, p.id ASC"
	}

	query := `SELECT ` + productViewFields +
		productJoins + filters +
		` ORDER BY ` + orderBy +
		` LIMIT $5 OFFSET $6`

	rows, err := r.db.Query(ctx, query, append(
		args,
		opts.PageSize,
		(opts.Page-1)*opts.PageSize,
	)...)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &ListResult{
		Items:    make([]ProductView, 0),
		Total:    total,
		Page:     opts.Page,
		PageSize: opts.PageSize,
	}

	for rows.Next() {
		p, err := scanProductView(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *p)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
