package payment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const pendingAttemptReuseWindow = 15 * time.Minute

var (
	// ErrInvalidPaymentInput indicates malformed or missing parameters.
	ErrInvalidPaymentInput = errors.New("invalid payment input")
	// ErrUnauthorized indicates that the caller is not authenticated.
	ErrUnauthorized = errors.New("authentication required")
	// ErrForbidden indicates that the authenticated caller does not have the required buyer role.
	ErrForbidden = errors.New("forbidden: buyer role required")
	// ErrOrderNotPayable indicates that the order is not in a pending-payment status.
	ErrOrderNotPayable = errors.New("order is not pending payment")
)

// RepositoryStore defines the persistence contract required by the payment service.
type RepositoryStore interface {
	GetLatestPendingAttemptByOrder(ctx context.Context, orderID string) (*PaymentAttempt, error)
	CreateAttempt(ctx context.Context, attempt PaymentAttempt) (*PaymentAttempt, error)
}

// OrderReader defines the contract for reading order payment metadata without coupling to Member 04's internal types.
type OrderReader interface {
	GetOrderForPayment(ctx context.Context, orderID string) (OrderForPayment, error)
}

// Service implements payment initiation, idempotency/reuse of recent pending attempts, and gateway delegation.
type Service struct {
	repo     RepositoryStore
	orders   OrderReader
	provider PaymentProvider
	nowFn    func() time.Time
}

// NewService creates a new payment service.
func NewService(repo RepositoryStore, orders OrderReader, provider PaymentProvider) *Service {
	return &Service{
		repo:     repo,
		orders:   orders,
		provider: provider,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// InitiatePayment validates the buyer and order state, reuses a recent (<15m) PENDING attempt if present,
// or creates a new payment_attempts record and returns the provider checkout descriptor.
func (s *Service) InitiatePayment(ctx context.Context, actor Actor, orderID string) (*InitiatePaymentResponse, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, ErrForbidden
	}

	trimmedOrderID := strings.TrimSpace(orderID)
	if trimmedOrderID == "" || len(trimmedOrderID) > 64 {
		return nil, fmt.Errorf("%w: valid order id is required", ErrInvalidPaymentInput)
	}

	if s.orders == nil {
		return nil, errors.New("order reader is not configured")
	}
	if s.provider == nil {
		return nil, ErrProviderNotConfigured
	}

	order, err := s.orders.GetOrderForPayment(ctx, trimmedOrderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	// Ownership check: do not leak the existence of another user's order; return 404.
	if strings.TrimSpace(order.UserID) == "" || !strings.EqualFold(strings.TrimSpace(order.UserID), actorID) {
		return nil, ErrOrderNotFound
	}

	// Order status must be PENDING or PENDING_PAYMENT.
	normalizedStatus := strings.ToUpper(strings.TrimSpace(order.Status))
	if normalizedStatus != "PENDING" && normalizedStatus != "PENDING_PAYMENT" {
		return nil, fmt.Errorf("%w: current order status is %s", ErrOrderNotPayable, normalizedStatus)
	}

	if order.AmountCents <= 0 {
		return nil, fmt.Errorf("%w: order amount must be positive", ErrOrderNotPayable)
	}

	currency := strings.ToUpper(strings.TrimSpace(order.Currency))
	if currency == "" {
		currency = "USD"
	}

	firstName, lastName := splitFullName(firstNonEmpty(order.BuyerName, actor.Name))
	buyerEmail := firstNonEmpty(order.BuyerEmail, "buyer@craftbridge.local")

	now := s.nowFn()
	existing, err := s.repo.GetLatestPendingAttemptByOrder(ctx, order.OrderID)
	if err != nil {
		return nil, err
	}

	if existing != nil && existing.Status == PaymentStatusPending && now.Sub(existing.CreatedAt) < pendingAttemptReuseWindow {
		attemptRef := existing.ID
		if existing.ProviderReference != nil && strings.TrimSpace(*existing.ProviderReference) != "" {
			attemptRef = strings.TrimSpace(*existing.ProviderReference)
		}

		initRes, provErr := s.provider.InitiatePayment(ctx, InitiateRequest{
			AttemptID:   attemptRef,
			OrderID:     order.OrderID,
			OrderNumber: order.OrderNumber,
			AmountCents: existing.AmountCents,
			Currency:    existing.Currency,
			Items:       "Order " + firstNonEmpty(order.OrderNumber, order.OrderID),
			FirstName:   firstName,
			LastName:    lastName,
			Email:       buyerEmail,
		})
		if provErr != nil {
			return nil, provErr
		}

		return &InitiatePaymentResponse{
			AttemptID: existing.ID,
			Status:    existing.Status,
			Checkout: CheckoutDescriptor{
				CheckoutURL: initRes.CheckoutURL,
				Method:      initRes.Method,
				Fields:      initRes.Fields,
			},
		}, nil
	}

	attemptID := generateAttemptID()
	initRes, provErr := s.provider.InitiatePayment(ctx, InitiateRequest{
		AttemptID:   attemptID,
		OrderID:     order.OrderID,
		OrderNumber: order.OrderNumber,
		AmountCents: order.AmountCents,
		Currency:    currency,
		Items:       "Order " + firstNonEmpty(order.OrderNumber, order.OrderID),
		FirstName:   firstName,
		LastName:    lastName,
		Email:       buyerEmail,
	})
	if provErr != nil {
		return nil, provErr
	}

	providerRef := strings.TrimSpace(initRes.ProviderReference)
	if providerRef == "" {
		providerRef = attemptID
	}

	created, err := s.repo.CreateAttempt(ctx, PaymentAttempt{
		ID:                attemptID,
		OrderID:           order.OrderID,
		UserID:            actorID,
		Provider:          s.provider.Name(),
		ProviderReference: &providerRef,
		AmountCents:       order.AmountCents,
		Currency:          currency,
		Status:            PaymentStatusPending,
	})
	if err != nil {
		return nil, err
	}

	return &InitiatePaymentResponse{
		AttemptID: created.ID,
		Status:    created.Status,
		Checkout: CheckoutDescriptor{
			CheckoutURL: initRes.CheckoutURL,
			Method:      initRes.Method,
			Fields:      initRes.Fields,
		},
	}, nil
}

func generateAttemptID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("att-%d", time.Now().UnixNano())
	}
	return "att-" + hex.EncodeToString(b)
}

func splitFullName(fullName string) (firstName, lastName string) {
	trimmed := strings.TrimSpace(fullName)
	if trimmed == "" {
		return "CraftBridge", "Buyer"
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 1 {
		return parts[0], "Buyer"
	}
	return parts[0], strings.Join(parts[1:], " ")
}
