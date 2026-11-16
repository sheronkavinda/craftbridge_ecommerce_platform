
package catalog

import "time"

// Category represents a handmade product category.
type Category struct {
    ID          int64     `json:"id"`
    Name        string    `json:"name"`
    Slug        string    `json:"slug"`
    Description string    `json:"description"`
    IsActive    bool      `json:"is_active"`
    CreatedAt   time.Time `json:"created_at"`
}

// Product represents a CraftBridge product.
type Product struct {
    ID          int64     `json:"id"`
    ShopID      int64     `json:"shop_id"`
    CategoryID  int64     `json:"category_id"`
    Title       string    `json:"title"`
    Description string    `json:"description"`
    PriceCents  int64     `json:"price_cents"`
    Currency    string    `json:"currency"`
    Status      string    `json:"status"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// CreateProductRequest receives new product details.
type CreateProductRequest struct {
    CategoryID  int64  `json:"category_id" binding:"required"`
    Title       string `json:"title" binding:"required"`
    Description string `json:"description" binding:"required"`
    PriceCents  int64  `json:"price_cents" binding:"required,gt=0"`
    StockQty    int32  `json:"stock_quantity" binding:"gte=0"`
}

// ProductImage stores an image reference.
type ProductImage struct {
    ID        int64     `json:"id"`
    ProductID int64     `json:"product_id"`
    ImageURL  string    `json:"image_url"`
    AltText   string    `json:"alt_text"`
    SortOrder int32     `json:"sort_order"`
    CreatedAt time.Time `json:"created_at"`
}

// Inventory tracks stock for each product.
type Inventory struct {
    ProductID    int64     `json:"product_id"`
    AvailableQty int32     `json:"available_qty"`
    ReservedQty  int32     `json:"reserved_qty"`
    UpdatedAt    time.Time `json:"updated_at"`
}

// InventoryMovement records a stock change.
type InventoryMovement struct {
    ID             int64     `json:"id"`
    ProductID      int64     `json:"product_id"`
    AvailableDelta int32     `json:"available_delta"`
    ReservedDelta  int32     `json:"reserved_delta"`
    Reason         string    `json:"reason"`
    ActorUserID    *int64    `json:"actor_user_id"`
    CreatedAt      time.Time `json:"created_at"`
}
