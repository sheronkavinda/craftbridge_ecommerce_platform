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

const (
	defaultReconcileAfterMinutes = 30
	defaultAttemptTTLHours       = 24
	reconcileBatchLimit          = 50
)

// AuditRecorder is an optional interface for recording administrative audit log entries
// without coupling the payment package to the audit package.
type AuditRecorder interface {
	RecordAdminAction(ctx context.Context, actor Actor, requestID, action, target, outcome, details string)
}

// ReconcilePendingAttempts scans PENDING payment attempts older than the configured reconciliation threshold
// (using SELECT ... FOR UPDATE SKIP LOCKED with a batch limit of 50), queries the payment provider for
// their authoritative status, and feeds each result through the shared state-machine, amount/currency
// verification, idempotent event persistence, and OrderNotifier pipeline.
func (s *Service) ReconcilePendingAttempts(ctx context.Context, now time.Time) (*ReconcileSummary, error) {
	if now.IsZero() {
		now = s.nowFn()
	}

	reconcileAfter := s.reconcileAfter
	if reconcileAfter <= 0 {
		reconcileAfter = defaultReconcileAfterMinutes * time.Minute
	}
	attemptTTL := s.attemptTTL
	if attemptTTL <= 0 {
		attemptTTL = defaultAttemptTTLHours * time.Hour
	}

	cutoff := now.Add(-reconcileAfter)
	var claimed []PaymentAttempt

	err := s.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		var claimErr error
		claimed, claimErr = s.repo.ClaimPendingAttemptsForReconcile(txCtx, cutoff, reconcileBatchLimit)
		return claimErr
	})
	if err != nil {
		return nil, err
	}

	summary := &ReconcileSummary{}
	notConfiguredLogged := false

	for i := range claimed {
		candidate := claimed[i]

		// Re-load current attempt state in case a concurrent webhook already transitioned it.
		currentAttempt, getErr := s.repo.GetAttemptByID(ctx, candidate.ID)
		if getErr != nil || currentAttempt == nil {
			continue
		}
		if currentAttempt.Status != PaymentStatusPending {
			// Already transitioned (e.g. SUCCESS via webhook); leave untouched.
			continue
		}

		provider := s.resolveProvider(currentAttempt.Provider)
		if provider == nil {
			if !notConfiguredLogged {
				log.Printf("[req_id=%s] payment reconcile skipped: no payment provider registered", reqIDFromContext(ctx))
				notConfiguredLogged = true
			}
			continue
		}

		queryRef := currentAttempt.ID
		if currentAttempt.ProviderReference != nil && strings.TrimSpace(*currentAttempt.ProviderReference) != "" {
			queryRef = strings.TrimSpace(*currentAttempt.ProviderReference)
		}

		provStatus, queryErr := provider.QueryStatus(ctx, queryRef)
		if queryErr != nil {
			if errors.Is(queryErr, ErrProviderNotConfigured) {
				if !notConfiguredLogged {
					log.Printf(
						"[req_id=%s] payment reconcile skipped: provider %s retrieval credentials not configured",
						reqIDFromContext(ctx),
						provider.Name(),
					)
					notConfiguredLogged = true
				}
				continue
			}

			summary.Checked++
			summary.Errors++
			log.Printf(
				"[req_id=%s] payment reconcile query error (attempt_id=%s, provider=%s): %v",
				reqIDFromContext(ctx),
				currentAttempt.ID,
				provider.Name(),
				queryErr,
			)
			continue
		}

		summary.Checked++

		// Case 1: Provider reports still PENDING (or not found).
		if provStatus.Status == "" || provStatus.Status == PaymentStatusPending {
			if now.Sub(currentAttempt.CreatedAt) < attemptTTL {
				// Still within TTL; leave PENDING for future webhook or reconciliation pass.
				continue
			}

			payloadBytes, _ := json.Marshal(map[string]string{
				"source":     "reconcile",
				"attempt_id": currentAttempt.ID,
				"status":     string(PaymentStatusExpired),
			})

			syntheticExpired := VerifiedEvent{
				Provider:          currentAttempt.Provider,
				ProviderEventID:   fmt.Sprintf("reconcile:%s:%s", currentAttempt.ID, PaymentStatusExpired),
				AttemptID:         currentAttempt.ID,
				ProviderReference: firstNonEmpty(provStatus.ProviderReference, derefStringPtr(currentAttempt.ProviderReference)),
				EventType:         "RECONCILE_EXPIRED",
				Status:            PaymentStatusExpired,
				AmountCents:       currentAttempt.AmountCents,
				Currency:          currentAttempt.Currency,
				SignatureValid:    true,
				FailureReason:     "expired",
				RawPayload:        payloadBytes,
			}

			_, transitioned, applyErr := s.applyVerifiedEvent(ctx, currentAttempt, syntheticExpired, now)
			if applyErr != nil {
				summary.Errors++
				continue
			}
			if transitioned {
				summary.Expired++
			}
			continue
		}

		// Case 2: Provider reports a terminal outcome (SUCCESS, FAILED, CANCELLED).
		eventAmountCents := provStatus.AmountCents
		eventCurrency := strings.ToUpper(strings.TrimSpace(provStatus.Currency))

		if provStatus.Status != PaymentStatusSuccess {
			if eventAmountCents == 0 {
				eventAmountCents = currentAttempt.AmountCents
			}
			if eventCurrency == "" {
				eventCurrency = currentAttempt.Currency
			}
		}

		payloadBytes, _ := json.Marshal(map[string]string{
			"source":     "reconcile",
			"attempt_id": currentAttempt.ID,
			"status":     string(provStatus.Status),
		})

		syntheticEvent := VerifiedEvent{
			Provider:          currentAttempt.Provider,
			ProviderEventID:   fmt.Sprintf("reconcile:%s:%s", currentAttempt.ID, provStatus.Status),
			AttemptID:         currentAttempt.ID,
			ProviderReference: firstNonEmpty(provStatus.ProviderReference, derefStringPtr(currentAttempt.ProviderReference)),
			EventType:         "RECONCILE_" + string(provStatus.Status),
			Status:            provStatus.Status,
			AmountCents:       eventAmountCents,
			Currency:          eventCurrency,
			SignatureValid:    true,
			FailureReason:     firstNonEmpty(provStatus.FailureReason, PublicFailureReason(provStatus.Status)),
			RawPayload:        payloadBytes,
		}

		_, transitioned, applyErr := s.applyVerifiedEvent(ctx, currentAttempt, syntheticEvent, now)
		if applyErr != nil {
			summary.Errors++
			continue
		}
		if transitioned {
			switch provStatus.Status {
			case PaymentStatusSuccess:
				summary.Succeeded++
			case PaymentStatusFailed:
				summary.Failed++
			case PaymentStatusCancelled:
				summary.Cancelled++
			case PaymentStatusExpired:
				summary.Expired++
			}
		}
	}

	return summary, nil
}

// StartReconcileWorker starts a background ticker that periodically invokes ReconcilePendingAttempts.
// If interval <= 0, the background worker is disabled and this method returns immediately.
// The worker stops cleanly when ctx is cancelled and recovers from any unexpected panics so it never crashes the server.
func (s *Service) StartReconcileWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		log.Println("payment reconciliation background worker is disabled (interval <= 0)")
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case tickTime := <-ticker.C:
				s.runSafeReconcilePass(ctx, tickTime.UTC())
			}
		}
	}()
}

func (s *Service) runSafeReconcilePass(ctx context.Context, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered from panic in payment reconciliation worker: %v", r)
		}
	}()

	summary, err := s.ReconcilePendingAttempts(ctx, now)
	if err != nil {
		log.Printf("payment reconciliation worker pass failed: %v", err)
		return
	}
	if summary != nil && summary.Checked > 0 {
		log.Printf(
			"payment reconciliation pass complete: checked=%d succeeded=%d failed=%d cancelled=%d expired=%d errors=%d",
			summary.Checked,
			summary.Succeeded,
			summary.Failed,
			summary.Cancelled,
			summary.Expired,
			summary.Errors,
		)
	}
}

func (s *Service) resolveProvider(providerName string) PaymentProvider {
	key := strings.ToLower(strings.TrimSpace(providerName))
	if key != "" && s.providers != nil {
		if p, ok := s.providers[key]; ok && p != nil {
			return p
		}
	}
	return s.provider
}

func derefStringPtr(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return strings.TrimSpace(*ptr)
}
