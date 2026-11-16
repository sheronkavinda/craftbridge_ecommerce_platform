
package order

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidOrder = errors.New("invalid order request")
	ErrEmptyCart = errors.New("cart is empty")
	ErrInvalidAddress = errors.New("invalid delivery address")
	ErrStockUnavailable = errors.New("product unavailable or insufficient stock")
	ErrOrderNotFound = errors.New("order not found")
	ErrInvalidTransition = errors.New("invalid order state transition")
	ErrInventoryUnavailable = errors.New("inventory integration unavailable")
	ErrIdempotencyConflict = errors.New("idempotency key reused for another checkout")
)

// Kanishka's catalog service implements this interface.
// Do not create another independent inventory table.
type InventoryManager interface {
	ReserveForCheckout(
		context.Context, pgx.Tx, int64, int32,
	) error

	ReleaseCheckoutReservation(
		context.Context, pgx.Tx, int64, int32,
	) error

	ConfirmCheckoutReservation(
		context.Context, pgx.Tx, int64, int32,
	) error
}

type CheckoutRequest struct {
	AddressID int64 `json:"address_id" binding:"required,gt=0"`
}

type Order struct {
	ID                 int64           `json:"id"`
	BuyerID            int64           `json:"buyer_id"`
	AddressID          int64           `json:"address_id"`
	AddressSnapshot    json.RawMessage `json:"address_snapshot"`
	SubtotalCents      int64           `json:"subtotal_cents"`
	ShippingTotalCents int64           `json:"shipping_total_cents"`
	GrandTotalCents    int64           `json:"grand_total_cents"`
	Currency           string          `json:"currency"`
	PaymentStatus      string          `json:"payment_status"`
	Status             string          `json:"status"`
	ExpiresAt          time.Time       `json:"expires_at"`
	CreatedAt          time.Time       `json:"created_at"`
	SellerOrders       []SellerOrder   `json:"seller_orders"`
}

type SellerOrder struct {
	ID            int64       `json:"id"`
	ShopID        int64       `json:"shop_id"`
	ShopName      string      `json:"shop_name"`
	SubtotalCents int64       `json:"subtotal_cents"`
	ShippingCents int64       `json:"shipping_cents"`
	Status        string      `json:"status"`
	Items         []OrderItem `json:"items"`
}

type OrderItem struct {
	ID             int64  `json:"id"`
	ProductID      int64  `json:"product_id"`
	Title          string `json:"title"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Quantity       int32  `json:"quantity"`
	LineTotalCents int64  `json:"line_total_cents"`
}

type OrderSummary struct {
	ID            int64     `json:"id"`
	GrandTotal    int64     `json:"grand_total_cents"`
	PaymentStatus string    `json:"payment_status"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type checkoutItem struct {
	ProductID int64
	ShopID    int64
	Title     string
	Price     int64
	Quantity  int32
}

type checkoutGroup struct {
	ShopID   int64
	Subtotal int64
	Items    []checkoutItem
}
