package refund

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const defaultRefundWindowDays = 30

var (
	// ErrInvalidRefundInput indicates missing or invalid refund request parameters.
	ErrInvalidRefundInput = errors.New("invalid refund input")
	// ErrUnauthorized indicates that the caller is not authenticated.
	ErrUnauthorized = errors.New("authentication required")
	// ErrForbidden indicates that the authenticated caller does not have the required role.
	ErrForbidden = errors.New("forbidden: insufficient role permissions")
	// ErrOrderNotFound indicates that the order does not exist or does not belong to the caller.
	ErrOrderNotFound = errors.New("order not found")
	// ErrNoVerifiedPayment indicates that the order does not have a verified SUCCESS payment attempt.
	ErrNoVerifiedPayment = errors.New("order has no verified payment")
	// ErrRefundWindowExpired indicates that the payment completion timestamp is older than REFUND_WINDOW_DAYS.
	ErrRefundWindowExpired = errors.New("refund window expired")
	// ErrAmountExceedsBalance indicates that the requested refund exceeds the remaining refundable balance.
	ErrAmountExceedsBalance = errors.New("amount exceeds refundable balance")
	// ErrRefundAlreadyRejected indicates that a REJECTED refund cannot be approved.
	ErrRefundAlreadyRejected = errors.New("cannot approve a rejected refund")
	// ErrGatewayNotConfigured indicates that the payment provider's refund credentials are not configured.
	ErrGatewayNotConfigured = errors.New("refund payment provider not configured")
	// ErrGatewayRefundFailed indicates that the payment provider refund call failed or timed out.
	ErrGatewayRefundFailed = errors.New("payment provider refund failed")
)

// RepositoryStore defines the persistence contract required by the refund service.
type RepositoryStore interface {
	GetByID(ctx context.Context, refundID string) (*Refund, error)
	GetByIdempotencyKey(ctx context.Context, idempotencyKey string) (*Refund, error)
	FindIdenticalOpenRefund(ctx context.Context, orderID, buyerID string, amountCents int64) (*Refund, error)
	SumActiveRefundsForOrder(ctx context.Context, orderID string) (int64, error)
	Create(ctx context.Context, ref Refund) (*Refund, error)
	TransitionPendingToApproved(ctx context.Context, refundID, adminID string, decidedAt time.Time) (*Refund, bool, error)
	UpdateAfterProviderRefund(
		ctx context.Context,
		refundID string,
		status RefundStatus,
		providerRef string,
		completedAt *time.Time,
	) (*Refund, error)
	ListByBuyer(ctx context.Context, filter ListBuyerRefundsFilter) ([]Refund, int64, error)
}

// PaymentReader defines the contract for querying an order's verified payment attempt and ownership.
type PaymentReader interface {
	GetPaidAttemptForOrder(ctx context.Context, orderID string) (PaidPayment, error)
}

// RefundGateway defines the contract for executing server-to-server refunds against a payment provider.
type RefundGateway interface {
	RefundPayment(ctx context.Context, input GatewayRefundInput) (GatewayRefundOutput, error)
}

// RefundNotification holds the payload sent to the order module when a refund reaches COMPLETED.
type RefundNotification struct {
	RefundID                string
	OrderID                 string
	PaymentAttemptID        string
	AmountCents             int64
	Currency                string
	Status                  RefundStatus
	ProviderRefundReference string
}

// RefundNotifier defines the contract for notifying the order module when a refund is completed.
type RefundNotifier interface {
	NotifyRefundCompleted(ctx context.Context, n RefundNotification) error
}

// LogRefundNotifier is a default stub implementation of RefundNotifier that logs only non-sensitive identifiers.
// TODO(member-04): Replace LogRefundNotifier with the real order service implementation.
type LogRefundNotifier struct{}

// NewLogRefundNotifier creates a new stub LogRefundNotifier.
func NewLogRefundNotifier() *LogRefundNotifier {
	return &LogRefundNotifier{}
}

// NotifyRefundCompleted logs the refund ID, order ID, and status without writing directly to the orders table.
func (n *LogRefundNotifier) NotifyRefundCompleted(ctx context.Context, notif RefundNotification) error {
	// TODO(member-04): Plug in the real OrderService refund completion callback here.
	log.Printf(
		"[req_id=%s] refund.LogRefundNotifier: refund_id=%s order_id=%s status=%s",
		reqIDFromContext(ctx),
		notif.RefundID,
		notif.OrderID,
		notif.Status,
	)
	return nil
}

// AuditRecorder is an optional interface for recording administrative audit log entries without coupling to the audit package.
type AuditRecorder interface {
	RecordAdminAction(ctx context.Context, actor Actor, requestID, action, target, outcome, details string)
}

type noopTxRunner struct{}

func (noopTxRunner) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

// Service implements buyer refund requests, admin refund approval + provider processing, and buyer refund views.
type Service struct {
	repo              RepositoryStore
	txRunner          TxRunner
	payments          PaymentReader
	gateway           RefundGateway
	notifier          RefundNotifier
	auditRecorder     AuditRecorder
	refundWindow      time.Duration
	nowFn             func() time.Time
	inFlightApprovals sync.Map
}

// NewService creates a new refund service.
func NewService(repo RepositoryStore, payments PaymentReader, gateway RefundGateway) *Service {
	var runner TxRunner = noopTxRunner{}
	if tr, ok := repo.(TxRunner); ok && tr != nil {
		runner = tr
	}

	return &Service{
		repo:         repo,
		txRunner:     runner,
		payments:     payments,
		gateway:      gateway,
		notifier:     NewLogRefundNotifier(),
		refundWindow: defaultRefundWindowDays * 24 * time.Hour,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// SetRefundWindowDays configures the maximum age of a completed payment eligible for a refund request.
func (s *Service) SetRefundWindowDays(days int) {
	if days > 0 {
		s.refundWindow = time.Duration(days) * 24 * time.Hour
	}
}

// SetNotifier configures the RefundNotifier invoked when a refund reaches COMPLETED.
func (s *Service) SetNotifier(notifier RefundNotifier) {
	if notifier != nil {
		s.notifier = notifier
	}
}

// SetAuditRecorder configures an optional AuditRecorder for administrative refund actions.
func (s *Service) SetAuditRecorder(recorder AuditRecorder) {
	s.auditRecorder = recorder
}

// SetTxRunner configures the transaction runner used by refund creation.
func (s *Service) SetTxRunner(runner TxRunner) {
	if runner != nil {
		s.txRunner = runner
	}
}

// RequestRefund validates and creates a buyer refund request for a paid order, enforcing ownership,
// refund window, remaining refundable balance, and idempotency.
// It returns (refund, createdNew, error); createdNew is false when an existing idempotent refund is returned.
func (s *Service) RequestRefund(
	ctx context.Context,
	actor Actor,
	orderID string,
	input CreateRefundRequestInput,
) (*Refund, bool, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, false, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, false, ErrForbidden
	}

	trimmedOrderID := strings.TrimSpace(orderID)
	if trimmedOrderID == "" || len(trimmedOrderID) > 64 {
		return nil, false, fmt.Errorf("%w: valid order id is required", ErrInvalidRefundInput)
	}

	rawIdemKey := strings.TrimSpace(input.IdempotencyKey)
	if len(rawIdemKey) > 100 {
		return nil, false, fmt.Errorf("%w: Idempotency-Key must not exceed 100 characters", ErrInvalidRefundInput)
	}

	reason := strings.ToUpper(strings.TrimSpace(input.Reason))
	if !IsValidRefundReason(reason) {
		return nil, false, fmt.Errorf("%w: invalid refund reason", ErrInvalidRefundInput)
	}

	details := sanitizeRawDetails(input.Details)
	detailsRunes := utf8.RuneCountInString(details)
	if detailsRunes < 10 || detailsRunes > 500 {
		return nil, false, fmt.Errorf("%w: details must be between 10 and 500 characters", ErrInvalidRefundInput)
	}

	amountCents, err := ParseDecimalToCents(input.Amount)
	if err != nil || amountCents <= 0 {
		return nil, false, fmt.Errorf("%w: amount must be a positive decimal value", ErrInvalidRefundInput)
	}

	if s.payments == nil {
		return nil, false, ErrNoVerifiedPayment
	}

	paid, payErr := s.payments.GetPaidAttemptForOrder(ctx, trimmedOrderID)
	if payErr != nil {
		if errors.Is(payErr, ErrOrderNotFound) {
			return nil, false, ErrOrderNotFound
		}
		// Check ownership first if BuyerID was returned alongside ErrNoVerifiedPayment so we never leak another user's order.
		if strings.TrimSpace(paid.BuyerID) != "" && !strings.EqualFold(strings.TrimSpace(paid.BuyerID), actorID) {
			return nil, false, ErrOrderNotFound
		}
		if errors.Is(payErr, ErrNoVerifiedPayment) {
			return nil, false, ErrNoVerifiedPayment
		}
		return nil, false, payErr
	}

	// Ownership check: return 404 so another user's order existence is never leaked.
	if strings.TrimSpace(paid.BuyerID) == "" || !strings.EqualFold(strings.TrimSpace(paid.BuyerID), actorID) {
		return nil, false, ErrOrderNotFound
	}

	if paid.AttemptID == "" || paid.AmountCents <= 0 {
		return nil, false, ErrNoVerifiedPayment
	}

	now := s.nowFn()
	if !paid.CompletedAt.IsZero() && now.Sub(paid.CompletedAt) > s.refundWindow {
		return nil, false, ErrRefundWindowExpired
	}

	currency := strings.ToUpper(strings.TrimSpace(paid.Currency))
	if currency == "" {
		currency = "USD"
	}

	var scopedIdemKey *string
	if rawIdemKey != "" {
		scoped := actorID + ":" + rawIdemKey
		scopedIdemKey = &scoped
	}

	var (
		resultRefund *Refund
		wasCreated   bool
	)

	txErr := s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		if scopedIdemKey != nil {
			existing, getErr := s.repo.GetByIdempotencyKey(txCtx, *scopedIdemKey)
			if getErr != nil {
				return getErr
			}
			if existing != nil {
				resultRefund = existing
				wasCreated = false
				return nil
			}
		} else {
			existingOpen, openErr := s.repo.FindIdenticalOpenRefund(txCtx, paid.OrderID, actorID, amountCents)
			if openErr != nil {
				return openErr
			}
			if existingOpen != nil {
				resultRefund = existingOpen
				wasCreated = false
				return nil
			}
		}

		usedCents, sumErr := s.repo.SumActiveRefundsForOrder(txCtx, paid.OrderID)
		if sumErr != nil {
			return sumErr
		}
		if amountCents > (paid.AmountCents - usedCents) {
			return ErrAmountExceedsBalance
		}

		attemptIDCopy := paid.AttemptID
		var sellerIDPtr *string
		if trimmedSeller := strings.TrimSpace(paid.SellerID); trimmedSeller != "" {
			sellerIDPtr = &trimmedSeller
		}

		newRefund := Refund{
			ID:               generateRefundID(),
			OrderID:          paid.OrderID,
			PaymentAttemptID: &attemptIDCopy,
			RequestedBy:      actorID,
			SellerID:         sellerIDPtr,
			Reason:           reason,
			Details:          details,
			Amount:           float64(amountCents) / 100.0,
			AmountCents:      amountCents,
			Currency:         currency,
			Status:           RefundStatusPending,
			IdempotencyKey:   scopedIdemKey,
			RequestedAt:      now,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		created, createErr := s.repo.Create(txCtx, newRefund)
		if createErr != nil {
			if errors.Is(createErr, ErrDuplicateIdempotencyKey) && scopedIdemKey != nil {
				existing, getErr := s.repo.GetByIdempotencyKey(txCtx, *scopedIdemKey)
				if getErr != nil {
					return getErr
				}
				if existing != nil {
					resultRefund = existing
					wasCreated = false
					return nil
				}
			}
			return createErr
		}

		resultRefund = created
		wasCreated = true
		return nil
	})
	if txErr != nil {
		return nil, false, txErr
	}

	return resultRefund, wasCreated, nil
}

// ApproveRefund atomically transitions a PENDING refund to APPROVED (or resumes an already APPROVED refund
// whose previous provider call failed), invokes the payment provider's refund API using the refund ID as
// the idempotency key, updates the refund to COMPLETED or PROCESSED, records an audit log entry, and
// notifies the order service when COMPLETED.
func (s *Service) ApproveRefund(ctx context.Context, actor Actor, refundID string) (*Refund, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "ADMIN") {
		return nil, ErrForbidden
	}

	trimmedID := strings.TrimSpace(refundID)
	if trimmedID == "" || len(trimmedID) > 64 {
		return nil, ErrRefundNotFound
	}

	// Ensure only one in-flight approval/provider call executes for a given refund ID at a time.
	if _, alreadyInFlight := s.inFlightApprovals.LoadOrStore(trimmedID, struct{}{}); alreadyInFlight {
		return nil, ErrRefundTransitionConflict
	}
	defer s.inFlightApprovals.Delete(trimmedID)

	current, err := s.repo.GetByID(ctx, trimmedID)
	if err != nil {
		if errors.Is(err, ErrRefundNotFound) {
			return nil, ErrRefundNotFound
		}
		return nil, err
	}
	if current == nil {
		return nil, ErrRefundNotFound
	}

	// Calling approve on an already COMPLETED or PROCESSED refund is an idempotent no-op returning 200.
	if current.Status == RefundStatusCompleted || current.Status == RefundStatusProcessed {
		return current, nil
	}
	if current.Status == RefundStatusRejected {
		return nil, ErrRefundAlreadyRejected
	}

	now := s.nowFn()
	approved := current

	if current.Status == RefundStatusPending {
		updated, transitioned, transErr := s.repo.TransitionPendingToApproved(ctx, trimmedID, actorID, now)
		if transErr != nil {
			return nil, transErr
		}
		if !transitioned {
			reloaded, reloadErr := s.repo.GetByID(ctx, trimmedID)
			if reloadErr == nil && reloaded != nil &&
				(reloaded.Status == RefundStatusCompleted || reloaded.Status == RefundStatusProcessed) {
				return reloaded, nil
			}
			return nil, ErrRefundTransitionConflict
		}
		approved = updated
	}

	if s.gateway == nil {
		s.recordAudit(ctx, actor, "REFUND_APPROVED", "refund:"+trimmedID, "FAILURE", "refund gateway not configured")
		return nil, ErrGatewayNotConfigured
	}

	providerRef := ""
	if s.payments != nil {
		if paid, payErr := s.payments.GetPaidAttemptForOrder(ctx, approved.OrderID); payErr == nil {
			providerRef = strings.TrimSpace(paid.ProviderReference)
			if providerRef == "" {
				providerRef = strings.TrimSpace(paid.AttemptID)
			}
		}
	}
	if providerRef == "" && approved.PaymentAttemptID != nil {
		providerRef = strings.TrimSpace(*approved.PaymentAttemptID)
	}
	if providerRef == "" {
		providerRef = approved.OrderID
	}

	// Call provider refund with IdempotencyKey = refund.ID so a retry never double-refunds at the provider.
	gwOut, gwErr := s.gateway.RefundPayment(ctx, GatewayRefundInput{
		ProviderReference: providerRef,
		AmountCents:       approved.AmountCents,
		Currency:          approved.Currency,
		Reason:            approved.Reason,
		IdempotencyKey:    approved.ID,
	})
	if gwErr != nil {
		if errors.Is(gwErr, ErrGatewayNotConfigured) {
			s.recordAudit(ctx, actor, "REFUND_APPROVED", "refund:"+trimmedID, "FAILURE", "refund gateway not configured")
			return nil, ErrGatewayNotConfigured
		}
		s.recordAudit(ctx, actor, "REFUND_APPROVED", "refund:"+trimmedID, "FAILURE", "provider refund request failed")
		return nil, fmt.Errorf("%w: %v", ErrGatewayRefundFailed, gwErr)
	}

	if gwOut.Status == GatewayOutcomeFailed {
		s.recordAudit(ctx, actor, "REFUND_APPROVED", "refund:"+trimmedID, "FAILURE", "provider reported refund failure")
		return nil, ErrGatewayRefundFailed
	}

	targetStatus := RefundStatusCompleted
	var completedAtPtr *time.Time
	if gwOut.Status == GatewayOutcomePending {
		targetStatus = RefundStatusProcessed
	} else {
		completedAtPtr = &now
	}

	provRefundRef := strings.TrimSpace(gwOut.ProviderRefundReference)
	if provRefundRef == "" {
		provRefundRef = "ref-prov-" + approved.ID
	}

	finalized, updErr := s.repo.UpdateAfterProviderRefund(ctx, approved.ID, targetStatus, provRefundRef, completedAtPtr)
	if updErr != nil {
		if errors.Is(updErr, ErrDuplicateProviderRefundRef) {
			s.recordAudit(ctx, actor, "REFUND_APPROVED", "refund:"+trimmedID, "FAILURE", "duplicate provider refund reference")
			return nil, ErrDuplicateProviderRefundRef
		}
		return nil, updErr
	}

	s.recordAudit(
		ctx,
		actor,
		"REFUND_APPROVED",
		"refund:"+trimmedID,
		"SUCCESS",
		fmt.Sprintf("refund %s transitioned to %s (amount_cents=%d)", trimmedID, targetStatus, finalized.AmountCents),
	)

	if targetStatus == RefundStatusCompleted && s.notifier != nil {
		attemptID := ""
		if finalized.PaymentAttemptID != nil {
			attemptID = *finalized.PaymentAttemptID
		}
		if notifyErr := s.notifier.NotifyRefundCompleted(ctx, RefundNotification{
			RefundID:                finalized.ID,
			OrderID:                 finalized.OrderID,
			PaymentAttemptID:        attemptID,
			AmountCents:             finalized.AmountCents,
			Currency:                finalized.Currency,
			Status:                  finalized.Status,
			ProviderRefundReference: provRefundRef,
		}); notifyErr != nil {
			log.Printf("[req_id=%s] refund notifier error (refund_id=%s): %v", reqIDFromContext(ctx), finalized.ID, notifyErr)
		}
	}

	return finalized, nil
}

// ListBuyerRefunds returns a paginated list of the authenticated buyer's refunds (newest first) with status timelines.
func (s *Service) ListBuyerRefunds(
	ctx context.Context,
	actor Actor,
	rawStatus string,
	page int,
	pageSize int,
) ([]BuyerRefundView, PaginationMeta, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, PaginationMeta{}, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, PaginationMeta{}, ErrForbidden
	}

	if page < 1 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: page must be >= 1", ErrInvalidRefundInput)
	}
	if pageSize < 1 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: page_size must be >= 1", ErrInvalidRefundInput)
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var statusFilter RefundStatus
	if trimmedStatus := strings.ToUpper(strings.TrimSpace(rawStatus)); trimmedStatus != "" {
		candidate := RefundStatus(trimmedStatus)
		if !IsValidRefundStatus(candidate) {
			return nil, PaginationMeta{}, fmt.Errorf("%w: invalid refund status filter", ErrInvalidRefundInput)
		}
		statusFilter = candidate
	}

	refunds, total, err := s.repo.ListByBuyer(ctx, ListBuyerRefundsFilter{
		UserID:   actorID,
		Status:   statusFilter,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, PaginationMeta{}, err
	}

	orderNumberCache := make(map[string]string)
	views := make([]BuyerRefundView, 0, len(refunds))
	for _, ref := range refunds {
		orderNum := orderNumberCache[ref.OrderID]
		if orderNum == "" && s.payments != nil {
			if paid, payErr := s.payments.GetPaidAttemptForOrder(ctx, ref.OrderID); payErr == nil && paid.OrderNumber != "" {
				orderNum = paid.OrderNumber
				orderNumberCache[ref.OrderID] = orderNum
			}
		}
		views = append(views, toBuyerRefundView(ref, orderNum))
	}

	return views, PaginationMeta{
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

// GetBuyerRefund returns a single refund belonging to the authenticated buyer, enriched with its timeline.
// Returns ErrRefundNotFound (HTTP 404) if the refund does not exist or belongs to another user.
func (s *Service) GetBuyerRefund(ctx context.Context, actor Actor, refundID string) (*BuyerRefundView, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, ErrForbidden
	}

	trimmedID := strings.TrimSpace(refundID)
	if trimmedID == "" || len(trimmedID) > 64 {
		return nil, ErrRefundNotFound
	}

	ref, err := s.repo.GetByID(ctx, trimmedID)
	if err != nil {
		if errors.Is(err, ErrRefundNotFound) {
			return nil, ErrRefundNotFound
		}
		return nil, err
	}
	if ref == nil || !strings.EqualFold(strings.TrimSpace(ref.RequestedBy), actorID) {
		return nil, ErrRefundNotFound
	}

	var orderNum string
	if s.payments != nil {
		if paid, payErr := s.payments.GetPaidAttemptForOrder(ctx, ref.OrderID); payErr == nil {
			orderNum = paid.OrderNumber
		}
	}

	view := toBuyerRefundView(*ref, orderNum)
	return &view, nil
}

func (s *Service) recordAudit(ctx context.Context, actor Actor, action, target, outcome, details string) {
	if s.auditRecorder != nil {
		s.auditRecorder.RecordAdminAction(ctx, actor, reqIDFromContext(ctx), action, target, outcome, details)
	}
}

func toBuyerRefundView(ref Refund, orderNumber string) BuyerRefundView {
	resolvedOrderNum := strings.TrimSpace(orderNumber)
	if resolvedOrderNum == "" {
		resolvedOrderNum = ref.OrderID
	}

	decidedAt := ref.DecidedAt
	if decidedAt == nil {
		decidedAt = ref.ReviewedAt
	}

	return BuyerRefundView{
		ID:          ref.ID,
		OrderID:     ref.OrderID,
		OrderNumber: resolvedOrderNum,
		Status:      ref.Status,
		Reason:      ref.Reason,
		Details:     ref.Details,
		AmountCents: ref.AmountCents,
		Amount:      FormatCentsDecimal(ref.AmountCents),
		Currency:    ref.Currency,
		RequestedAt: ref.RequestedAt,
		DecidedAt:   decidedAt,
		CompletedAt: ref.CompletedAt,
		Timeline:    BuildRefundTimeline(ref),
	}
}

// sanitizeRawDetails strips non-printable control characters (except newlines and tabs)
// and trims surrounding whitespace without HTML-escaping user text.
func sanitizeRawDetails(val string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, val)
	return strings.TrimSpace(cleaned)
}

func generateRefundID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("ref-%d", time.Now().UnixNano())
	}
	return "ref-" + hex.EncodeToString(b)
}
