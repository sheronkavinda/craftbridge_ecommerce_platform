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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
	if err := os.Setenv("ENABLE_LOCAL_DEV_AUTH", "1"); err != nil {
		panic(err)
	}
}

type fakePaymentRepo struct {
	latestPending   *PaymentAttempt
	latestErr       error
	createdAttempts []PaymentAttempt
	createErr       error
	attempts        map[string]*PaymentAttempt
	events          map[string]*PaymentEvent
}

func (f *fakePaymentRepo) GetLatestPendingAttemptByOrder(_ context.Context, _ string) (*PaymentAttempt, error) {
	if f.latestErr != nil {
		return nil, f.latestErr
	}
	return f.latestPending, nil
}

func (f *fakePaymentRepo) CreateAttempt(_ context.Context, attempt PaymentAttempt) (*PaymentAttempt, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	now := time.Now().UTC()
	attempt.CreatedAt = now
	attempt.UpdatedAt = now
	f.createdAttempts = append(f.createdAttempts, attempt)
	if f.attempts != nil {
		cp := attempt
		f.attempts[attempt.ID] = &cp
	}
	return &attempt, nil
}

func (f *fakePaymentRepo) GetAttemptByID(_ context.Context, attemptID string) (*PaymentAttempt, error) {
	if f.attempts != nil {
		if att, ok := f.attempts[attemptID]; ok && att != nil {
			cp := *att
			return &cp, nil
		}
	}
	return nil, ErrAttemptNotFound
}

func (f *fakePaymentRepo) UpdateAttemptStatus(
	_ context.Context,
	attemptID string,
	status PaymentStatus,
	providerRef *string,
	failureReason *string,
	completedAt *time.Time,
) (*PaymentAttempt, error) {
	if f.attempts == nil {
		return nil, ErrAttemptNotFound
	}
	att, ok := f.attempts[attemptID]
	if !ok || att == nil {
		return nil, ErrAttemptNotFound
	}
	att.Status = status
	if providerRef != nil {
		refCopy := *providerRef
		att.ProviderReference = &refCopy
	}
	if status == PaymentStatusSuccess {
		att.FailureReason = nil
	} else if failureReason != nil {
		reasonCopy := *failureReason
		att.FailureReason = &reasonCopy
	}
	if completedAt != nil {
		compCopy := *completedAt
		att.CompletedAt = &compCopy
	}
	att.UpdatedAt = time.Now().UTC()
	cp := *att
	return &cp, nil
}

func (f *fakePaymentRepo) ClaimPendingAttemptsForReconcile(_ context.Context, olderThan time.Time, limit int) ([]PaymentAttempt, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	var out []PaymentAttempt
	for _, att := range f.attempts {
		if att == nil || att.Status != PaymentStatusPending {
			continue
		}
		if !att.CreatedAt.After(olderThan) {
			out = append(out, *att)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakePaymentRepo) ListAttemptsByUser(_ context.Context, filter ListBuyerPaymentsFilter) ([]PaymentAttempt, int64, error) {
	var matched []PaymentAttempt
	for _, att := range f.attempts {
		if att == nil || !strings.EqualFold(att.UserID, filter.UserID) {
			continue
		}
		if filter.Status != "" && att.Status != filter.Status {
			continue
		}
		matched = append(matched, *att)
	}

	// Sort newest first by CreatedAt DESC, ID DESC
	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			if matched[j].CreatedAt.After(matched[i].CreatedAt) ||
				(matched[j].CreatedAt.Equal(matched[i].CreatedAt) && matched[j].ID > matched[i].ID) {
				matched[i], matched[j] = matched[j], matched[i]
			}
		}
	}

	total := int64(len(matched))
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	if offset >= len(matched) {
		return []PaymentAttempt{}, total, nil
	}
	end := offset + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], total, nil
}

func (f *fakePaymentRepo) InsertEventIfNew(_ context.Context, event PaymentEvent) (bool, *time.Time, error) {
	if f.events == nil {
		f.events = make(map[string]*PaymentEvent)
	}
	key := event.Provider + "|" + event.ProviderEventID
	if existing, ok := f.events[key]; ok {
		return false, existing.ProcessedAt, nil
	}
	cp := event
	f.events[key] = &cp
	return true, nil, nil
}

func (f *fakePaymentRepo) MarkEventProcessed(_ context.Context, provider, providerEventID string, processedAt time.Time) error {
	if f.events == nil {
		return nil
	}
	key := provider + "|" + providerEventID
	if existing, ok := f.events[key]; ok {
		ts := processedAt
		existing.ProcessedAt = &ts
	}
	return nil
}

type fakeOrderReader struct {
	orders map[string]OrderForPayment
	err    error
}

func (f *fakeOrderReader) GetOrderForPayment(_ context.Context, orderID string) (OrderForPayment, error) {
	if f.err != nil {
		return OrderForPayment{}, f.err
	}
	if order, ok := f.orders[orderID]; ok {
		return order, nil
	}
	return OrderForPayment{}, ErrOrderNotFound
}

func validSandboxProvider() *PayHereSandboxProvider {
	return NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{
		MerchantID:     "1211149",
		MerchantSecret: "sandbox_test_secret_do_not_expose_789",
		CheckoutURL:    "https://sandbox.payhere.lk/pay/checkout",
		ReturnURL:      "http://localhost:4200/checkout/return",
		CancelURL:      "http://localhost:4200/checkout/cancel",
		NotifyURL:      "http://localhost:8080/api/v1/payments/webhook/payhere",
	})
}

func setupPaymentTestRouter(svc *Service) *gin.Engine {
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	h := NewHandler(svc)
	RegisterRoutes(v1, h)
	return engine
}

func TestPayHereSandboxProvider_HashAndSecretProtection(t *testing.T) {
	t.Parallel()

	const (
		merchantID     = "1211149"
		merchantSecret = "my_fixed_sandbox_secret_phrase"
		attemptID      = "att-fixed-001"
		amountStr      = "250.00"
		currency       = "USD"
	)

	provider := NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{
		MerchantID:     merchantID,
		MerchantSecret: merchantSecret,
		ReturnURL:      "http://localhost:4200/payments/return",
		CancelURL:      "http://localhost:4200/payments/cancel",
		NotifyURL:      "http://localhost:8080/api/v1/payments/notify",
	})

	res, err := provider.InitiatePayment(context.Background(), InitiateRequest{
		AttemptID:   attemptID,
		OrderID:     "order-101",
		OrderNumber: "ORD-101",
		AmountCents: 25000,
		Currency:    currency,
		FirstName:   "Nimal",
		LastName:    "Perera",
		Email:       "nimal@craftbridge.test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Independently compute expected PayHere MD5 signature:
	// MD5(merchant_id + order_id + amount + currency + UPPER(MD5(merchant_secret)))
	secretSum := md5.Sum([]byte(merchantSecret))
	upperSecretHash := strings.ToUpper(hex.EncodeToString(secretSum[:]))
	outerSum := md5.Sum([]byte(merchantID + attemptID + amountStr + currency + upperSecretHash))
	expectedHash := strings.ToUpper(hex.EncodeToString(outerSum[:]))

	if res.Fields["hash"] != expectedHash {
		t.Errorf("got hash %q, want %q", res.Fields["hash"], expectedHash)
	}
	if res.Fields["amount"] != amountStr {
		t.Errorf("got amount %q, want %q", res.Fields["amount"], amountStr)
	}
	if res.Fields["order_id"] != attemptID {
		t.Errorf("got order_id %q, want %q", res.Fields["order_id"], attemptID)
	}
	if res.Method != "POST" || res.CheckoutURL != defaultPayHereCheckoutURL {
		t.Errorf("unexpected checkout endpoint: method=%q url=%q", res.Method, res.CheckoutURL)
	}

	// Ensure merchant secret never appears anywhere in the serialized output or fields
	serialized, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if strings.Contains(string(serialized), merchantSecret) || strings.Contains(string(serialized), upperSecretHash) {
		t.Fatalf("merchant secret leaked in InitiateResult JSON: %s", string(serialized))
	}

	// VerifyNotification on empty input returns ErrMalformedNotification; QueryStatus without AppID/AppSecret returns ErrProviderNotConfigured
	if _, err := provider.VerifyNotification(NotificationInput{}); !errors.Is(err, ErrMalformedNotification) {
		t.Errorf("VerifyNotification got err %v, want ErrMalformedNotification", err)
	}
	if _, err := provider.QueryStatus(context.Background(), attemptID); !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("QueryStatus got err %v, want ErrProviderNotConfigured", err)
	}
}

func TestInitiatePayment_TableDriven(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name              string
		actorRole         string
		actorID           string
		orderID           string
		requestBody       string
		orders            map[string]OrderForPayment
		existingPending   *PaymentAttempt
		provider          PaymentProvider
		wantStatus        int
		wantErrorContains string
		wantAttemptID     string
		wantAmountField   string
		wantCurrencyField string
		wantCreatedCount  int
	}{
		{
			name:      "success creates new PENDING attempt and returns PayHere checkout form",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-005",
			orders: map[string]OrderForPayment{
				"order-005": {
					OrderID:     "order-005",
					OrderNumber: "ORD-005",
					UserID:      "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					Status:      "PENDING",
					BuyerName:   "Nimal Perera",
					BuyerEmail:  "nimal@craftbridge.test",
				},
			},
			provider:          validSandboxProvider(),
			wantStatus:        http.StatusCreated,
			wantAmountField:   "2000.00",
			wantCurrencyField: "USD",
			wantCreatedCount:  1,
		},
		{
			name:              "order not found returns 404",
			actorRole:         "buyer",
			actorID:           "buyer-001",
			orderID:           "order-missing",
			orders:            map[string]OrderForPayment{},
			provider:          validSandboxProvider(),
			wantStatus:        http.StatusNotFound,
			wantErrorContains: "order not found",
			wantCreatedCount:  0,
		},
		{
			name:      "order belonging to another user returns 404 without leaking existence",
			actorRole: "buyer",
			actorID:   "buyer-002",
			orderID:   "order-005",
			orders: map[string]OrderForPayment{
				"order-005": {
					OrderID:     "order-005",
					UserID:      "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					Status:      "PENDING",
				},
			},
			provider:          validSandboxProvider(),
			wantStatus:        http.StatusNotFound,
			wantErrorContains: "order not found",
			wantCreatedCount:  0,
		},
		{
			name:      "order not in pending status returns 409 Conflict",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-001",
			orders: map[string]OrderForPayment{
				"order-001": {
					OrderID:     "order-001",
					UserID:      "buyer-001",
					AmountCents: 1250000,
					Currency:    "USD",
					Status:      "PAID",
				},
			},
			provider:          validSandboxProvider(),
			wantStatus:        http.StatusConflict,
			wantErrorContains: "order is not pending payment",
			wantCreatedCount:  0,
		},
		{
			name:        "amount and currency in request body are ignored in favor of stored order",
			actorRole:   "buyer",
			actorID:     "buyer-001",
			orderID:     "order-005",
			requestBody: `{"amount":0.01,"amount_cents":1,"currency":"EUR"}`,
			orders: map[string]OrderForPayment{
				"order-005": {
					OrderID:     "order-005",
					OrderNumber: "ORD-005",
					UserID:      "buyer-001",
					AmountCents: 4550,
					Currency:    "USD",
					Status:      "PENDING_PAYMENT",
				},
			},
			provider:          validSandboxProvider(),
			wantStatus:        http.StatusCreated,
			wantAmountField:   "45.50",
			wantCurrencyField: "USD",
			wantCreatedCount:  1,
		},
		{
			name:      "existing recent PENDING attempt (<15 min) is reused without creating duplicate row",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-005",
			orders: map[string]OrderForPayment{
				"order-005": {
					OrderID:     "order-005",
					UserID:      "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					Status:      "PENDING",
				},
			},
			existingPending: &PaymentAttempt{
				ID:          "att-existing-recent",
				OrderID:     "order-005",
				UserID:      "buyer-001",
				Provider:    "payhere_sandbox",
				AmountCents: 200000,
				Currency:    "USD",
				Status:      PaymentStatusPending,
				CreatedAt:   now.Add(-5 * time.Minute),
			},
			provider:          validSandboxProvider(),
			wantStatus:        http.StatusCreated,
			wantAttemptID:     "att-existing-recent",
			wantAmountField:   "2000.00",
			wantCurrencyField: "USD",
			wantCreatedCount:  0,
		},
		{
			name:      "provider not configured returns 503 Service Unavailable",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-005",
			orders: map[string]OrderForPayment{
				"order-005": {
					OrderID:     "order-005",
					UserID:      "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					Status:      "PENDING",
				},
			},
			provider:          NewPayHereSandboxProviderFromConfig(PayHereSandboxConfig{}),
			wantStatus:        http.StatusServiceUnavailable,
			wantErrorContains: "payment provider not configured",
			wantCreatedCount:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakePaymentRepo{latestPending: tc.existingPending}
			orderReader := &fakeOrderReader{orders: tc.orders}
			svc := NewService(repo, orderReader, tc.provider)
			svc.nowFn = func() time.Time { return now }

			router := setupPaymentTestRouter(svc)

			var bodyReader *bytes.Reader
			if tc.requestBody != "" {
				bodyReader = bytes.NewReader([]byte(tc.requestBody))
			} else {
				bodyReader = bytes.NewReader(nil)
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+tc.orderID+"/payments", bodyReader)
			if tc.requestBody != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.actorRole != "" {
				req.Header.Set("X-Actor-Role", tc.actorRole)
			}
			if tc.actorID != "" {
				req.Header.Set("X-Actor-ID", tc.actorID)
				req.Header.Set("X-Actor-Name", "Nimal Perera")
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("got HTTP status %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if len(repo.createdAttempts) != tc.wantCreatedCount {
				t.Errorf("got %d created attempts, want %d", len(repo.createdAttempts), tc.wantCreatedCount)
			}

			if tc.wantErrorContains != "" {
				var errResp map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("unmarshal error response: %v", err)
				}
				if !strings.Contains(errResp["error"], tc.wantErrorContains) {
					t.Errorf("got error %q, want substring %q", errResp["error"], tc.wantErrorContains)
				}
				return
			}

			var okResp struct {
				Data InitiatePaymentResponse `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &okResp); err != nil {
				t.Fatalf("unmarshal success response: %v", err)
			}

			if tc.wantAttemptID != "" && okResp.Data.AttemptID != tc.wantAttemptID {
				t.Errorf("got attempt_id %q, want %q", okResp.Data.AttemptID, tc.wantAttemptID)
			}
			if okResp.Data.AttemptID == "" {
				t.Error("expected non-empty attempt_id")
			}
			if okResp.Data.Status != PaymentStatusPending {
				t.Errorf("got status %q, want %q", okResp.Data.Status, PaymentStatusPending)
			}
			if okResp.Data.Checkout.Fields["amount"] != tc.wantAmountField {
				t.Errorf("got checkout amount %q, want %q", okResp.Data.Checkout.Fields["amount"], tc.wantAmountField)
			}
			if okResp.Data.Checkout.Fields["currency"] != tc.wantCurrencyField {
				t.Errorf("got checkout currency %q, want %q", okResp.Data.Checkout.Fields["currency"], tc.wantCurrencyField)
			}
		})
	}
}
