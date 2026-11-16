package payment

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const (
	testWebhookMerchantID     = "1211149"
	testWebhookMerchantSecret = "sandbox_test_secret_do_not_expose_789"
)

type fakeOrderNotifier struct {
	calls   []PaymentResult
	failErr error
}

func (f *fakeOrderNotifier) NotifyPaymentResult(_ context.Context, result PaymentResult) error {
	f.calls = append(f.calls, result)
	return f.failErr
}

func buildValidPayHereSig(merchantID, orderID, amount, currency, statusCode, secret string) string {
	secSum := md5.Sum([]byte(secret))
	secUpper := strings.ToUpper(hex.EncodeToString(secSum[:]))
	outerSum := md5.Sum([]byte(merchantID + orderID + amount + currency + statusCode + secUpper))
	return strings.ToUpper(hex.EncodeToString(outerSum[:]))
}

func encodeWebhookForm(fields map[string]string) string {
	vals := url.Values{}
	for k, v := range fields {
		vals.Set(k, v)
	}
	return vals.Encode()
}

func postWebhookRequest(router http.Handler, providerName string, formBody string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/webhooks/payments/"+providerName,
		bytes.NewBufferString(formBody),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestParseDecimalToCents(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input     string
		wantCents int64
		wantErr   bool
	}{
		{input: "2000.00", wantCents: 200000, wantErr: false},
		{input: "0.10", wantCents: 10, wantErr: false},
		{input: "12.5", wantCents: 1250, wantErr: false},
		{input: "45", wantCents: 4500, wantErr: false},
		{input: "0.00", wantCents: 0, wantErr: false},
		{input: "  99.99 ", wantCents: 9999, wantErr: false},
		{input: "garbage", wantErr: true},
		{input: "", wantErr: true},
		{input: "-10.00", wantErr: true},
		{input: "12.345", wantErr: true},
		{input: "12.", wantErr: true},
		{input: ".50", wantErr: true},
		{input: "12.3.4", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseDecimalToCents(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseDecimalToCents(%q) expected error, got %d", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseDecimalToCents(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.wantCents {
				t.Errorf("ParseDecimalToCents(%q) = %d, want %d", tc.input, got, tc.wantCents)
			}
		})
	}
}

func TestTransitionAttemptStatus_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		current     PaymentStatus
		incoming    PaymentStatus
		wantNext    PaymentStatus
		wantChanged bool
		wantLate    bool
	}{
		{
			name:        "PENDING to SUCCESS",
			current:     PaymentStatusPending,
			incoming:    PaymentStatusSuccess,
			wantNext:    PaymentStatusSuccess,
			wantChanged: true,
			wantLate:    false,
		},
		{
			name:        "PENDING to FAILED",
			current:     PaymentStatusPending,
			incoming:    PaymentStatusFailed,
			wantNext:    PaymentStatusFailed,
			wantChanged: true,
			wantLate:    false,
		},
		{
			name:        "PENDING to CANCELLED",
			current:     PaymentStatusPending,
			incoming:    PaymentStatusCancelled,
			wantNext:    PaymentStatusCancelled,
			wantChanged: true,
			wantLate:    false,
		},
		{
			name:        "PENDING to EXPIRED",
			current:     PaymentStatusPending,
			incoming:    PaymentStatusExpired,
			wantNext:    PaymentStatusExpired,
			wantChanged: true,
			wantLate:    false,
		},
		{
			name:        "PENDING to PENDING is ignored",
			current:     PaymentStatusPending,
			incoming:    PaymentStatusPending,
			wantNext:    PaymentStatusPending,
			wantChanged: false,
			wantLate:    false,
		},
		{
			name:        "FAILED to SUCCESS is allowed (retried payment confirmed)",
			current:     PaymentStatusFailed,
			incoming:    PaymentStatusSuccess,
			wantNext:    PaymentStatusSuccess,
			wantChanged: true,
			wantLate:    false,
		},
		{
			name:        "CANCELLED to SUCCESS is allowed",
			current:     PaymentStatusCancelled,
			incoming:    PaymentStatusSuccess,
			wantNext:    PaymentStatusSuccess,
			wantChanged: true,
			wantLate:    false,
		},
		{
			name:        "EXPIRED to SUCCESS is allowed and marked Late=true",
			current:     PaymentStatusExpired,
			incoming:    PaymentStatusSuccess,
			wantNext:    PaymentStatusSuccess,
			wantChanged: true,
			wantLate:    true,
		},
		{
			name:        "SUCCESS to PENDING is ignored (out-of-order protection)",
			current:     PaymentStatusSuccess,
			incoming:    PaymentStatusPending,
			wantNext:    PaymentStatusSuccess,
			wantChanged: false,
			wantLate:    false,
		},
		{
			name:        "SUCCESS to FAILED is ignored (never downgrade SUCCESS)",
			current:     PaymentStatusSuccess,
			incoming:    PaymentStatusFailed,
			wantNext:    PaymentStatusSuccess,
			wantChanged: false,
			wantLate:    false,
		},
		{
			name:        "SUCCESS to CANCELLED is ignored (never downgrade SUCCESS)",
			current:     PaymentStatusSuccess,
			incoming:    PaymentStatusCancelled,
			wantNext:    PaymentStatusSuccess,
			wantChanged: false,
			wantLate:    false,
		},
		{
			name:        "FAILED to PENDING is ignored",
			current:     PaymentStatusFailed,
			incoming:    PaymentStatusPending,
			wantNext:    PaymentStatusFailed,
			wantChanged: false,
			wantLate:    false,
		},
		{
			name:        "CHARGEBACK (empty incoming status) does not change attempt status",
			current:     PaymentStatusSuccess,
			incoming:    "",
			wantNext:    PaymentStatusSuccess,
			wantChanged: false,
			wantLate:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotNext, gotChanged, gotLate := TransitionAttemptStatus(tc.current, tc.incoming)
			if gotNext != tc.wantNext || gotChanged != tc.wantChanged || gotLate != tc.wantLate {
				t.Errorf(
					"TransitionAttemptStatus(%q, %q) = (%q, %v, %v), want (%q, %v, %v)",
					tc.current, tc.incoming,
					gotNext, gotChanged, gotLate,
					tc.wantNext, tc.wantChanged, tc.wantLate,
				)
			}
		})
	}
}

func TestHandleWebhook_TableDriven(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 15, 11, 30, 0, 0, time.UTC)

	cases := []struct {
		name              string
		providerParam     string
		initialAttempt    *PaymentAttempt
		orders            map[string]OrderForPayment
		rawFormBody       string
		formFields        map[string]string
		wantHTTPStatus    int
		wantDataStatus    string
		wantAttemptStatus PaymentStatus
		wantCompletedSet  bool
		wantFailureReason string
		wantNotifierCalls int
		wantNotifierOut   PaymentStatus
		wantNotifierLate  bool
	}{
		{
			name:          "success (status_code=2) transitions to SUCCESS, sets completed_at, and notifies order service once",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-001",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-001",
				"payment_id":       "ph-pay-9001",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"status_message":   "Successfully received the VISA payment",
				"method":           "VISA",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-001", "2000.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusOK,
			wantDataStatus:    "ok",
			wantAttemptStatus: PaymentStatusSuccess,
			wantCompletedSet:  true,
			wantNotifierCalls: 1,
			wantNotifierOut:   PaymentStatusSuccess,
			wantNotifierLate:  false,
		},
		{
			name:          "failed (-2) updates status to FAILED and calls notifier with FAILED outcome",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-002",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-002",
				"payment_id":       "ph-pay-9002",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "-2",
				"status_message":   "Card declined by issuer",
				"method":           "VISA",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-002", "2000.00", "USD", "-2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusOK,
			wantDataStatus:    "ok",
			wantAttemptStatus: PaymentStatusFailed,
			wantCompletedSet:  false,
			wantFailureReason: "Card declined by issuer",
			wantNotifierCalls: 1,
			wantNotifierOut:   PaymentStatusFailed,
		},
		{
			name:          "cancelled (-1) updates status to CANCELLED and calls notifier with CANCELLED outcome",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-003",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-003",
				"payment_id":       "ph-pay-9003",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "-1",
				"status_message":   "Customer cancelled checkout",
				"method":           "VISA",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-003", "2000.00", "USD", "-1", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusOK,
			wantDataStatus:    "ok",
			wantAttemptStatus: PaymentStatusCancelled,
			wantCompletedSet:  false,
			wantFailureReason: "Customer cancelled checkout",
			wantNotifierCalls: 1,
			wantNotifierOut:   PaymentStatusCancelled,
		},
		{
			name:          "invalid signature returns 401, no DB change, notifier not called",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-004",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-004",
				"payment_id":       "ph-pay-9004",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"md5sig":           "00000000000000000000000000000000",
			},
			wantHTTPStatus:    http.StatusUnauthorized,
			wantAttemptStatus: PaymentStatusPending,
			wantNotifierCalls: 0,
		},
		{
			name:          "tampered amount with a valid-looking old signature returns 401 invalid signature",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-005",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-005",
				"payment_id":       "ph-pay-9005",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				// Signature was computed for 10.00, but body claims 2000.00
				"md5sig": buildValidPayHereSig(testWebhookMerchantID, "att-005", "10.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusUnauthorized,
			wantAttemptStatus: PaymentStatusPending,
			wantNotifierCalls: 0,
		},
		{
			name:          "valid signature but amount mismatch vs stored attempt returns 422, status unchanged, SUCCESS never set",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-006",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-006",
				"payment_id":       "ph-pay-9006",
				"payhere_amount":   "1500.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-006", "1500.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusUnprocessableEntity,
			wantAttemptStatus: PaymentStatusPending,
			wantFailureReason: "amount_mismatch",
			wantNotifierCalls: 0,
		},
		{
			name:          "currency mismatch vs stored attempt returns 422",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-007",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-007",
				"payment_id":       "ph-pay-9007",
				"payhere_amount":   "2000.00",
				"payhere_currency": "LKR",
				"status_code":      "2",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-007", "2000.00", "LKR", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusUnprocessableEntity,
			wantAttemptStatus: PaymentStatusPending,
			wantFailureReason: "currency_mismatch",
			wantNotifierCalls: 0,
		},
		{
			name:          "FAILED then SUCCESS (provider confirms retry) transitions to SUCCESS",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-008",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusFailed,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-008",
				"payment_id":       "ph-pay-9008",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-008", "2000.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusOK,
			wantDataStatus:    "ok",
			wantAttemptStatus: PaymentStatusSuccess,
			wantCompletedSet:  true,
			wantNotifierCalls: 1,
			wantNotifierOut:   PaymentStatusSuccess,
			wantNotifierLate:  false,
		},
		{
			name:          "late SUCCESS on EXPIRED attempt transitions to SUCCESS with Late=true",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-009",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusExpired,
			},
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-009",
				"payment_id":       "ph-pay-9009",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-009", "2000.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusOK,
			wantDataStatus:    "ok",
			wantAttemptStatus: PaymentStatusSuccess,
			wantCompletedSet:  true,
			wantNotifierCalls: 1,
			wantNotifierOut:   PaymentStatusSuccess,
			wantNotifierLate:  true,
		},
		{
			name:          "unknown provider returns 404",
			providerParam: "unknown_gateway",
			formFields: map[string]string{
				"merchant_id": testWebhookMerchantID,
				"order_id":    "att-001",
			},
			wantHTTPStatus:    http.StatusNotFound,
			wantNotifierCalls: 0,
		},
		{
			name:          "unknown attempt returns 404",
			providerParam: "payhere_sandbox",
			formFields: map[string]string{
				"merchant_id":      testWebhookMerchantID,
				"order_id":         "att-does-not-exist",
				"payment_id":       "ph-pay-9010",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-does-not-exist", "2000.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusNotFound,
			wantNotifierCalls: 0,
		},
		{
			name:              "malformed body returns 400",
			providerParam:     "payhere_sandbox",
			rawFormBody:       "merchant_id=1211149&order_id=",
			wantHTTPStatus:    http.StatusBadRequest,
			wantNotifierCalls: 0,
		},
		{
			name:          "wrong merchant_id returns 401",
			providerParam: "payhere_sandbox",
			initialAttempt: &PaymentAttempt{
				ID:          "att-010",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
			formFields: map[string]string{
				"merchant_id":      "9999999",
				"order_id":         "att-010",
				"payment_id":       "ph-pay-9011",
				"payhere_amount":   "2000.00",
				"payhere_currency": "USD",
				"status_code":      "2",
				"md5sig":           buildValidPayHereSig("9999999", "att-010", "2000.00", "USD", "2", testWebhookMerchantSecret),
			},
			wantHTTPStatus:    http.StatusUnauthorized,
			wantAttemptStatus: PaymentStatusPending,
			wantNotifierCalls: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakePaymentRepo{
				attempts: make(map[string]*PaymentAttempt),
				events:   make(map[string]*PaymentEvent),
			}
			if tc.initialAttempt != nil {
				cp := *tc.initialAttempt
				repo.attempts[cp.ID] = &cp
			}

			orderReader := &fakeOrderReader{orders: tc.orders}
			notifier := &fakeOrderNotifier{}
			svc := NewService(repo, orderReader, validSandboxProvider())
			svc.SetNotifier(notifier)
			svc.nowFn = func() time.Time { return fixedNow }

			router := setupPaymentTestRouter(svc)

			body := tc.rawFormBody
			if body == "" && len(tc.formFields) > 0 {
				body = encodeWebhookForm(tc.formFields)
			}

			rec := postWebhookRequest(router, tc.providerParam, body)
			if rec.Code != tc.wantHTTPStatus {
				t.Fatalf("got HTTP %d, want %d (body: %s)", rec.Code, tc.wantHTTPStatus, rec.Body.String())
			}

			if tc.wantDataStatus != "" {
				var okResp struct {
					Data WebhookResponse `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &okResp); err != nil {
					t.Fatalf("unmarshal webhook response: %v", err)
				}
				if okResp.Data.Status != tc.wantDataStatus {
					t.Errorf("got data.status %q, want %q", okResp.Data.Status, tc.wantDataStatus)
				}
			}

			if tc.initialAttempt != nil {
				stored := repo.attempts[tc.initialAttempt.ID]
				if stored.Status != tc.wantAttemptStatus {
					t.Errorf("got attempt status %q, want %q", stored.Status, tc.wantAttemptStatus)
				}
				if tc.wantCompletedSet && stored.CompletedAt == nil {
					t.Error("expected completed_at to be set on SUCCESS, got nil")
				}
				if !tc.wantCompletedSet && stored.CompletedAt != nil {
					t.Errorf("expected completed_at to be nil, got %v", stored.CompletedAt)
				}
				if tc.wantFailureReason != "" {
					if stored.FailureReason == nil || *stored.FailureReason != tc.wantFailureReason {
						t.Errorf("got failure_reason %v, want %q", stored.FailureReason, tc.wantFailureReason)
					}
				}
			}

			if len(notifier.calls) != tc.wantNotifierCalls {
				t.Fatalf("got %d notifier calls, want %d", len(notifier.calls), tc.wantNotifierCalls)
			}
			if tc.wantNotifierCalls > 0 {
				call := notifier.calls[0]
				if call.Outcome != tc.wantNotifierOut {
					t.Errorf("got notifier outcome %q, want %q", call.Outcome, tc.wantNotifierOut)
				}
				if call.Late != tc.wantNotifierLate {
					t.Errorf("got notifier Late=%v, want %v", call.Late, tc.wantNotifierLate)
				}
			}
		})
	}
}

func TestHandleWebhook_DuplicateAndRetryResilience(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 15, 11, 45, 0, 0, time.UTC)

	t.Run("duplicate webhook returns 200 duplicate and notifier is NOT called twice", func(t *testing.T) {
		t.Parallel()

		repo := &fakePaymentRepo{
			attempts: map[string]*PaymentAttempt{
				"att-dup-1": {
					ID:          "att-dup-1",
					OrderID:     "order-005",
					UserID:      "buyer-001",
					Provider:    "payhere_sandbox",
					AmountCents: 200000,
					Currency:    "USD",
					Status:      PaymentStatusPending,
				},
			},
			events: make(map[string]*PaymentEvent),
		}
		notifier := &fakeOrderNotifier{}
		svc := NewService(repo, &fakeOrderReader{}, validSandboxProvider())
		svc.SetNotifier(notifier)
		svc.nowFn = func() time.Time { return fixedNow }
		router := setupPaymentTestRouter(svc)

		formBody := encodeWebhookForm(map[string]string{
			"merchant_id":      testWebhookMerchantID,
			"order_id":         "att-dup-1",
			"payment_id":       "ph-dup-100",
			"payhere_amount":   "2000.00",
			"payhere_currency": "USD",
			"status_code":      "2",
			"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-dup-1", "2000.00", "USD", "2", testWebhookMerchantSecret),
		})

		// First delivery -> 200 ok, notifier called once
		rec1 := postWebhookRequest(router, "payhere_sandbox", formBody)
		if rec1.Code != http.StatusOK {
			t.Fatalf("first delivery got HTTP %d, want 200", rec1.Code)
		}
		if len(notifier.calls) != 1 {
			t.Fatalf("after first delivery got %d notifier calls, want 1", len(notifier.calls))
		}

		// Second identical delivery -> 200 duplicate, notifier still called only once
		rec2 := postWebhookRequest(router, "payhere_sandbox", formBody)
		if rec2.Code != http.StatusOK {
			t.Fatalf("second delivery got HTTP %d, want 200", rec2.Code)
		}
		var resp2 struct {
			Data WebhookResponse `json:"data"`
		}
		if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
			t.Fatalf("unmarshal second response: %v", err)
		}
		if resp2.Data.Status != "duplicate" {
			t.Errorf("second delivery got status %q, want %q", resp2.Data.Status, "duplicate")
		}
		if len(notifier.calls) != 1 {
			t.Errorf("after duplicate delivery got %d notifier calls, want 1", len(notifier.calls))
		}
	})

	t.Run("duplicate where first notify failed (processed_at NULL) retries notifier and sets processed_at", func(t *testing.T) {
		t.Parallel()

		repo := &fakePaymentRepo{
			attempts: map[string]*PaymentAttempt{
				"att-retry-1": {
					ID:          "att-retry-1",
					OrderID:     "order-005",
					UserID:      "buyer-001",
					Provider:    "payhere_sandbox",
					AmountCents: 200000,
					Currency:    "USD",
					Status:      PaymentStatusPending,
				},
			},
			events: make(map[string]*PaymentEvent),
		}
		notifier := &fakeOrderNotifier{
			failErr: errors.New("temporary order service outage"),
		}
		svc := NewService(repo, &fakeOrderReader{}, validSandboxProvider())
		svc.SetNotifier(notifier)
		svc.nowFn = func() time.Time { return fixedNow }
		router := setupPaymentTestRouter(svc)

		formBody := encodeWebhookForm(map[string]string{
			"merchant_id":      testWebhookMerchantID,
			"order_id":         "att-retry-1",
			"payment_id":       "ph-retry-200",
			"payhere_amount":   "2000.00",
			"payhere_currency": "USD",
			"status_code":      "2",
			"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-retry-1", "2000.00", "USD", "2", testWebhookMerchantSecret),
		})

		// First delivery: DB tx succeeds, notifier fails -> processed_at stays NULL
		rec1 := postWebhookRequest(router, "payhere_sandbox", formBody)
		if rec1.Code != http.StatusOK {
			t.Fatalf("first delivery got HTTP %d, want 200", rec1.Code)
		}
		eventKey := "payhere_sandbox|ph-retry-200:2"
		if repo.events[eventKey].ProcessedAt != nil {
			t.Fatalf("expected processed_at to be nil after failed notify, got %v", repo.events[eventKey].ProcessedAt)
		}

		// Second delivery: notifier recovers -> notifier retried and processed_at set
		notifier.failErr = nil
		rec2 := postWebhookRequest(router, "payhere_sandbox", formBody)
		if rec2.Code != http.StatusOK {
			t.Fatalf("second delivery got HTTP %d, want 200", rec2.Code)
		}
		if len(notifier.calls) != 2 {
			t.Fatalf("expected notifier to be called twice (initial + retry), got %d", len(notifier.calls))
		}
		if repo.events[eventKey].ProcessedAt == nil {
			t.Fatal("expected processed_at to be set after successful retry, got nil")
		}
	})
}

func TestHandleWebhook_OutOfOrderProtection(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 15, 11, 50, 0, 0, time.UTC)

	repo := &fakePaymentRepo{
		attempts: map[string]*PaymentAttempt{
			"att-ooo-1": {
				ID:          "att-ooo-1",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
			},
		},
		events: make(map[string]*PaymentEvent),
	}
	notifier := &fakeOrderNotifier{}
	svc := NewService(repo, &fakeOrderReader{}, validSandboxProvider())
	svc.SetNotifier(notifier)
	svc.nowFn = func() time.Time { return fixedNow }
	router := setupPaymentTestRouter(svc)

	// 1. First arrive SUCCESS (status_code = 2)
	successForm := encodeWebhookForm(map[string]string{
		"merchant_id":      testWebhookMerchantID,
		"order_id":         "att-ooo-1",
		"payment_id":       "ph-ooo-300",
		"payhere_amount":   "2000.00",
		"payhere_currency": "USD",
		"status_code":      "2",
		"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-ooo-1", "2000.00", "USD", "2", testWebhookMerchantSecret),
	})
	recSuccess := postWebhookRequest(router, "payhere_sandbox", successForm)
	if recSuccess.Code != http.StatusOK {
		t.Fatalf("SUCCESS webhook got HTTP %d, want 200", recSuccess.Code)
	}

	// 2. Late PENDING (status_code = 0) arrives out of order -> must stay SUCCESS
	pendingForm := encodeWebhookForm(map[string]string{
		"merchant_id":      testWebhookMerchantID,
		"order_id":         "att-ooo-1",
		"payment_id":       "ph-ooo-300",
		"payhere_amount":   "2000.00",
		"payhere_currency": "USD",
		"status_code":      "0",
		"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-ooo-1", "2000.00", "USD", "0", testWebhookMerchantSecret),
	})
	recPending := postWebhookRequest(router, "payhere_sandbox", pendingForm)
	if recPending.Code != http.StatusOK {
		t.Fatalf("late PENDING webhook got HTTP %d, want 200", recPending.Code)
	}
	if repo.attempts["att-ooo-1"].Status != PaymentStatusSuccess {
		t.Fatalf("attempt status downgraded to %q after late PENDING, want SUCCESS", repo.attempts["att-ooo-1"].Status)
	}

	// 3. Late FAILED (status_code = -2) arrives out of order -> must stay SUCCESS
	failedForm := encodeWebhookForm(map[string]string{
		"merchant_id":      testWebhookMerchantID,
		"order_id":         "att-ooo-1",
		"payment_id":       "ph-ooo-300",
		"payhere_amount":   "2000.00",
		"payhere_currency": "USD",
		"status_code":      "-2",
		"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-ooo-1", "2000.00", "USD", "-2", testWebhookMerchantSecret),
	})
	recFailed := postWebhookRequest(router, "payhere_sandbox", failedForm)
	if recFailed.Code != http.StatusOK {
		t.Fatalf("late FAILED webhook got HTTP %d, want 200", recFailed.Code)
	}
	if repo.attempts["att-ooo-1"].Status != PaymentStatusSuccess {
		t.Fatalf("attempt status downgraded to %q after late FAILED, want SUCCESS", repo.attempts["att-ooo-1"].Status)
	}

	// Notifier should have been called only once (for the initial SUCCESS)
	if len(notifier.calls) != 1 {
		t.Errorf("got %d notifier calls, want 1", len(notifier.calls))
	}
}
