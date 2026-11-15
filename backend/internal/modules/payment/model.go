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
	OrderID     string
	OrderNumber string
	UserID      string
	AmountCents int64
	Currency    string
	Status      string
	BuyerName   string
	BuyerEmail  string
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
