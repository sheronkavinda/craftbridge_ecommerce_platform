package payment

import (
	"encoding/json"
	"time"
)

// PaymentStatus represents the lifecycle state of a payment attempt.
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "PENDING"
	PaymentStatusSuccess   PaymentStatus = "SUCCESS"
	PaymentStatusFailed    PaymentStatus = "FAILED"
	PaymentStatusExpired   PaymentStatus = "EXPIRED"
	PaymentStatusCancelled PaymentStatus = "CANCELLED"
)

// IsValidPaymentStatus reports whether s is a known PaymentStatus value.
func IsValidPaymentStatus(s PaymentStatus) bool {
	switch s {
	case PaymentStatusPending, PaymentStatusSuccess, PaymentStatusFailed, PaymentStatusExpired, PaymentStatusCancelled:
		return true
	default:
		return false
	}
}

// PublicFailureReason returns a sanitized short public reason for non-successful terminal payment statuses.
// Internal failure_reason strings from payment_attempts are never exposed directly to buyers.
func PublicFailureReason(status PaymentStatus) string {
	switch status {
	case PaymentStatusFailed:
		return "payment_failed"
	case PaymentStatusCancelled:
		return "cancelled"
	case PaymentStatusExpired:
		return "expired"
	default:
		return ""
	}
}

// PaymentAttempt represents a single checkout payment attempt initiated for an order.
type PaymentAttempt struct {
	ID                string        `json:"id"`
	OrderID           string        `json:"order_id"`
	UserID            string        `json:"user_id"`
	Provider          string        `json:"provider"`
	ProviderReference *string       `json:"provider_reference,omitempty"`
	AmountCents       int64         `json:"amount_cents"`
	Currency          string        `json:"currency"`
	Status            PaymentStatus `json:"status"`
	FailureReason     *string       `json:"failure_reason,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	CompletedAt       *time.Time    `json:"completed_at,omitempty"`
}

// PaymentEvent represents an incoming provider webhook or callback event stored for idempotency and auditing.
type PaymentEvent struct {
	ID              string          `json:"id"`
	Provider        string          `json:"provider"`
	ProviderEventID string          `json:"provider_event_id"`
	AttemptID       *string         `json:"attempt_id,omitempty"`
	EventType       string          `json:"event_type"`
	Payload         json.RawMessage `json:"payload"`
	SignatureValid  bool            `json:"signature_valid"`
	ProcessedAt     *time.Time      `json:"processed_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// Actor represents the authenticated caller initiating a payment operation.
type Actor struct {
	ID   string
	Name string
	Role string
}

// OrderForPayment holds the order fields needed to validate and initiate a payment attempt.
type OrderForPayment struct {
	OrderID       string
	OrderNumber   string
	UserID        string
	SellerID      string
	AmountCents   int64
	Currency      string
	Status        string
	PaymentStatus string
	ExpiresAt     *time.Time
	BuyerName     string
	BuyerEmail    string
}

// CheckoutDescriptor represents the checkout redirect/form descriptor returned to the client.
type CheckoutDescriptor struct {
	CheckoutURL string            `json:"checkout_url"`
	Method      string            `json:"method"`
	Fields      map[string]string `json:"fields"`
}

// InitiatePaymentResponse represents the payload returned by POST /api/v1/orders/:id/payments.
type InitiatePaymentResponse struct {
	AttemptID string             `json:"attempt_id"`
	Status    PaymentStatus      `json:"status"`
	Checkout  CheckoutDescriptor `json:"checkout"`
}

// WebhookResponse represents the payload returned by POST /api/v1/webhooks/payments/:provider.
type WebhookResponse struct {
	Status string `json:"status"`
}

// ReconcileSummary reports the outcome counts of a pending-payment reconciliation pass.
type ReconcileSummary struct {
	Checked   int `json:"checked"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Cancelled int `json:"cancelled"`
	Expired   int `json:"expired"`
	Errors    int `json:"errors"`
}

// ListBuyerPaymentsFilter specifies pagination and status filtering for a buyer's payment history.
type ListBuyerPaymentsFilter struct {
	UserID   string
	Status   PaymentStatus
	Page     int
	PageSize int
}

// PaginationMeta holds pagination metadata for list responses.
type PaginationMeta struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// BuyerPaymentItem represents a single payment attempt in the buyer's payment history list/detail responses.
// Raw event payloads, cryptographic signatures, and internal failure_reason details are never included.
type BuyerPaymentItem struct {
	ID                string        `json:"id"`
	OrderID           string        `json:"order_id"`
	OrderNumber       string        `json:"order_number"`
	OrderStatus       string        `json:"order_status,omitempty"`
	Status            PaymentStatus `json:"status"`
	AmountCents       int64         `json:"amount_cents"`
	Amount            string        `json:"amount"`
	Currency          string        `json:"currency"`
	Provider          string        `json:"provider"`
	ProviderReference *string       `json:"provider_reference,omitempty"`
	PublicReason      string        `json:"public_reason,omitempty"`
	CreatedAt         time.Time     `json:"created_at"`
	CompletedAt       *time.Time    `json:"completed_at,omitempty"`
}
