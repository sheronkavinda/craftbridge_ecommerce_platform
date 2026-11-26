package payment

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeTx struct {
	committed  bool
	rolledBack bool
	commitErr  error
}

func (t *fakeTx) Begin(context.Context) (pgx.Tx, error) { return t, nil }
func (t *fakeTx) Commit(context.Context) error {
	if t.commitErr != nil {
		return t.commitErr
	}
	t.committed = true
	return nil
}
func (t *fakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	return nil
}
func (t *fakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *fakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (t *fakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (t *fakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *fakeTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (t *fakeTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (t *fakeTx) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }
func (t *fakeTx) Conn() *pgx.Conn                                         { return nil }

type fakeOrderTxBeginner struct {
	tx  *fakeTx
	err error
}

func (b *fakeOrderTxBeginner) Begin(context.Context) (pgx.Tx, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.tx, nil
}

type fakeOrderPaidMarker struct {
	calls []int64
	err   error
}

func (m *fakeOrderPaidMarker) MarkPaidTx(_ context.Context, _ pgx.Tx, orderID int64) error {
	m.calls = append(m.calls, orderID)
	return m.err
}

func TestOrderServiceNotifier(t *testing.T) {
	t.Parallel()

	errFakeInvalidTransition := errors.New("invalid order state transition")

	t.Run("SUCCESS parses numeric order ID, calls MarkPaidTx, and commits tx", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{}
		beginner := &fakeOrderTxBeginner{tx: tx}
		marker := &fakeOrderPaidMarker{}
		notifier := NewOrderServiceNotifier(beginner, marker, errFakeInvalidTransition)

		err := notifier.NotifyPaymentResult(context.Background(), PaymentResult{
			OrderID:   "42",
			AttemptID: "att-42",
			Outcome:   PaymentStatusSuccess,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(marker.calls) != 1 || marker.calls[0] != 42 {
			t.Fatalf("expected MarkPaidTx(42), got %v", marker.calls)
		}
		if !tx.committed {
			t.Fatal("expected transaction to be committed")
		}
	})

	t.Run("already paid order is a no-op success", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{}
		beginner := &fakeOrderTxBeginner{tx: tx}
		marker := &fakeOrderPaidMarker{err: nil}
		notifier := NewOrderServiceNotifier(beginner, marker, errFakeInvalidTransition)

		if err := notifier.NotifyPaymentResult(context.Background(), PaymentResult{
			OrderID:   "42",
			AttemptID: "att-42-dup",
			Outcome:   PaymentStatusSuccess,
		}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !tx.committed {
			t.Fatal("expected transaction to commit on idempotent no-op")
		}
	})

	t.Run("late or invalid transition logs warning and returns nil so provider does not retry forever", func(t *testing.T) {
		t.Parallel()

		tx := &fakeTx{}
		beginner := &fakeOrderTxBeginner{tx: tx}
		marker := &fakeOrderPaidMarker{err: errFakeInvalidTransition}
		notifier := NewOrderServiceNotifier(beginner, marker, errFakeInvalidTransition)

		err := notifier.NotifyPaymentResult(context.Background(), PaymentResult{
			OrderID:   "42",
			AttemptID: "att-late-1",
			Outcome:   PaymentStatusSuccess,
			Late:      true,
		})
		if err != nil {
			t.Fatalf("expected nil error on ErrInvalidTransition, got %v", err)
		}
		if tx.committed {
			t.Fatal("did not expect commit on ErrInvalidTransition")
		}
		if !tx.rolledBack {
			t.Fatal("expected rollback on ErrInvalidTransition")
		}
	})

	t.Run("real DB error is returned so webhook/reconciler can retry", func(t *testing.T) {
		t.Parallel()

		dbErr := errors.New("connection reset by peer")
		tx := &fakeTx{}
		beginner := &fakeOrderTxBeginner{tx: tx}
		marker := &fakeOrderPaidMarker{err: dbErr}
		notifier := NewOrderServiceNotifier(beginner, marker, errFakeInvalidTransition)

		err := notifier.NotifyPaymentResult(context.Background(), PaymentResult{
			OrderID:   "42",
			AttemptID: "att-err-1",
			Outcome:   PaymentStatusSuccess,
		})
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected %v, got %v", dbErr, err)
		}
		if tx.committed {
			t.Fatal("did not expect commit on DB error")
		}
		if !tx.rolledBack {
			t.Fatal("expected rollback on DB error")
		}
	})

	t.Run("FAILED, CANCELLED, and EXPIRED outcomes leave order untouched", func(t *testing.T) {
		t.Parallel()

		for _, outcome := range []PaymentStatus{PaymentStatusFailed, PaymentStatusCancelled, PaymentStatusExpired} {
			tx := &fakeTx{}
			beginner := &fakeOrderTxBeginner{tx: tx}
			marker := &fakeOrderPaidMarker{}
			notifier := NewOrderServiceNotifier(beginner, marker, errFakeInvalidTransition)

			if err := notifier.NotifyPaymentResult(context.Background(), PaymentResult{
				OrderID:   "42",
				AttemptID: "att-non-success",
				Outcome:   outcome,
			}); err != nil {
				t.Fatalf("outcome %s: unexpected error %v", outcome, err)
			}
			if len(marker.calls) != 0 {
				t.Fatalf("outcome %s: expected 0 MarkPaidTx calls, got %d", outcome, len(marker.calls))
			}
		}
	})
}

func TestInitiatePayment_Member04OrderValidation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	future := now.Add(20 * time.Minute)
	past := now.Add(-5 * time.Minute)

	cases := []struct {
		name       string
		order      OrderForPayment
		wantStatus int
	}{
		{
			name: "pending_payment + pending + expires_at in future succeeds",
			order: OrderForPayment{
				OrderID:       "42",
				OrderNumber:   "42",
				UserID:        "7",
				AmountCents:   9500,
				Currency:      "USD",
				Status:        "PENDING_PAYMENT",
				PaymentStatus: "PENDING",
				ExpiresAt:     &future,
			},
			wantStatus: http.StatusCreated,
		},
		{
			name: "already paid order (payment_status=paid) returns 409",
			order: OrderForPayment{
				OrderID:       "42",
				UserID:        "7",
				AmountCents:   9500,
				Currency:      "USD",
				Status:        "PENDING_PAYMENT",
				PaymentStatus: "PAID",
				ExpiresAt:     &future,
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "cancelled order returns 409",
			order: OrderForPayment{
				OrderID:       "42",
				UserID:        "7",
				AmountCents:   9500,
				Currency:      "USD",
				Status:        "CANCELLED",
				PaymentStatus: "PENDING",
				ExpiresAt:     &future,
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "expired order status returns 409",
			order: OrderForPayment{
				OrderID:       "42",
				UserID:        "7",
				AmountCents:   9500,
				Currency:      "USD",
				Status:        "EXPIRED",
				PaymentStatus: "PENDING",
				ExpiresAt:     &future,
			},
			wantStatus: http.StatusConflict,
		},
		{
			name: "pending_payment order with expires_at in the past returns 409",
			order: OrderForPayment{
				OrderID:       "42",
				UserID:        "7",
				AmountCents:   9500,
				Currency:      "USD",
				Status:        "PENDING_PAYMENT",
				PaymentStatus: "PENDING",
				ExpiresAt:     &past,
			},
			wantStatus: http.StatusConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakePaymentRepo{}
			orderReader := &fakeOrderReader{orders: map[string]OrderForPayment{tc.order.OrderID: tc.order}}
			svc := NewService(repo, orderReader, validSandboxProvider())
			svc.nowFn = func() time.Time { return now }

			res, err := svc.InitiatePayment(context.Background(), Actor{ID: "7", Role: "BUYER"}, tc.order.OrderID)
			if tc.wantStatus == http.StatusCreated {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res == nil || res.AttemptID == "" {
					t.Fatal("expected created payment attempt")
				}
			} else if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
