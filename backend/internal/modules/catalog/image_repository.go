
package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var ErrImageLimitReached = errors.New("maximum product images reached")
var ErrImageNotFound = errors.New("product image not found")

func scanImage(row scanner) (*ProductImage, error) {
	var img ProductImage

	err := row.Scan(
		&img.ID,
		&img.ProductID,
		&img.ImageURL,
		&img.AltText,
		&img.SortOrder,
		&img.CreatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &img, nil
}

// AddProductImage verifies product ownership and
// saves image metadata in the database.
func (r *Repository) AddProductImage(
	ctx context.Context,
	userID, productID int64,
	imageURL, altText string,
) (*ProductImage, error) {

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var ownedProductID int64

	err = tx.QueryRow(ctx, `
		SELECT p.id
		FROM products p
		JOIN shops s ON s.id = p.shop_id
		WHERE p.id = $1
		  AND s.owner_user_id = $2
		  AND s.status = 'active'
		  AND p.status <> 'archived'
		FOR UPDATE OF p
	`, productID, userID).Scan(&ownedProductID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}

	var imageCount int

	err = tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM product_images
		WHERE product_id = $1
	`, productID).Scan(&imageCount)

	if err != nil {
		return nil, err
	}

	if imageCount >= 8 {
		return nil, ErrImageLimitReached
	}

	image, err := scanImage(tx.QueryRow(ctx, `
		INSERT INTO product_images (
			product_id, image_url, alt_text, sort_order
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id, product_id, image_url,
			COALESCE(alt_text, ''),
			sort_order, created_at
	`, productID, imageURL, altText, imageCount))

	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return image, nil
}

// List images for a publicly available product.
func (r *Repository) ListPublicProductImages(
	ctx context.Context,
	productID int64,
) ([]ProductImage, error) {

	rows, err := r.db.Query(ctx, `
		SELECT
			pi.id,
			pi.product_id,
			pi.image_url,
			COALESCE(pi.alt_text, ''),
			pi.sort_order,
			pi.created_at
		FROM product_images pi
		JOIN products p ON p.id = pi.product_id
		JOIN shops s ON s.id = p.shop_id
		JOIN categories c ON c.id = p.category_id
		WHERE p.id = $1
		  AND p.status = 'published'
		  AND s.status = 'active'
		  AND c.is_active = TRUE
		ORDER BY pi.sort_order, pi.id
	`, productID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]ProductImage, 0)

	for rows.Next() {
		image, err := scanImage(rows)
		if err != nil {
			return nil, err
		}

		result = append(result, *image)
	}

	return result, rows.Err()
}

// Remove metadata only when the seller owns the image.
func (r *Repository) RemoveProductImage(
	ctx context.Context,
	userID, imageID int64,
) (string, error) {

	var imageURL string

	err := r.db.QueryRow(ctx, `
		DELETE FROM product_images pi
		USING products p, shops s
		WHERE pi.id = $1
		  AND pi.product_id = p.id
		  AND p.shop_id = s.id
		  AND s.owner_user_id = $2
		  AND s.status = 'active'
		  AND p.status <> 'archived'
		RETURNING pi.image_url
	`, imageID, userID).Scan(&imageURL)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrImageNotFound
	}

	return imageURL, err
}
