package payment

import (
	"context"
	"errors"
	"net/http"
)

// HTTPDoer abstracts an HTTP client for provider API requests so tests can inject httptest.Server clients.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

var (
	// ErrNotImplemented is returned when a provider method is not yet implemented.
	ErrNotImplemented = errors.New("payment provider operation not implemented")
	// ErrProviderNotConfigured is returned when required payment gateway environment variables are missing.
	ErrProviderNotConfigured = errors.New("payment provider not configured")
	// ErrInvalidSignature is returned when a webhook notification fails cryptographic signature or merchant verification.
	ErrInvalidSignature = errors.New("invalid webhook signature")
	// ErrMalformedNotification is returned when a webhook notification is missing required fields or has invalid values.
	ErrMalformedNotification = errors.New("malformed webhook notification")
)

// InitiateRequest contains the order and buyer metadata needed to build a provider checkout session.
type InitiateRequest struct {
	AttemptID   string
	OrderID     string
	OrderNumber string
	AmountCents int64
	Currency    string
	Items       string
	FirstName   string
	LastName    string
	Email       string
	Phone       string
	Address     string
	City        string
	Country     string
}

// InitiateResult contains the redirect/form parameters returned by a payment provider when initiating checkout.
type InitiateResult struct {
	ProviderReference string            `json:"-"`
	CheckoutURL       string            `json:"checkout_url"`
	Method            string            `json:"method"`
	Fields            map[string]string `json:"fields"`
}

// NotificationInput holds raw webhook/callback request data for provider signature verification.
type NotificationInput struct {
	Headers map[string]string
	Form    map[string]string
	RawBody []byte
}

// VerifiedEvent represents a cryptographically verified webhook notification from a payment provider.
type VerifiedEvent struct {
	Provider          string
	ProviderEventID   string
	AttemptID         string
	ProviderReference string
	EventType         string
	Status            PaymentStatus
	AmountCents       int64
	Currency          string
	SignatureValid    bool
	FailureReason     string
	RawPayload        []byte
}

// ProviderStatus represents the payment status reported by a provider status query endpoint.
type ProviderStatus struct {
	ProviderReference string
	Status            PaymentStatus
	AmountCents       int64
	Currency          string
	FailureReason     string
}

// PaymentProvider defines the contract implemented by external payment gateway adapters.
type PaymentProvider interface {
	Name() string
	InitiatePayment(ctx context.Context, req InitiateRequest) (InitiateResult, error)
	VerifyNotification(input NotificationInput) (VerifiedEvent, error)
	QueryStatus(ctx context.Context, providerRef string) (ProviderStatus, error)
}
