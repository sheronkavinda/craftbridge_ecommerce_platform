package receipt

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Receipt represents an immutable digital payment receipt stored in the receipts table.
type Receipt struct {
	ID               string    `json:"id"`
	OrderID          string    `json:"order_id"`
	PaymentAttemptID string    `json:"payment_attempt_id"`
	UserID           string    `json:"user_id"`
	ReceiptNumber    string    `json:"receipt_number"`
	AmountCents      int64     `json:"amount_cents"`
	Currency         string    `json:"currency"`
	IssuedAt         time.Time `json:"issued_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// Actor represents the authenticated caller requesting a receipt.
type Actor struct {
	ID   string
	Name string
	Role string
}

// PaidOrderPayment holds the verified SUCCESS payment attempt and buyer metadata for an order.
type PaidOrderPayment struct {
	AttemptID         string
	OrderID           string
	OrderNumber       string
	BuyerID           string
	BuyerName         string
	BuyerEmail        string
	AmountCents       int64
	Currency          string
	Provider          string
	ProviderReference string
	CompletedAt       time.Time
}

// ReceiptLineItem represents a single order item line on a receipt when provided by the orders schema.
type ReceiptLineItem struct {
	Title          string `json:"title"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	UnitPrice      string `json:"unit_price"`
	LineTotalCents int64  `json:"line_total_cents"`
	LineTotal      string `json:"line_total"`
}

// OrderSummary holds optional line items and subtotal/shipping/tax breakdowns when available.
type OrderSummary struct {
	Items         []ReceiptLineItem `json:"items"`
	SubtotalCents *int64            `json:"subtotal_cents,omitempty"`
	ShippingCents *int64            `json:"shipping_cents,omitempty"`
	TaxCents      *int64            `json:"tax_cents,omitempty"`
}

// ReceiptView represents the enriched owner-restricted receipt payload returned by GET /api/v1/me/orders/:id/receipt.
// Internal signatures, raw webhook payloads, and idempotency keys are never included.
type ReceiptView struct {
	ReceiptNumber     string            `json:"receipt_number"`
	IssuedAt          time.Time         `json:"issued_at"`
	OrderID           string            `json:"order_id"`
	OrderNumber       string            `json:"order_number"`
	BuyerName         string            `json:"buyer_name"`
	BuyerEmail        string            `json:"buyer_email"`
	PaymentMethod     string            `json:"payment_method"`
	ProviderReference string            `json:"provider_reference"`
	AmountCents       int64             `json:"amount_cents"`
	Amount            string            `json:"amount"`
	Currency          string            `json:"currency"`
	Items             []ReceiptLineItem `json:"items"`
	RefundedCents     int64             `json:"refunded_cents"`
	NetPaidCents      int64             `json:"net_paid_cents"`
	Status            string            `json:"status"`
}

// GenerateReceiptNumber builds a deterministic unique receipt number in the format:
// RCP-YYYYMMDD-<6 uppercase hex characters from md5(order_id)>.
func GenerateReceiptNumber(orderID string, refTime time.Time) string {
	sum := md5.Sum([]byte(strings.TrimSpace(orderID)))
	hex6 := strings.ToUpper(hex.EncodeToString(sum[:]))[:6]
	dateStr := refTime.UTC().Format("20060102")
	return fmt.Sprintf("RCP-%s-%s", dateStr, hex6)
}

// FormatPaymentMethodLabel maps a canonical provider code to a human-readable payment method label.
func FormatPaymentMethodLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "payhere_sandbox", "payhere":
		return "PayHere Sandbox"
	default:
		if trimmed := strings.TrimSpace(provider); trimmed != "" {
			return trimmed
		}
		return "Online Payment"
	}
}

// FormatCentsDecimal formats integer cents as a 2-decimal string (e.g. 200000 -> "2000.00") without floating-point math.
func FormatCentsDecimal(cents int64) string {
	if cents < 0 {
		abs := -cents
		return fmt.Sprintf("-%d.%02d", abs/100, abs%100)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
