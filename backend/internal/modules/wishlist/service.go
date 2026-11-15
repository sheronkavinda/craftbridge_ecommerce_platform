
package wishlist

import (
    "context"
    "errors"
    "strings"
)

var (
    ErrInvalidUserID    = errors.New("user ID is required")
    ErrInvalidProductID = errors.New("product ID is required")
)

// Service handles wishlist business logic.
type Service struct {
    repo *Repository
}

// NewService creates a wishlist service.
func NewService(repo *Repository) *Service {
    return &Service{repo: repo}
}

// Add saves a product to the user's wishlist.
func (s *Service) Add(
    ctx context.Context,
    userID string,
    productID string,
) (*WishlistItem, error) {
    userID = strings.TrimSpace(userID)
    productID = strings.TrimSpace(productID)

    if userID == "" {
        return nil, ErrInvalidUserID
    }

    if productID == "" {
        return nil, ErrInvalidProductID
    }

    return s.repo.Add(ctx, userID, productID)
}

// List returns the user's wishlist.
func (s *Service) List(
    ctx context.Context,
    userID string,
) (*WishlistResponse, error) {
    userID = strings.TrimSpace(userID)

    if userID == "" {
        return nil, ErrInvalidUserID
    }

    items, err := s.repo.List(ctx, userID)
    if err != nil {
        return nil, err
    }

    return &WishlistResponse{
        Items: items,
        Total: len(items),
    }, nil
}

// Remove deletes a product from the user's wishlist.
func (s *Service) Remove(
    ctx context.Context,
    userID string,
    productID string,
) error {
    userID = strings.TrimSpace(userID)
    productID = strings.TrimSpace(productID)

    if userID == "" {
        return ErrInvalidUserID
    }

    if productID == "" {
        return ErrInvalidProductID
    }

    return s.repo.Remove(ctx, userID, productID)
}
