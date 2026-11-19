package cart

import "errors"

var (
	ErrInvalidCart      = errors.New("invalid cart request")
	ErrUnavailable      = errors.New("product unavailable or insufficient stock")
	ErrCartItemNotFound = errors.New("cart item not found")
	ErrCartEmpty        = errors.New("cart is empty")
)

type Item struct {
	ID             int64  `json:"id"`
	ProductID      int64  `json:"product_id"`
	ShopID         int64  `json:"shop_id"`
	ShopName       string `json:"shop_name"`
	Title          string `json:"title"`
	Quantity       int32  `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	LineTotalCents int64  `json:"line_total_cents"`
	AvailableQty   int32  `json:"available_qty"`
	IsAvailable    bool   `json:"is_available"`
}

type Summary struct {
	Items         []Item `json:"items"`
	SubtotalCents int64  `json:"subtotal_cents"`
	Currency      string `json:"currency"`
}

type AddItemRequest struct {
	ProductID int64 `json:"product_id" binding:"required,gt=0"`
	Quantity  int32 `json:"quantity" binding:"required,min=1,max=99"`
}

type UpdateItemRequest struct {
	Quantity int32 `json:"quantity" binding:"required,min=1,max=99"`
}
