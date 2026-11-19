
package catalog

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidProductInput = errors.New("invalid product data")
	ErrProductNotFound     = errors.New("product not found")
	ErrShopUnavailable    = errors.New("approved active seller shop required")
	ErrCategoryUnavailable = errors.New("product category not found or inactive")
)

// ProductView includes product, shop and stock details.
type ProductView struct {
	Product
	ShopName     string `json:"shop_name"`
	CategoryName string `json:"category_name"`
	AvailableQty int32  `json:"available_qty"`
	ReservedQty  int32  `json:"reserved_qty"`
}

// Optional fields allow partial product updates.
type UpdateProductRequest struct {
	CategoryID  *int64  `json:"category_id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	PriceCents  *int64  `json:"price_cents"`
	Status      *string `json:"status"`
}

type ListOptions struct {
	Search        string
	CategoryID    *int64
	MinPriceCents *int64
	MaxPriceCents *int64
	Sort          string
	Page          int
	PageSize      int
}

type ListResult struct {
	Items    []ProductView `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func validateProductText(title, description string) bool {
	titleLen := utf8.RuneCountInString(title)
	descLen := utf8.RuneCountInString(description)

	return titleLen >= 3 &&
		titleLen <= 200 &&
		descLen >= 1 &&
		descLen <= 5000
}

// Validate product data before saving.
func (s *Service) CreateProduct(
	ctx context.Context,
	userID int64,
	req CreateProductRequest,
) (*Product, error) {

	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)

	if userID <= 0 ||
		req.CategoryID <= 0 ||
		req.PriceCents <= 0 ||
		req.StockQty < 0 ||
		!validateProductText(req.Title, req.Description) {
		return nil, ErrInvalidProductInput
	}

	return s.repo.CreateProduct(ctx, userID, req)
}

// Retrieve all products owned by one seller.
func (s *Service) ListMyProducts(
	ctx context.Context,
	userID int64,
) ([]ProductView, error) {

	if userID <= 0 {
		return nil, ErrInvalidProductInput
	}

	return s.repo.ListMyProducts(ctx, userID)
}

// Retrieve one seller-owned product.
func (s *Service) GetMyProduct(
	ctx context.Context,
	userID, productID int64,
) (*ProductView, error) {

	if userID <= 0 || productID <= 0 {
		return nil, ErrInvalidProductInput
	}

	p, err := s.repo.GetMyProduct(ctx, userID, productID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}

	return p, err
}

// Update product fields.
func (s *Service) UpdateProduct(
	ctx context.Context,
	userID, productID int64,
	req UpdateProductRequest,
) (*Product, error) {

	if userID <= 0 || productID <= 0 {
		return nil, ErrInvalidProductInput
	}

	if req.CategoryID == nil &&
		req.Title == nil &&
		req.Description == nil &&
		req.PriceCents == nil &&
		req.Status == nil {
		return nil, ErrInvalidProductInput
	}

	if req.CategoryID != nil && *req.CategoryID <= 0 {
		return nil, ErrInvalidProductInput
	}

	if req.PriceCents != nil && *req.PriceCents <= 0 {
		return nil, ErrInvalidProductInput
	}

	if req.Title != nil {
		value := strings.TrimSpace(*req.Title)

		if n := utf8.RuneCountInString(value); n < 3 || n > 200 {
			return nil, ErrInvalidProductInput
		}

		req.Title = &value
	}

	if req.Description != nil {
		value := strings.TrimSpace(*req.Description)

		if n := utf8.RuneCountInString(value); n < 1 || n > 5000 {
			return nil, ErrInvalidProductInput
		}

		req.Description = &value
	}

	if req.Status != nil {
		value := strings.TrimSpace(*req.Status)

		if value != "draft" &&
			value != "published" &&
			value != "archived" {
			return nil, ErrInvalidProductInput
		}

		req.Status = &value
	}

	p, err := s.repo.UpdateProduct(ctx, userID, productID, req)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}

	return p, err
}

// Archive a product without deleting its history.
func (s *Service) ArchiveProduct(
	ctx context.Context,
	userID, productID int64,
) (*Product, error) {

	if userID <= 0 || productID <= 0 {
		return nil, ErrInvalidProductInput
	}

	p, err := s.repo.ArchiveProduct(ctx, userID, productID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}

	return p, err
}

// Display active categories.
func (s *Service) ListCategories(
	ctx context.Context,
) ([]Category, error) {
	return s.repo.ListCategories(ctx)
}

// Public product details.
func (s *Service) GetPublicProduct(
	ctx context.Context,
	productID int64,
) (*ProductView, error) {

	if productID <= 0 {
		return nil, ErrInvalidProductInput
	}

	p, err := s.repo.GetPublicProduct(ctx, productID)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}

	return p, err
}

// Validate public catalog search and pagination.
func (s *Service) ListPublicProducts(
	ctx context.Context,
	opts ListOptions,
) (*ListResult, error) {

	opts.Search = strings.TrimSpace(opts.Search)

	if opts.Page < 1 ||
		opts.Page > 10000 ||
		opts.PageSize < 1 ||
		opts.PageSize > 50 ||
		utf8.RuneCountInString(opts.Search) > 120 {
		return nil, ErrInvalidProductInput
	}

	if opts.CategoryID != nil && *opts.CategoryID <= 0 {
		return nil, ErrInvalidProductInput
	}

	if opts.MinPriceCents != nil && *opts.MinPriceCents < 0 {
		return nil, ErrInvalidProductInput
	}

	if opts.MaxPriceCents != nil && *opts.MaxPriceCents < 0 {
		return nil, ErrInvalidProductInput
	}

	if opts.MinPriceCents != nil &&
		opts.MaxPriceCents != nil &&
		*opts.MinPriceCents > *opts.MaxPriceCents {
		return nil, ErrInvalidProductInput
	}

	if opts.Sort != "newest" &&
		opts.Sort != "oldest" &&
		opts.Sort != "price_asc" &&
		opts.Sort != "price_desc" {
		return nil, ErrInvalidProductInput
	}

	return s.repo.ListPublicProducts(ctx, opts)
}
