package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// PaymentResult represents the verified payment outcome delivered to the order service after a webhook commits.
type PaymentResult struct {
	OrderID           string
	AttemptID         string
	Outcome           PaymentStatus
	AmountCents       int64
	Currency          string
	ProviderReference string
	Late              bool
}

// OrderNotifier defines the contract for notifying the order module when a payment attempt reaches an outcome.
type OrderNotifier interface {
	NotifyPaymentResult(ctx context.Context, result PaymentResult) error
}

// LogOrderNotifier is a default stub implementation of OrderNotifier that logs only the order ID and outcome.
// TODO(member-04): Replace LogOrderNotifier with the real order service implementation to transition orders.status.
type LogOrderNotifier struct{}

// NewLogOrderNotifier creates a new stub LogOrderNotifier.
func NewLogOrderNotifier() *LogOrderNotifier {
	return &LogOrderNotifier{}
}

// NotifyPaymentResult logs the order ID and payment outcome without writing directly to the orders table or logging secrets.
func (n *LogOrderNotifier) NotifyPaymentResult(ctx context.Context, result PaymentResult) error {
	// TODO(member-04): Plug in the real OrderService payment callback handler here.
	log.Printf(
		"[req_id=%s] payment.LogOrderNotifier: order_id=%s outcome=%s late=%t",
		reqIDFromContext(ctx),
		result.OrderID,
		result.Outcome,
		result.Late,
	)
	return nil
}

// TransitionAttemptStatus is a pure state-machine function for payment_attempts.status transitions:
//   - PENDING -> SUCCESS | FAILED | CANCELLED | EXPIRED
//   - FAILED / CANCELLED / EXPIRED -> SUCCESS is allowed (late or retried payment confirmed by provider)
//   - SUCCESS -> anything else is ignored (out-of-order webhook must never downgrade SUCCESS)
//   - PENDING (webhook) on a non-PENDING attempt -> ignored
//   - Empty incoming status (e.g. CHARGEBACK -3) -> ignored
func TransitionAttemptStatus(current PaymentStatus, incoming PaymentStatus) (next PaymentStatus, changed bool, late bool) {
	if incoming == "" {
		return current, false, false
	}

	switch current {
	case PaymentStatusSuccess:
		// Out-of-order protection: never downgrade a completed SUCCESS payment.
		return PaymentStatusSuccess, false, false

	case PaymentStatusPending:
		switch incoming {
		case PaymentStatusSuccess, PaymentStatusFailed, PaymentStatusCancelled, PaymentStatusExpired:
			return incoming, true, false
		default:
			return PaymentStatusPending, false, false
		}

	case PaymentStatusFailed, PaymentStatusCancelled:
		if incoming == PaymentStatusSuccess {
			return PaymentStatusSuccess, true, false
		}
		return current, false, false

	case PaymentStatusExpired:
		if incoming == PaymentStatusSuccess {
			return PaymentStatusSuccess, true, true
		}
		return current, false, false

	default:
		return current, false, false
	}
}

// HandleWebhook verifies an incoming provider webhook notification, enforces amount/currency matching
// and idempotency, transitions the payment attempt state machine, and notifies the order service.
func (s *Service) HandleWebhook(ctx context.Context, providerName string, input NotificationInput) (*WebhookResponse, error) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(providerName))
	provider, exists := s.providers[normalizedProvider]
	if !exists || provider == nil {
		return nil, ErrUnknownProvider
	}

	verified, err := provider.VerifyNotification(input)
	if err != nil {
		if errors.Is(err, ErrInvalidSignature) {
			attemptHint := ""
			if input.Form != nil {
				attemptHint = strings.TrimSpace(input.Form["order_id"])
			}
			log.Printf(
				"[req_id=%s] payment webhook warning: invalid signature (provider=%s, attempt_id=%s)",
				reqIDFromContext(ctx),
				provider.Name(),
				attemptHint,
			)
		}
		return nil, err
	}

	attempt, err := s.repo.GetAttemptByID(ctx, verified.AttemptID)
	if err != nil {
		if errors.Is(err, ErrAttemptNotFound) {
			return nil, ErrAttemptNotFound
		}
		return nil, err
	}
	if attempt == nil {
		return nil, ErrAttemptNotFound
	}

	now := s.nowFn()
	attemptIDCopy := attempt.ID
	payloadJSON := json.RawMessage(verified.RawPayload)
	if len(payloadJSON) == 0 {
		payloadJSON = json.RawMessage(`{}`)
	}

	// Amount and currency verification against the stored attempt.
	currencyMatch := strings.EqualFold(strings.TrimSpace(verified.Currency), strings.TrimSpace(attempt.Currency))
	amountMatch := verified.AmountCents == attempt.AmountCents
	if !currencyMatch || !amountMatch {
		mismatchReason := "amount_mismatch"
		mismatchEventType := "AMOUNT_MISMATCH"
		if !currencyMatch {
			mismatchReason = "currency_mismatch"
			mismatchEventType = "CURRENCY_MISMATCH"
		}

		txErr := s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
			_, _, insErr := s.repo.InsertEventIfNew(txCtx, PaymentEvent{
				ID:              generateEventID(),
				Provider:        verified.Provider,
				ProviderEventID: verified.ProviderEventID,
				AttemptID:       &attemptIDCopy,
				EventType:       mismatchEventType,
				Payload:         payloadJSON,
				SignatureValid:  true,
				ProcessedAt:     &now,
				CreatedAt:       now,
			})
			if insErr != nil {
				return insErr
			}
			// Keep attempt status unchanged; record the mismatch failure reason.
			_, updErr := s.repo.UpdateAttemptStatus(
				txCtx,
				attempt.ID,
				attempt.Status,
				nil,
				&mismatchReason,
				attempt.CompletedAt,
			)
			return updErr
		})
		if txErr != nil {
			return nil, txErr
		}

		return nil, fmt.Errorf("%w: %s", ErrAmountMismatch, mismatchReason)
	}

	nextStatus, changed, late := TransitionAttemptStatus(attempt.Status, verified.Status)

	// Check if the order is no longer awaiting payment when transitioning to SUCCESS (or retrying a late SUCCESS).
	if (nextStatus == PaymentStatusSuccess || attempt.Status == PaymentStatusSuccess) && s.orders != nil {
		if ord, ordErr := s.orders.GetOrderForPayment(ctx, attempt.OrderID); ordErr == nil {
			ordStatus := strings.ToUpper(strings.TrimSpace(ord.Status))
			if ordStatus != "" && ordStatus != "PENDING" && ordStatus != "PENDING_PAYMENT" {
				late = true
			}
		}
	}

	eventType := verified.EventType
	if late && nextStatus == PaymentStatusSuccess {
		eventType = "LATE_PAYMENT"
	}

	var (
		isDuplicateAlreadyProcessed bool
		isDuplicateRetryNotify      bool
		shouldNotify                bool
		wasIgnored                  bool
		finalAttempt                = attempt
	)

	txErr := s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		inserted, existingProcessedAt, insErr := s.repo.InsertEventIfNew(txCtx, PaymentEvent{
			ID:              generateEventID(),
			Provider:        verified.Provider,
			ProviderEventID: verified.ProviderEventID,
			AttemptID:       &attemptIDCopy,
			EventType:       eventType,
			Payload:         payloadJSON,
			SignatureValid:  true,
			ProcessedAt:     nil,
			CreatedAt:       now,
		})
		if insErr != nil {
			return insErr
		}

		if !inserted {
			if existingProcessedAt != nil {
				isDuplicateAlreadyProcessed = true
				return nil
			}
			isDuplicateRetryNotify = true
			return nil
		}

		if !changed {
			wasIgnored = true
			return s.repo.MarkEventProcessed(txCtx, verified.Provider, verified.ProviderEventID, now)
		}

		var providerRefPtr *string
		if strings.TrimSpace(verified.ProviderReference) != "" {
			ref := strings.TrimSpace(verified.ProviderReference)
			providerRefPtr = &ref
		}

		var failureReasonPtr *string
		if (nextStatus == PaymentStatusFailed || nextStatus == PaymentStatusCancelled) && strings.TrimSpace(verified.FailureReason) != "" {
			reason := strings.TrimSpace(verified.FailureReason)
			failureReasonPtr = &reason
		}

		var completedAtPtr *time.Time
		if nextStatus == PaymentStatusSuccess {
			completedAtPtr = &now
		}

		updated, updErr := s.repo.UpdateAttemptStatus(
			txCtx,
			attempt.ID,
			nextStatus,
			providerRefPtr,
			failureReasonPtr,
			completedAtPtr,
		)
		if updErr != nil {
			return updErr
		}
		finalAttempt = updated

		if isNotifierOutcome(nextStatus) {
			shouldNotify = true
			return nil
		}

		return s.repo.MarkEventProcessed(txCtx, verified.Provider, verified.ProviderEventID, now)
	})
	if txErr != nil {
		return nil, txErr
	}

	if isDuplicateAlreadyProcessed {
		return &WebhookResponse{Status: "duplicate"}, nil
	}
	if wasIgnored {
		return &WebhookResponse{Status: "ignored"}, nil
	}

	if shouldNotify || isDuplicateRetryNotify {
		outcome := nextStatus
		if isDuplicateRetryNotify {
			if isNotifierOutcome(finalAttempt.Status) {
				outcome = finalAttempt.Status
			} else if isNotifierOutcome(verified.Status) {
				outcome = verified.Status
			}
		}

		provRef := strings.TrimSpace(verified.ProviderReference)
		if provRef == "" && finalAttempt.ProviderReference != nil {
			provRef = strings.TrimSpace(*finalAttempt.ProviderReference)
		}

		if s.notifier != nil && isNotifierOutcome(outcome) {
			notifyErr := s.notifier.NotifyPaymentResult(ctx, PaymentResult{
				OrderID:           finalAttempt.OrderID,
				AttemptID:         finalAttempt.ID,
				Outcome:           outcome,
				AmountCents:       finalAttempt.AmountCents,
				Currency:          finalAttempt.Currency,
				ProviderReference: provRef,
				Late:              late,
			})
			if notifyErr != nil {
				// Leave processed_at NULL so a subsequent webhook retry will re-attempt notification.
				log.Printf(
					"[req_id=%s] payment webhook notifier error (order_id=%s, attempt_id=%s, outcome=%s): %v",
					reqIDFromContext(ctx),
					finalAttempt.OrderID,
					finalAttempt.ID,
					outcome,
					notifyErr,
				)
				return &WebhookResponse{Status: "ok"}, nil
			}
		}

		if markErr := s.repo.MarkEventProcessed(ctx, verified.Provider, verified.ProviderEventID, now); markErr != nil {
			log.Printf(
				"[req_id=%s] payment webhook mark processed error (provider=%s, event_id=%s): %v",
				reqIDFromContext(ctx),
				verified.Provider,
				verified.ProviderEventID,
				markErr,
			)
		}
	}

	if isDuplicateRetryNotify {
		return &WebhookResponse{Status: "duplicate"}, nil
	}
	return &WebhookResponse{Status: "ok"}, nil
}

func isNotifierOutcome(status PaymentStatus) bool {
	return status == PaymentStatusSuccess || status == PaymentStatusFailed || status == PaymentStatusCancelled
}

func generateEventID() string {
	return strings.Replace(generateAttemptID(), "att-", "evt-", 1)
}
