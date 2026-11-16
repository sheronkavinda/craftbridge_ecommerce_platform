package payment

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	name      string
	statuses  map[string]ProviderStatus
	errorsMap map[string]error
	calls     []string
}

func (f *fakeProvider) Name() string {
	if f.name != "" {
		return f.name
	}
	return "payhere_sandbox"
}

func (f *fakeProvider) InitiatePayment(_ context.Context, _ InitiateRequest) (InitiateResult, error) {
	return InitiateResult{}, ErrNotImplemented
}

func (f *fakeProvider) VerifyNotification(_ NotificationInput) (VerifiedEvent, error) {
	return VerifiedEvent{}, ErrNotImplemented
}

func (f *fakeProvider) QueryStatus(_ context.Context, providerRef string) (ProviderStatus, error) {
	f.calls = append(f.calls, providerRef)
	if f.errorsMap != nil {
		if err, ok := f.errorsMap[providerRef]; ok && err != nil {
			return ProviderStatus{}, err
		}
	}
	if f.statuses != nil {
		if st, ok := f.statuses[providerRef]; ok {
			return st, nil
		}
	}
	return ProviderStatus{
		ProviderReference: providerRef,
		Status:            PaymentStatusPending,
	}, nil
}

type fakeAuditRecorder struct {
	actions []string
}

func (f *fakeAuditRecorder) RecordAdminAction(_ context.Context, _ Actor, _, action, _, outcome, _ string) {
	f.actions = append(f.actions, action+":"+outcome)
}

func TestWebhook_FailureReasonClearedOnSuccessAndPreservedWhenAlreadySuccess(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

	repo := &fakePaymentRepo{
		attempts: map[string]*PaymentAttempt{
			"att-fix-1": {
				ID:          "att-fix-1",
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

	// 1. Send an amount_mismatch webhook (1500.00 instead of 2000.00) -> 422, sets failure_reason = "amount_mismatch"
	mismatchForm1 := encodeWebhookForm(map[string]string{
		"merchant_id":      testWebhookMerchantID,
		"order_id":         "att-fix-1",
		"payment_id":       "ph-fix-mismatch-1",
		"payhere_amount":   "1500.00",
		"payhere_currency": "USD",
		"status_code":      "2",
		"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-fix-1", "1500.00", "USD", "2", testWebhookMerchantSecret),
	})
	recMismatch1 := postWebhookRequest(router, "payhere_sandbox", mismatchForm1)
	if recMismatch1.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch 1 got HTTP %d, want 422", recMismatch1.Code)
	}
	if repo.attempts["att-fix-1"].FailureReason == nil || *repo.attempts["att-fix-1"].FailureReason != "amount_mismatch" {
		t.Fatalf("expected failure_reason='amount_mismatch', got %v", repo.attempts["att-fix-1"].FailureReason)
	}

	// 2. Send a valid SUCCESS webhook (2000.00) -> 200, transitions to SUCCESS and clears failure_reason (nil)
	validSuccessForm := encodeWebhookForm(map[string]string{
		"merchant_id":      testWebhookMerchantID,
		"order_id":         "att-fix-1",
		"payment_id":       "ph-fix-success-1",
		"payhere_amount":   "2000.00",
		"payhere_currency": "USD",
		"status_code":      "2",
		"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-fix-1", "2000.00", "USD", "2", testWebhookMerchantSecret),
	})
	recSuccess := postWebhookRequest(router, "payhere_sandbox", validSuccessForm)
	if recSuccess.Code != http.StatusOK {
		t.Fatalf("valid success got HTTP %d, want 200", recSuccess.Code)
	}
	if repo.attempts["att-fix-1"].Status != PaymentStatusSuccess {
		t.Fatalf("got status %q, want SUCCESS", repo.attempts["att-fix-1"].Status)
	}
	if repo.attempts["att-fix-1"].FailureReason != nil {
		t.Fatalf("expected failure_reason to be cleared (nil) on SUCCESS, got %q", *repo.attempts["att-fix-1"].FailureReason)
	}

	// 3. Send another mismatch webhook on the ALREADY-SUCCESS attempt -> 422, must NOT overwrite failure_reason
	mismatchForm2 := encodeWebhookForm(map[string]string{
		"merchant_id":      testWebhookMerchantID,
		"order_id":         "att-fix-1",
		"payment_id":       "ph-fix-mismatch-2",
		"payhere_amount":   "999.00",
		"payhere_currency": "USD",
		"status_code":      "2",
		"md5sig":           buildValidPayHereSig(testWebhookMerchantID, "att-fix-1", "999.00", "USD", "2", testWebhookMerchantSecret),
	})
	recMismatch2 := postWebhookRequest(router, "payhere_sandbox", mismatchForm2)
	if recMismatch2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatch 2 got HTTP %d, want 422", recMismatch2.Code)
	}
	if repo.attempts["att-fix-1"].Status != PaymentStatusSuccess {
		t.Fatalf("got status %q, want SUCCESS", repo.attempts["att-fix-1"].Status)
	}
	if repo.attempts["att-fix-1"].FailureReason != nil {
		t.Fatalf("expected failure_reason to remain nil on already-SUCCESS attempt, got %q", *repo.attempts["att-fix-1"].FailureReason)
	}
}

type inMemoryHTTPTestServer struct {
	handler http.Handler
}

func (s *inMemoryHTTPTestServer) Do(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func TestPayHereSandboxProvider_QueryStatus_WithHTTPTestServer(t *testing.T) {
	t.Parallel()

	const (
		appID       = "4OVy7xAppIdSandbox"
		appSecret   = "super_secret_payhere_app_secret_999"
		accessToken = "tok_live_never_log_this_token_value_12345"
	)

	t.Run("missing credentials returns ErrProviderNotConfigured", func(t *testing.T) {
		t.Parallel()
		prov := NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{
			MerchantID:     "1211149",
			MerchantSecret: "sec",
		})
		_, err := prov.QueryStatus(context.Background(), "att-001")
		if !errors.Is(err, ErrProviderNotConfigured) {
			t.Fatalf("got err %v, want ErrProviderNotConfigured", err)
		}
	})

	t.Run("token fetch and status mapping (RECEIVED, CANCELED, FAILED, PENDING/empty) without leaking secrets", func(t *testing.T) {
		t.Parallel()

		expectedBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte(appID+":"+appSecret))

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/merchant/v1/oauth/token":
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				if r.Header.Get("Authorization") != expectedBasic {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
					return
				}
				body, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(body), "grant_type=client_credentials") {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"` + accessToken + `","token_type":"bearer","expires_in":3600}`))

			case "/merchant/v1/payment/search":
				if r.Header.Get("Authorization") != "Bearer "+accessToken {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				orderID := r.URL.Query().Get("order_id")
				w.Header().Set("Content-Type", "application/json")
				switch orderID {
				case "att-received":
					_, _ = w.Write([]byte(`{"status":1,"msg":"Success","data":[{"payment_id":3200991,"status":"RECEIVED","amount":2000.00,"currency":"USD"}]}`))
				case "att-canceled":
					_, _ = w.Write([]byte(`{"status":1,"msg":"Success","data":[{"payment_id":"3200992","status":"CANCELED","amount":"2000.00","currency":"USD"}]}`))
				case "att-failed":
					_, _ = w.Write([]byte(`{"status":1,"msg":"Success","data":[{"payment_id":"3200993","status":"FAILED","amount":"2000.00","currency":"USD"}]}`))
				case "att-empty":
					_, _ = w.Write([]byte(`{"status":0,"msg":"No payments found","data":[]}`))
				case "att-500":
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal_failure","secret_echo":"` + appSecret + `"}`))
				default:
					_, _ = w.Write([]byte(`{"status":1,"data":[{"payment_id":"3200994","status":"PENDING","amount":"2000.00","currency":"USD"}]}`))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		})
		unstartedSrv := httptest.NewUnstartedServer(handler)
		defer unstartedSrv.Close()

		prov := NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{
			MerchantID:     "1211149",
			MerchantSecret: "merch_sec",
			AppID:          appID,
			AppSecret:      appSecret,
			APIBaseURL:     "https://sandbox.payhere.lk",
			HTTPClient:     &inMemoryHTTPTestServer{handler: unstartedSrv.Config.Handler},
		})

		// 1. RECEIVED -> SUCCESS
		stReceived, err := prov.QueryStatus(context.Background(), "att-received")
		if err != nil {
			t.Fatalf("att-received error: %v", err)
		}
		if stReceived.Status != PaymentStatusSuccess || stReceived.AmountCents != 200000 || stReceived.Currency != "USD" || stReceived.ProviderReference != "3200991" {
			t.Errorf("unexpected stReceived: %+v", stReceived)
		}

		// 2. CANCELED -> CANCELLED
		stCanceled, err := prov.QueryStatus(context.Background(), "att-canceled")
		if err != nil {
			t.Fatalf("att-canceled error: %v", err)
		}
		if stCanceled.Status != PaymentStatusCancelled {
			t.Errorf("got status %q, want CANCELLED", stCanceled.Status)
		}

		// 3. FAILED -> FAILED
		stFailed, err := prov.QueryStatus(context.Background(), "att-failed")
		if err != nil {
			t.Fatalf("att-failed error: %v", err)
		}
		if stFailed.Status != PaymentStatusFailed {
			t.Errorf("got status %q, want FAILED", stFailed.Status)
		}

		// 4. Empty data -> PENDING
		stEmpty, err := prov.QueryStatus(context.Background(), "att-empty")
		if err != nil {
			t.Fatalf("att-empty error: %v", err)
		}
		if stEmpty.Status != PaymentStatusPending {
			t.Errorf("got status %q, want PENDING", stEmpty.Status)
		}

		// 5. Server 500 error -> error message must NEVER contain appSecret or accessToken
		_, err500 := prov.QueryStatus(context.Background(), "att-500")
		if err500 == nil {
			t.Fatal("expected error for att-500, got nil")
		}
		if strings.Contains(err500.Error(), appSecret) || strings.Contains(err500.Error(), accessToken) {
			t.Fatalf("secret or access token leaked in error: %v", err500)
		}
	})
}

func TestReconcilePendingAttempts_Comprehensive(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

	repo := &fakePaymentRepo{
		attempts: map[string]*PaymentAttempt{
			// 1. RECEIVED -> SUCCESS
			"att-rec-success": {
				ID:          "att-rec-success",
				OrderID:     "order-101",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-45 * time.Minute),
			},
			// 2. FAILED -> FAILED
			"att-rec-failed": {
				ID:          "att-rec-failed",
				OrderID:     "order-102",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 15000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-45 * time.Minute),
			},
			// 3. CANCELED -> CANCELLED
			"att-rec-cancelled": {
				ID:          "att-rec-cancelled",
				OrderID:     "order-103",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 30000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-45 * time.Minute),
			},
			// 4. Still pending and young (45m old, TTL is 24h) -> stays PENDING
			"att-rec-young-pending": {
				ID:          "att-rec-young-pending",
				OrderID:     "order-104",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 10000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-45 * time.Minute),
			},
			// 5. Still pending and older than TTL (25h old) -> EXPIRED and notified
			"att-rec-expired": {
				ID:          "att-rec-expired",
				OrderID:     "order-105",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 10000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-25 * time.Hour),
			},
			// 6. Amount mismatch -> NOT SUCCESS
			"att-rec-mismatch": {
				ID:          "att-rec-mismatch",
				OrderID:     "order-106",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-45 * time.Minute),
			},
			// 7. Provider error for one attempt -> does not stop others
			"att-rec-provider-err": {
				ID:          "att-rec-provider-err",
				OrderID:     "order-107",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-45 * time.Minute),
			},
			// 8. Already SUCCESS via webhook -> left alone
			"att-already-success": {
				ID:          "att-already-success",
				OrderID:     "order-108",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusSuccess,
				CreatedAt:   now.Add(-60 * time.Minute),
			},
		},
		events: make(map[string]*PaymentEvent),
	}

	fakeProv := &fakeProvider{
		name: "payhere_sandbox",
		statuses: map[string]ProviderStatus{
			"att-rec-success": {
				ProviderReference: "ph-101",
				Status:            PaymentStatusSuccess,
				AmountCents:       200000,
				Currency:          "USD",
			},
			"att-rec-failed": {
				ProviderReference: "ph-102",
				Status:            PaymentStatusFailed,
				AmountCents:       15000,
				Currency:          "USD",
				FailureReason:     "payment_failed",
			},
			"att-rec-cancelled": {
				ProviderReference: "ph-103",
				Status:            PaymentStatusCancelled,
				AmountCents:       30000,
				Currency:          "USD",
				FailureReason:     "cancelled",
			},
			"att-rec-young-pending": {
				ProviderReference: "att-rec-young-pending",
				Status:            PaymentStatusPending,
			},
			"att-rec-expired": {
				ProviderReference: "att-rec-expired",
				Status:            PaymentStatusPending,
			},
			"att-rec-mismatch": {
				ProviderReference: "ph-106",
				Status:            PaymentStatusSuccess,
				AmountCents:       5000, // wrong amount!
				Currency:          "USD",
			},
		},
		errorsMap: map[string]error{
			"att-rec-provider-err": errors.New("gateway timeout"),
		},
	}

	notifier := &fakeOrderNotifier{}
	auditRec := &fakeAuditRecorder{}
	svc := NewService(repo, &fakeOrderReader{}, fakeProv)
	svc.SetNotifier(notifier)
	svc.SetAuditRecorder(auditRec)
	svc.nowFn = func() time.Time { return now }

	router := setupPaymentTestRouter(svc)

	// Trigger pass 1 via Admin endpoint POST /api/v1/admin/payments/reconcile
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/payments/reconcile", nil)
	req1.Header.Set("X-Actor-Role", "admin")
	req1.Header.Set("X-Actor-ID", "admin-001")
	req1.Header.Set("X-Actor-Name", "System Admin")
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("admin reconcile got HTTP %d, want 200 (body: %s)", rec1.Code, rec1.Body.String())
	}

	var resp1 struct {
		Data ReconcileSummary `json:"data"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("unmarshal reconcile summary: %v", err)
	}

	if resp1.Data.Checked != 7 {
		t.Errorf("got Checked=%d, want 7", resp1.Data.Checked)
	}
	if resp1.Data.Succeeded != 1 {
		t.Errorf("got Succeeded=%d, want 1", resp1.Data.Succeeded)
	}
	if resp1.Data.Failed != 1 {
		t.Errorf("got Failed=%d, want 1", resp1.Data.Failed)
	}
	if resp1.Data.Cancelled != 1 {
		t.Errorf("got Cancelled=%d, want 1", resp1.Data.Cancelled)
	}
	if resp1.Data.Expired != 1 {
		t.Errorf("got Expired=%d, want 1", resp1.Data.Expired)
	}
	// 2 errors: 1 provider timeout + 1 amount mismatch
	if resp1.Data.Errors != 2 {
		t.Errorf("got Errors=%d, want 2", resp1.Data.Errors)
	}

	// Verify attempt statuses in repo
	if repo.attempts["att-rec-success"].Status != PaymentStatusSuccess {
		t.Errorf("att-rec-success status = %q, want SUCCESS", repo.attempts["att-rec-success"].Status)
	}
	if repo.attempts["att-rec-failed"].Status != PaymentStatusFailed {
		t.Errorf("att-rec-failed status = %q, want FAILED", repo.attempts["att-rec-failed"].Status)
	}
	if repo.attempts["att-rec-cancelled"].Status != PaymentStatusCancelled {
		t.Errorf("att-rec-cancelled status = %q, want CANCELLED", repo.attempts["att-rec-cancelled"].Status)
	}
	if repo.attempts["att-rec-young-pending"].Status != PaymentStatusPending {
		t.Errorf("att-rec-young-pending status = %q, want PENDING", repo.attempts["att-rec-young-pending"].Status)
	}
	if repo.attempts["att-rec-expired"].Status != PaymentStatusExpired {
		t.Errorf("att-rec-expired status = %q, want EXPIRED", repo.attempts["att-rec-expired"].Status)
	}
	if repo.attempts["att-rec-mismatch"].Status != PaymentStatusPending {
		t.Errorf("att-rec-mismatch must NOT become SUCCESS on mismatch; got %q", repo.attempts["att-rec-mismatch"].Status)
	}
	if repo.attempts["att-already-success"].Status != PaymentStatusSuccess {
		t.Errorf("att-already-success status = %q, want SUCCESS", repo.attempts["att-already-success"].Status)
	}

	// Notifier should have been called 4 times: SUCCESS, FAILED, CANCELLED, EXPIRED
	if len(notifier.calls) != 4 {
		t.Fatalf("got %d notifier calls after pass 1, want 4", len(notifier.calls))
	}

	// Verify audit recorder was called
	if len(auditRec.actions) != 1 || auditRec.actions[0] != "PAYMENT_RECONCILE_RUN:SUCCESS" {
		t.Errorf("unexpected audit actions: %v", auditRec.actions)
	}

	// Running reconcile a second time must NOT notify again for already transitioned attempts
	summary2, err := svc.ReconcilePendingAttempts(context.Background(), now)
	if err != nil {
		t.Fatalf("second reconcile pass error: %v", err)
	}
	if summary2.Succeeded != 0 || summary2.Failed != 0 || summary2.Cancelled != 0 || summary2.Expired != 0 {
		t.Errorf("expected 0 transitions on second pass, got %+v", summary2)
	}
	if len(notifier.calls) != 4 {
		t.Errorf("expected notifier call count to remain 4 after second pass, got %d", len(notifier.calls))
	}
}

func TestBuyerPaymentHistory_Endpoints(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	internalFailReason := "internal_sql_or_gateway_detail_do_not_expose"
	ref1 := "ph-ref-101"

	repo := &fakePaymentRepo{
		attempts: map[string]*PaymentAttempt{
			"att-b1-1": {
				ID:                "att-b1-1",
				OrderID:           "order-001",
				UserID:            "buyer-001",
				Provider:          "payhere_sandbox",
				ProviderReference: &ref1,
				AmountCents:       200000,
				Currency:          "USD",
				Status:            PaymentStatusSuccess,
				CreatedAt:         t1,
				CompletedAt:       &t1,
			},
			"att-b1-2": {
				ID:            "att-b1-2",
				OrderID:       "order-002",
				UserID:        "buyer-001",
				Provider:      "payhere_sandbox",
				AmountCents:   4550,
				Currency:      "USD",
				Status:        PaymentStatusFailed,
				FailureReason: &internalFailReason,
				CreatedAt:     t2,
			},
			"att-b1-3": {
				ID:          "att-b1-3",
				OrderID:     "order-003",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 9900,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   t3,
			},
			"att-other-user": {
				ID:          "att-other-user",
				OrderID:     "order-999",
				UserID:      "buyer-002",
				Provider:    "payhere_sandbox",
				AmountCents: 50000,
				Currency:    "USD",
				Status:      PaymentStatusSuccess,
				CreatedAt:   t3,
			},
		},
	}

	orders := &fakeOrderReader{
		orders: map[string]OrderForPayment{
			"order-001": {OrderID: "order-001", OrderNumber: "ORD-2026-001", Status: "PAID"},
			"order-002": {OrderID: "order-002", OrderNumber: "ORD-2026-002", Status: "PENDING_PAYMENT"},
			"order-003": {OrderID: "order-003", OrderNumber: "ORD-2026-003", Status: "PENDING"},
		},
	}

	svc := NewService(repo, orders, validSandboxProvider())
	router := setupPaymentTestRouter(svc)

	t.Run("lists only caller's own payments newest first with pagination math and no internal failure_reason/signature fields", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/payments?page=1&page_size=2", nil)
		req.Header.Set("X-Actor-Role", "buyer")
		req.Header.Set("X-Actor-ID", "buyer-001")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got HTTP %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		bodyStr := rec.Body.String()
		for _, forbidden := range []string{internalFailReason, "failure_reason", "md5sig", "signature", "payload", "att-other-user"} {
			if strings.Contains(bodyStr, forbidden) {
				t.Fatalf("response leaked forbidden token %q: %s", forbidden, bodyStr)
			}
		}

		var resp struct {
			Data []BuyerPaymentItem `json:"data"`
			Meta PaginationMeta     `json:"meta"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}

		if resp.Meta.Page != 1 || resp.Meta.PageSize != 2 || resp.Meta.Total != 3 {
			t.Errorf("unexpected meta: %+v", resp.Meta)
		}
		if len(resp.Data) != 2 {
			t.Fatalf("got %d items, want 2", len(resp.Data))
		}
		// Newest first: att-b1-3 (t3), then att-b1-2 (t2)
		if resp.Data[0].ID != "att-b1-3" || resp.Data[1].ID != "att-b1-2" {
			t.Errorf("unexpected order of items: %s, %s", resp.Data[0].ID, resp.Data[1].ID)
		}
		if resp.Data[1].Amount != "45.50" || resp.Data[1].PublicReason != "payment_failed" {
			t.Errorf("unexpected item 2 fields: %+v", resp.Data[1])
		}
	})

	t.Run("status filter valid and invalid", func(t *testing.T) {
		t.Parallel()

		// Valid filter: ?status=SUCCESS
		reqValid := httptest.NewRequest(http.MethodGet, "/api/v1/me/payments?status=SUCCESS", nil)
		reqValid.Header.Set("X-Actor-Role", "buyer")
		reqValid.Header.Set("X-Actor-ID", "buyer-001")
		recValid := httptest.NewRecorder()
		router.ServeHTTP(recValid, reqValid)

		if recValid.Code != http.StatusOK {
			t.Fatalf("valid status filter got HTTP %d, want 200", recValid.Code)
		}
		var resp struct {
			Data []BuyerPaymentItem `json:"data"`
			Meta PaginationMeta     `json:"meta"`
		}
		_ = json.Unmarshal(recValid.Body.Bytes(), &resp)
		if len(resp.Data) != 1 || resp.Data[0].ID != "att-b1-1" || resp.Meta.Total != 1 {
			t.Errorf("unexpected filtered result: %+v (meta: %+v)", resp.Data, resp.Meta)
		}

		// Invalid filter: ?status=INVALID_STATUS -> 400
		reqInvalid := httptest.NewRequest(http.MethodGet, "/api/v1/me/payments?status=INVALID_STATUS", nil)
		reqInvalid.Header.Set("X-Actor-Role", "buyer")
		reqInvalid.Header.Set("X-Actor-ID", "buyer-001")
		recInvalid := httptest.NewRecorder()
		router.ServeHTTP(recInvalid, reqInvalid)

		if recInvalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid status filter got HTTP %d, want 400", recInvalid.Code)
		}
	})

	t.Run("detail endpoint returns own attempt with order summary and 404 for another user's attempt", func(t *testing.T) {
		t.Parallel()

		// Own attempt -> 200
		reqOwn := httptest.NewRequest(http.MethodGet, "/api/v1/me/payments/att-b1-1", nil)
		reqOwn.Header.Set("X-Actor-Role", "buyer")
		reqOwn.Header.Set("X-Actor-ID", "buyer-001")
		recOwn := httptest.NewRecorder()
		router.ServeHTTP(recOwn, reqOwn)

		if recOwn.Code != http.StatusOK {
			t.Fatalf("own attempt got HTTP %d, want 200", recOwn.Code)
		}
		var detailResp struct {
			Data BuyerPaymentItem `json:"data"`
		}
		if err := json.Unmarshal(recOwn.Body.Bytes(), &detailResp); err != nil {
			t.Fatalf("unmarshal detail: %v", err)
		}
		if detailResp.Data.OrderNumber != "ORD-2026-001" || detailResp.Data.OrderStatus != "PAID" || detailResp.Data.Amount != "2000.00" {
			t.Errorf("unexpected detail fields: %+v", detailResp.Data)
		}

		// Another user's attempt -> 404 (no existence leak)
		reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/me/payments/att-other-user", nil)
		reqOther.Header.Set("X-Actor-Role", "buyer")
		reqOther.Header.Set("X-Actor-ID", "buyer-001")
		recOther := httptest.NewRecorder()
		router.ServeHTTP(recOther, reqOther)

		if recOther.Code != http.StatusNotFound {
			t.Fatalf("other user's attempt got HTTP %d, want 404", recOther.Code)
		}
	})

	t.Run("non-buyer role returns 403 Forbidden", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/payments", nil)
		req.Header.Set("X-Actor-Role", "seller")
		req.Header.Set("X-Actor-ID", "seller-001")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("non-buyer role got HTTP %d, want 403", rec.Code)
		}
	})
}

func TestReconcile_MissingCredentialsLogsOnceWithoutSpam(t *testing.T) {
	var buf bytes.Buffer
	origOut := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(origOut)

	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	repo := &fakePaymentRepo{
		attempts: map[string]*PaymentAttempt{
			"att-unconf-1": {
				ID:        "att-unconf-1",
				OrderID:   "ord-1",
				UserID:    "buyer-1",
				Provider:  "payhere_sandbox",
				Status:    PaymentStatusPending,
				CreatedAt: now.Add(-60 * time.Minute),
			},
			"att-unconf-2": {
				ID:        "att-unconf-2",
				OrderID:   "ord-2",
				UserID:    "buyer-1",
				Provider:  "payhere_sandbox",
				Status:    PaymentStatusPending,
				CreatedAt: now.Add(-60 * time.Minute),
			},
		},
	}

	// Provider without AppID/AppSecret
	unconfiguredProv := NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{
		MerchantID:     "1211149",
		MerchantSecret: "sec",
	})
	svc := NewService(repo, &fakeOrderReader{}, unconfiguredProv)
	summary, err := svc.ReconcilePendingAttempts(context.Background(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Errors != 0 {
		t.Errorf("expected 0 errors when provider retrieval credentials are not configured, got %d", summary.Errors)
	}
	if count := strings.Count(buf.String(), "retrieval credentials not configured"); count != 1 {
		t.Errorf("expected 'retrieval credentials not configured' to be logged once per run, got %d", count)
	}
}
