package refund

import "time"

// RefundStatus represents the review and processing state of a refund request.
type RefundStatus string

const (
	RefundStatusPending   RefundStatus = "PENDING"
	RefundStatusApproved  RefundStatus = "APPROVED"
	RefundStatusProcessed RefundStatus = "PROCESSED"
	RefundStatusCompleted RefundStatus = "COMPLETED"
	RefundStatusRejected  RefundStatus = "REJECTED"
)

// Refund represents a buyer or seller-initiated refund request linked to an order and payment attempt.
type Refund struct {
	ID                      string       `json:"id"`
	OrderID                 string       `json:"order_id"`
	PaymentAttemptID        *string      `json:"payment_attempt_id,omitempty"`
	RequestedBy             string       `json:"requested_by"`
	SellerID                *string      `json:"seller_id,omitempty"`
	Reason                  string       `json:"reason"`
	Details                 string       `json:"details"`
	Amount                  float64      `json:"amount"`
	AmountCents             int64        `json:"amount_cents"`
	Currency                string       `json:"currency"`
	Status                  RefundStatus `json:"status"`
	ProviderRefundReference *string      `json:"provider_refund_reference,omitempty"`
	IdempotencyKey          *string      `json:"idempotency_key,omitempty"`
	RequestedAt             time.Time    `json:"requested_at"`
	ReviewedAt              *time.Time   `json:"reviewed_at,omitempty"`
	DecidedAt               *time.Time   `json:"decided_at,omitempty"`
	DecidedBy               *string      `json:"decided_by,omitempty"`
	CompletedAt             *time.Time   `json:"completed_at,omitempty"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
}
