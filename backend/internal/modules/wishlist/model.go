
package wishlist

import "time"

// WishlistItem represents a saved product.
type WishlistItem struct {
    ID        int64     `json:"id"`
    UserID    string    `json:"userId"`
    ProductID string    `json:"productId"`
    CreatedAt time.Time `json:"createdAt"`
}

// AddWishlistRequest is the request body for saving a product.
type AddWishlistRequest struct {
    ProductID string `json:"productId" binding:"required"`
}

// WishlistResponse contains the user's saved products.
type WishlistResponse struct {
    Items []WishlistItem `json:"items"`
    Total int            `json:"total"`
}
