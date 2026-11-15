package receipt

import "time"

// Receipt represents an immutable digital payment receipt issued upon successful order payment.
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
