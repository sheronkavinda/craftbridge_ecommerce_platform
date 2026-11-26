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
	// ErrForbidden indicates that the authenticated caller does not have the required role.
	ErrForbidden = errors.New("forbidden: insufficient role permissions")
	// ErrOrderNotPayable indicates that the order is not in a pending-payment status.
	ErrOrderNotPayable = errors.New("order is not pending payment")
	// ErrUnknownProvider indicates that a webhook was received for an unregistered payment provider.
	ErrUnknownProvider = errors.New("unknown payment provider")
	// ErrAmountMismatch indicates that a webhook's amount or currency does not match the stored payment attempt.
	ErrAmountMismatch = errors.New("payment amount or currency mismatch")
)

// RepositoryStore defines the persistence contract required by the payment service.
type RepositoryStore interface {
	GetLatestPendingAttemptByOrder(ctx context.Context, orderID string) (*PaymentAttempt, error)
	CreateAttempt(ctx context.Context, attempt PaymentAttempt) (*PaymentAttempt, error)
	GetAttemptByID(ctx context.Context, attemptID string) (*PaymentAttempt, error)
	UpdateAttemptStatus(
		ctx context.Context,
		attemptID string,
		status PaymentStatus,
		providerRef *string,
		failureReason *string,
		completedAt *time.Time,
	) (*PaymentAttempt, error)
	ClaimPendingAttemptsForReconcile(ctx context.Context, olderThan time.Time, limit int) ([]PaymentAttempt, error)
	ListAttemptsByUser(ctx context.Context, filter ListBuyerPaymentsFilter) ([]PaymentAttempt, int64, error)
	InsertEventIfNew(ctx context.Context, event PaymentEvent) (bool, *time.Time, error)
	MarkEventProcessed(ctx context.Context, provider, providerEventID string, processedAt time.Time) error
}

// OrderReader defines the contract for reading order payment metadata without coupling to Member 04's internal types.
type OrderReader interface {
	GetOrderForPayment(ctx context.Context, orderID string) (OrderForPayment, error)
}

type noopTxRunner struct{}

func (noopTxRunner) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

// Service implements payment initiation, idempotency/reuse of recent pending attempts, webhook verification,
// reconciliation of timed-out pending attempts, buyer payment history queries, and seller earnings calculations.
type Service struct {
	repo              RepositoryStore
	txRunner          TxRunner
	orders            OrderReader
	salesReader       SellerSalesReader
	provider          PaymentProvider
	providers         map[string]PaymentProvider
	notifier          OrderNotifier
	auditRecorder     AuditRecorder
	reconcileAfter    time.Duration
	attemptTTL        time.Duration
	commissionPercent int
	payoutHoldDays    int
	nowFn             func() time.Time
}

// NewService creates a new payment service.
func NewService(repo RepositoryStore, orders OrderReader, provider PaymentProvider) *Service {
	var runner TxRunner = noopTxRunner{}
	if tr, ok := repo.(TxRunner); ok && tr != nil {
		runner = tr
	}

	var sales SellerSalesReader
	if sr, ok := repo.(SellerSalesReader); ok && sr != nil {
		sales = sr
	}

	providers := make(map[string]PaymentProvider)
	if provider != nil {
		name := strings.ToLower(strings.TrimSpace(provider.Name()))
		if name != "" {
			providers[name] = provider
		}
		if name == "payhere_sandbox" {
			providers["payhere"] = provider
		}
	}

	return &Service{
		repo:              repo,
		txRunner:          runner,
		orders:            orders,
		salesReader:       sales,
		provider:          provider,
		providers:         providers,
		notifier:          NewLogOrderNotifier(),
		reconcileAfter:    defaultReconcileAfterMinutes * time.Minute,
		attemptTTL:        defaultAttemptTTLHours * time.Hour,
		commissionPercent: defaultPlatformCommissionPercent,
		payoutHoldDays:    defaultPayoutHoldDays,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// SetSellerSalesReader configures the SellerSalesReader used by GetSellerEarnings.
func (s *Service) SetSellerSalesReader(reader SellerSalesReader) {
	s.salesReader = reader
}

// SetEarningsConfig configures the platform commission percentage and payout hold window in days.
func (s *Service) SetEarningsConfig(commissionPercent, payoutHoldDays int) {
	if commissionPercent >= 0 {
		s.commissionPercent = commissionPercent
	}
	if payoutHoldDays >= 0 {
		s.payoutHoldDays = payoutHoldDays
	}
}

// SetNotifier configures the OrderNotifier used to notify the order service after verified webhooks or reconciliations.
func (s *Service) SetNotifier(notifier OrderNotifier) {
	if notifier != nil {
		s.notifier = notifier
	}
}

// SetTxRunner configures the transaction runner used by webhook and reconciliation processing.
func (s *Service) SetTxRunner(runner TxRunner) {
	if runner != nil {
		s.txRunner = runner
	}
}

// SetReconcileConfig configures the age threshold before PENDING attempts are reconciled and their expiration TTL.
func (s *Service) SetReconcileConfig(reconcileAfter, attemptTTL time.Duration) {
	if reconcileAfter > 0 {
		s.reconcileAfter = reconcileAfter
	}
	if attemptTTL > 0 {
		s.attemptTTL = attemptTTL
	}
}

// SetAuditRecorder configures an optional AuditRecorder for administrative payment operations.
func (s *Service) SetAuditRecorder(recorder AuditRecorder) {
	s.auditRecorder = recorder
}

// RegisterProvider registers a PaymentProvider in the webhook provider registry by name.
func (s *Service) RegisterProvider(name string, provider PaymentProvider) {
	if s.providers == nil {
		s.providers = make(map[string]PaymentProvider)
	}
	key := strings.ToLower(strings.TrimSpace(name))
	if key != "" && provider != nil {
		s.providers[key] = provider
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

	now := s.nowFn()

	// Order must be awaiting payment (status pending_payment/pending, payment_status pending when set, and not expired).
	normalizedStatus := strings.ToUpper(strings.TrimSpace(order.Status))
	if normalizedStatus != "PENDING" && normalizedStatus != "PENDING_PAYMENT" {
		return nil, fmt.Errorf("%w: current order status is %s", ErrOrderNotPayable, normalizedStatus)
	}
	normalizedPayStatus := strings.ToUpper(strings.TrimSpace(order.PaymentStatus))
	if normalizedPayStatus != "" && normalizedPayStatus != "PENDING" {
		return nil, fmt.Errorf("%w: current payment status is %s", ErrOrderNotPayable, normalizedPayStatus)
	}
	if order.ExpiresAt != nil && !now.Before(*order.ExpiresAt) {
		return nil, fmt.Errorf("%w: order payment window has expired", ErrOrderNotPayable)
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

// ListBuyerPayments returns a paginated list of the authenticated buyer's payment attempts (newest first).
func (s *Service) ListBuyerPayments(
	ctx context.Context,
	actor Actor,
	rawStatus string,
	page int,
	pageSize int,
) ([]BuyerPaymentItem, PaginationMeta, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, PaginationMeta{}, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, PaginationMeta{}, ErrForbidden
	}

	if page < 1 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: page must be >= 1", ErrInvalidPaymentInput)
	}
	if pageSize < 1 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: page_size must be >= 1", ErrInvalidPaymentInput)
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var statusFilter PaymentStatus
	if trimmedStatus := strings.ToUpper(strings.TrimSpace(rawStatus)); trimmedStatus != "" {
		candidate := PaymentStatus(trimmedStatus)
		if !IsValidPaymentStatus(candidate) {
			return nil, PaginationMeta{}, fmt.Errorf("%w: invalid payment status filter", ErrInvalidPaymentInput)
		}
		statusFilter = candidate
	}

	attempts, total, err := s.repo.ListAttemptsByUser(ctx, ListBuyerPaymentsFilter{
		UserID:   actorID,
		Status:   statusFilter,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, PaginationMeta{}, err
	}

	orderCache := make(map[string]OrderForPayment)
	items := make([]BuyerPaymentItem, 0, len(attempts))
	for _, att := range attempts {
		var ord OrderForPayment
		if cached, ok := orderCache[att.OrderID]; ok {
			ord = cached
		} else if s.orders != nil {
			if loaded, ordErr := s.orders.GetOrderForPayment(ctx, att.OrderID); ordErr == nil {
				ord = loaded
				orderCache[att.OrderID] = loaded
			}
		}

		items = append(items, toBuyerPaymentItem(att, ord.OrderNumber, ""))
	}

	return items, PaginationMeta{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// GetBuyerPayment returns a single payment attempt belonging to the authenticated buyer, enriched with order summary fields.
// If the attempt does not exist or belongs to another user, ErrAttemptNotFound (HTTP 404) is returned without leaking existence.
func (s *Service) GetBuyerPayment(ctx context.Context, actor Actor, attemptID string) (*BuyerPaymentItem, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, ErrForbidden
	}

	trimmedID := strings.TrimSpace(attemptID)
	if trimmedID == "" || len(trimmedID) > 64 {
		return nil, ErrAttemptNotFound
	}

	att, err := s.repo.GetAttemptByID(ctx, trimmedID)
	if err != nil {
		if errors.Is(err, ErrAttemptNotFound) {
			return nil, ErrAttemptNotFound
		}
		return nil, err
	}
	if att == nil || !strings.EqualFold(strings.TrimSpace(att.UserID), actorID) {
		return nil, ErrAttemptNotFound
	}

	var orderNumber, orderStatus string
	if s.orders != nil {
		if ord, ordErr := s.orders.GetOrderForPayment(ctx, att.OrderID); ordErr == nil {
			orderNumber = ord.OrderNumber
			orderStatus = ord.Status
		}
	}

	item := toBuyerPaymentItem(*att, orderNumber, orderStatus)
	return &item, nil
}

// TriggerAdminReconcile validates the admin caller, runs a single pending-payment reconciliation pass,
// and records an audit trail entry if an AuditRecorder is configured.
func (s *Service) TriggerAdminReconcile(ctx context.Context, actor Actor) (*ReconcileSummary, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "ADMIN") {
		return nil, ErrForbidden
	}

	summary, err := s.ReconcilePendingAttempts(ctx, s.nowFn())
	if err != nil {
		if s.auditRecorder != nil {
			s.auditRecorder.RecordAdminAction(
				ctx,
				actor,
				reqIDFromContext(ctx),
				"PAYMENT_RECONCILE_RUN",
				"payment_attempts:pending",
				"FAILURE",
				"admin payment reconciliation pass failed",
			)
		}
		return nil, err
	}

	if s.auditRecorder != nil && summary != nil {
		details := fmt.Sprintf(
			"reconcile pass completed: checked=%d succeeded=%d failed=%d cancelled=%d expired=%d errors=%d",
			summary.Checked,
			summary.Succeeded,
			summary.Failed,
			summary.Cancelled,
			summary.Expired,
			summary.Errors,
		)
		s.auditRecorder.RecordAdminAction(
			ctx,
			actor,
			reqIDFromContext(ctx),
			"PAYMENT_RECONCILE_RUN",
			"payment_attempts:pending",
			"SUCCESS",
			details,
		)
	}

	return summary, nil
}

func toBuyerPaymentItem(att PaymentAttempt, orderNumber, orderStatus string) BuyerPaymentItem {
	resolvedOrderNum := strings.TrimSpace(orderNumber)
	if resolvedOrderNum == "" {
		resolvedOrderNum = att.OrderID
	}

	return BuyerPaymentItem{
		ID:                att.ID,
		OrderID:           att.OrderID,
		OrderNumber:       resolvedOrderNum,
		OrderStatus:       strings.TrimSpace(orderStatus),
		Status:            att.Status,
		AmountCents:       att.AmountCents,
		Amount:            FormatCentsDecimal(att.AmountCents),
		Currency:          att.Currency,
		Provider:          att.Provider,
		ProviderReference: att.ProviderReference,
		PublicReason:      PublicFailureReason(att.Status),
		CreatedAt:         att.CreatedAt,
		CompletedAt:       att.CompletedAt,
	}
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
