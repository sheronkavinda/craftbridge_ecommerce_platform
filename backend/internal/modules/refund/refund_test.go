package refund

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
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

type fakeRefundRepo struct {
	mu          sync.Mutex
	refunds     map[string]*Refund
	idemKeys    map[string]string
	providerRef map[string]string
	created     []*Refund
}

func newFakeRefundRepo() *fakeRefundRepo {
	return &fakeRefundRepo{
		refunds:     make(map[string]*Refund),
		idemKeys:    make(map[string]string),
		providerRef: make(map[string]string),
	}
}

func (f *fakeRefundRepo) seed(ref Refund) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := ref
	f.refunds[cp.ID] = &cp
	if cp.IdempotencyKey != nil && *cp.IdempotencyKey != "" {
		f.idemKeys[*cp.IdempotencyKey] = cp.ID
	}
	if cp.ProviderRefundReference != nil && *cp.ProviderRefundReference != "" {
		f.providerRef[*cp.ProviderRefundReference] = cp.ID
	}
}

func (f *fakeRefundRepo) GetByID(_ context.Context, refundID string) (*Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.refunds[refundID]; ok && r != nil {
		cp := *r
		return &cp, nil
	}
	return nil, ErrRefundNotFound
}

func (f *fakeRefundRepo) GetByIdempotencyKey(_ context.Context, idempotencyKey string) (*Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.idemKeys[idempotencyKey]; ok {
		if r, exists := f.refunds[id]; exists && r != nil {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeRefundRepo) FindIdenticalOpenRefund(_ context.Context, orderID, buyerID string, amountCents int64) (*Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.refunds {
		if r == nil {
			continue
		}
		if r.OrderID == orderID &&
			strings.EqualFold(r.RequestedBy, buyerID) &&
			r.AmountCents == amountCents &&
			(r.Status == RefundStatusPending || r.Status == RefundStatusApproved) {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeRefundRepo) SumActiveRefundsForOrder(_ context.Context, orderID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var sum int64
	for _, r := range f.refunds {
		if r != nil && r.OrderID == orderID && r.Status != RefundStatusRejected {
			sum += r.AmountCents
		}
	}
	return sum, nil
}

func (f *fakeRefundRepo) Create(_ context.Context, ref Refund) (*Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ref.IdempotencyKey != nil && *ref.IdempotencyKey != "" {
		if _, exists := f.idemKeys[*ref.IdempotencyKey]; exists {
			return nil, ErrDuplicateIdempotencyKey
		}
		f.idemKeys[*ref.IdempotencyKey] = ref.ID
	}
	cp := ref
	f.refunds[cp.ID] = &cp
	f.created = append(f.created, &cp)
	out := cp
	return &out, nil
}

func (f *fakeRefundRepo) TransitionPendingToApproved(_ context.Context, refundID, adminID string, decidedAt time.Time) (*Refund, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.refunds[refundID]
	if !ok || r == nil {
		return nil, false, nil
	}
	if r.Status != RefundStatusPending {
		return nil, false, nil
	}
	r.Status = RefundStatusApproved
	adminCopy := adminID
	tsCopy := decidedAt
	r.DecidedBy = &adminCopy
	r.DecidedAt = &tsCopy
	r.ReviewedAt = &tsCopy
	r.UpdatedAt = tsCopy
	cp := *r
	return &cp, true, nil
}

func (f *fakeRefundRepo) UpdateAfterProviderRefund(
	_ context.Context,
	refundID string,
	status RefundStatus,
	providerRef string,
	completedAt *time.Time,
) (*Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.refunds[refundID]
	if !ok || r == nil || r.Status != RefundStatusApproved {
		return nil, ErrRefundTransitionConflict
	}
	trimmedRef := strings.TrimSpace(providerRef)
	if trimmedRef != "" {
		if ownerID, exists := f.providerRef[trimmedRef]; exists && ownerID != refundID {
			return nil, ErrDuplicateProviderRefundRef
		}
		f.providerRef[trimmedRef] = refundID
		r.ProviderRefundReference = &trimmedRef
	}
	r.Status = status
	if completedAt != nil {
		tsCopy := *completedAt
		r.CompletedAt = &tsCopy
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRefundRepo) ListByBuyer(_ context.Context, filter ListBuyerRefundsFilter) ([]Refund, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var matched []Refund
	for _, r := range f.refunds {
		if r == nil || !strings.EqualFold(r.RequestedBy, filter.UserID) {
			continue
		}
		if filter.Status != "" && r.Status != filter.Status {
			continue
		}
		matched = append(matched, *r)
	}

	for i := 0; i < len(matched); i++ {
		for j := i + 1; j < len(matched); j++ {
			if matched[j].RequestedAt.After(matched[i].RequestedAt) ||
				(matched[j].RequestedAt.Equal(matched[i].RequestedAt) && matched[j].ID > matched[i].ID) {
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
		return []Refund{}, total, nil
	}
	end := offset + pageSize
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], total, nil
}

type fakePaymentReader struct {
	paid map[string]PaidPayment
	errs map[string]error
}

func (f *fakePaymentReader) GetPaidAttemptForOrder(_ context.Context, orderID string) (PaidPayment, error) {
	if f.errs != nil {
		if err, ok := f.errs[orderID]; ok && err != nil {
			if p, exists := f.paid[orderID]; exists {
				return p, err
			}
			return PaidPayment{}, err
		}
	}
	if p, ok := f.paid[orderID]; ok {
		return p, nil
	}
	return PaidPayment{}, ErrOrderNotFound
}

type fakeRefundGateway struct {
	mu      sync.Mutex
	calls   []GatewayRefundInput
	byKey   map[string]GatewayRefundOutput
	outcome GatewayRefundOutcome
	ref     string
	failErr error
}

func (f *fakeRefundGateway) RefundPayment(_ context.Context, input GatewayRefundInput) (GatewayRefundOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, input)

	if f.failErr != nil {
		return GatewayRefundOutput{}, f.failErr
	}
	if f.byKey == nil {
		f.byKey = make(map[string]GatewayRefundOutput)
	}
	if existing, ok := f.byKey[input.IdempotencyKey]; ok {
		return existing, nil
	}

	status := f.outcome
	if status == "" {
		status = GatewayOutcomeCompleted
	}
	ref := f.ref
	if ref == "" {
		ref = "ph-refund-" + input.IdempotencyKey
	}
	out := GatewayRefundOutput{
		ProviderRefundReference: ref,
		Status:                  status,
		Raw:                     `{"status":"ok"}`,
	}
	f.byKey[input.IdempotencyKey] = out
	return out, nil
}

type fakeRefundNotifier struct {
	mu    sync.Mutex
	calls []RefundNotification
}

func (f *fakeRefundNotifier) NotifyRefundCompleted(_ context.Context, n RefundNotification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, n)
	return nil
}

type fakeRefundAudit struct {
	mu      sync.Mutex
	entries []string
}

func (f *fakeRefundAudit) RecordAdminAction(_ context.Context, _ Actor, _, action, target, outcome, _ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, action+":"+target+":"+outcome)
}

func setupRefundTestRouter(svc *Service) *gin.Engine {
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	h := NewHandler(svc)
	RegisterRoutes(v1, h)
	return engine
}

func TestParseDecimalToCents_EdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input     string
		wantCents int64
		wantErr   bool
	}{
		{input: "2000.00", wantCents: 200000, wantErr: false},
		{input: "0.01", wantCents: 1, wantErr: false},
		{input: "12.5", wantCents: 1250, wantErr: false},
		{input: "99", wantCents: 9900, wantErr: false},
		{input: "0.00", wantCents: 0, wantErr: false},
		{input: "", wantErr: true},
		{input: "-5.00", wantErr: true},
		{input: "12.345", wantErr: true},
		{input: "12.", wantErr: true},
		{input: ".99", wantErr: true},
		{input: "abc", wantErr: true},
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

func TestRequestRefund_TableDriven(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name              string
		actorRole         string
		actorID           string
		orderID           string
		bodyJSON          string
		idemKey           string
		paidOrders        map[string]PaidPayment
		orderErrs         map[string]error
		wantStatus        int
		wantErrorContains string
		wantAmountCents   int64
		wantDetails       string
	}{
		{
			name:      "success creates PENDING refund with raw unescaped details and synced amount/amount_cents",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-101",
			bodyJSON:  `{"reason":"ARRIVED_DAMAGED","details":"Box & vase arrived cracked <rim>!","amount":"45.50"}`,
			paidOrders: map[string]PaidPayment{
				"order-101": {
					AttemptID:         "att-101",
					OrderID:           "order-101",
					OrderNumber:       "ORD-101",
					BuyerID:           "buyer-001",
					SellerID:          "seller-001",
					ProviderReference: "ph-101",
					AmountCents:       200000,
					Currency:          "USD",
					CompletedAt:       now.Add(-2 * 24 * time.Hour),
				},
			},
			wantStatus:      http.StatusCreated,
			wantAmountCents: 4550,
			wantDetails:     "Box & vase arrived cracked <rim>!",
		},
		{
			name:      "order without SUCCESS payment returns 409 order has no verified payment",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-unpaid",
			bodyJSON:  `{"reason":"CHANGED_MIND","details":"Changed my mind about this order.","amount":"10.00"}`,
			paidOrders: map[string]PaidPayment{
				"order-unpaid": {
					OrderID: "order-unpaid",
					BuyerID: "buyer-001",
				},
			},
			orderErrs: map[string]error{
				"order-unpaid": ErrNoVerifiedPayment,
			},
			wantStatus:        http.StatusConflict,
			wantErrorContains: "order has no verified payment",
		},
		{
			name:      "another user's order returns 404 without leaking existence",
			actorRole: "buyer",
			actorID:   "buyer-002",
			orderID:   "order-101",
			bodyJSON:  `{"reason":"ARRIVED_DAMAGED","details":"Trying to refund someone else's order.","amount":"10.00"}`,
			paidOrders: map[string]PaidPayment{
				"order-101": {
					AttemptID:   "att-101",
					OrderID:     "order-101",
					BuyerID:     "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					CompletedAt: now.Add(-1 * time.Hour),
				},
			},
			wantStatus:        http.StatusNotFound,
			wantErrorContains: "order not found",
		},
		{
			name:      "refund window expired (> 30 days) returns 409 refund window expired",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-old",
			bodyJSON:  `{"reason":"ARRIVED_DAMAGED","details":"Requesting refund after 35 days.","amount":"20.00"}`,
			paidOrders: map[string]PaidPayment{
				"order-old": {
					AttemptID:   "att-old",
					OrderID:     "order-old",
					BuyerID:     "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					CompletedAt: now.Add(-35 * 24 * time.Hour),
				},
			},
			wantStatus:        http.StatusConflict,
			wantErrorContains: "refund window expired",
		},
		{
			name:      "amount exceeding refundable balance returns 422",
			actorRole: "buyer",
			actorID:   "buyer-001",
			orderID:   "order-101",
			bodyJSON:  `{"reason":"WRONG_ITEM","details":"Received wrong item entirely.","amount":"2500.00"}`,
			paidOrders: map[string]PaidPayment{
				"order-101": {
					AttemptID:   "att-101",
					OrderID:     "order-101",
					BuyerID:     "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					CompletedAt: now.Add(-1 * time.Hour),
				},
			},
			wantStatus:        http.StatusUnprocessableEntity,
			wantErrorContains: "amount exceeds refundable balance",
		},
		{
			name:              "invalid reason returns 400",
			actorRole:         "buyer",
			actorID:           "buyer-001",
			orderID:           "order-101",
			bodyJSON:          `{"reason":"NOT_A_VALID_REASON","details":"Valid length details text.","amount":"10.00"}`,
			wantStatus:        http.StatusBadRequest,
			wantErrorContains: "invalid refund reason",
		},
		{
			name:              "details too short (< 10 chars) returns 400",
			actorRole:         "buyer",
			actorID:           "buyer-001",
			orderID:           "order-101",
			bodyJSON:          `{"reason":"OTHER","details":"too short","amount":"10.00"}`,
			wantStatus:        http.StatusBadRequest,
			wantErrorContains: "details must be between 10 and 500 characters",
		},
		{
			name:              "invalid amount returns 400",
			actorRole:         "buyer",
			actorID:           "buyer-001",
			orderID:           "order-101",
			bodyJSON:          `{"reason":"OTHER","details":"Valid length details text.","amount":"0.00"}`,
			wantStatus:        http.StatusBadRequest,
			wantErrorContains: "amount must be a positive decimal value",
		},
		{
			name:       "non-buyer role returns 403",
			actorRole:  "seller",
			actorID:    "seller-001",
			orderID:    "order-101",
			bodyJSON:   `{"reason":"OTHER","details":"Valid length details text.","amount":"10.00"}`,
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRefundRepo()
			payReader := &fakePaymentReader{paid: tc.paidOrders, errs: tc.orderErrs}
			svc := NewService(repo, payReader, &fakeRefundGateway{})
			svc.nowFn = func() time.Time { return now }
			router := setupRefundTestRouter(svc)

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/orders/"+tc.orderID+"/refund-requests",
				bytes.NewBufferString(tc.bodyJSON),
			)
			req.Header.Set("Content-Type", "application/json")
			if tc.actorRole != "" {
				req.Header.Set("X-Actor-Role", tc.actorRole)
			}
			if tc.actorID != "" {
				req.Header.Set("X-Actor-ID", tc.actorID)
			}
			if tc.idemKey != "" {
				req.Header.Set("Idempotency-Key", tc.idemKey)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("got HTTP %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if tc.wantErrorContains != "" {
				if !strings.Contains(rec.Body.String(), tc.wantErrorContains) {
					t.Errorf("got body %s, want substring %q", rec.Body.String(), tc.wantErrorContains)
				}
				return
			}
			if tc.wantStatus != http.StatusCreated {
				return
			}

			var resp struct {
				Data Refund `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if resp.Data.AmountCents != tc.wantAmountCents {
				t.Errorf("got amount_cents %d, want %d", resp.Data.AmountCents, tc.wantAmountCents)
			}
			if resp.Data.Details != tc.wantDetails {
				t.Errorf("got details %q, want %q", resp.Data.Details, tc.wantDetails)
			}
			if resp.Data.Status != RefundStatusPending {
				t.Errorf("got status %q, want PENDING", resp.Data.Status)
			}
		})
	}
}

func TestRequestRefund_IdempotencyAndPartialBalance(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 16, 10, 30, 0, 0, time.UTC)

	t.Run("same Idempotency-Key twice creates one row and returns 200 second time; identical open request without key creates no duplicate", func(t *testing.T) {
		t.Parallel()

		repo := newFakeRefundRepo()
		payReader := &fakePaymentReader{
			paid: map[string]PaidPayment{
				"order-200": {
					AttemptID:   "att-200",
					OrderID:     "order-200",
					BuyerID:     "buyer-001",
					AmountCents: 200000,
					Currency:    "USD",
					CompletedAt: now.Add(-1 * time.Hour),
				},
			},
		}
		svc := NewService(repo, payReader, &fakeRefundGateway{})
		svc.nowFn = func() time.Time { return now }
		router := setupRefundTestRouter(svc)

		body1 := `{"reason":"ARRIVED_DAMAGED","details":"Handle broken during delivery.","amount":"500.00"}`
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/orders/order-200/refund-requests", bytes.NewBufferString(body1))
		req1.Header.Set("Content-Type", "application/json")
		req1.Header.Set("X-Actor-Role", "buyer")
		req1.Header.Set("X-Actor-ID", "buyer-001")
		req1.Header.Set("Idempotency-Key", "idem-key-alpha")
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusCreated {
			t.Fatalf("first request got HTTP %d, want 201", rec1.Code)
		}

		// Second request with same Idempotency-Key -> 200 OK, still 1 created row
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/orders/order-200/refund-requests", bytes.NewBufferString(body1))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Actor-Role", "buyer")
		req2.Header.Set("X-Actor-ID", "buyer-001")
		req2.Header.Set("Idempotency-Key", "idem-key-alpha")
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusOK {
			t.Fatalf("second request with same Idempotency-Key got HTTP %d, want 200", rec2.Code)
		}
		if len(repo.created) != 1 {
			t.Fatalf("expected 1 created refund row, got %d", len(repo.created))
		}

		// Identical open request without Idempotency-Key (same order, buyer, amount 500.00) -> 200 OK, no duplicate
		req3 := httptest.NewRequest(http.MethodPost, "/api/v1/orders/order-200/refund-requests", bytes.NewBufferString(body1))
		req3.Header.Set("Content-Type", "application/json")
		req3.Header.Set("X-Actor-Role", "buyer")
		req3.Header.Set("X-Actor-ID", "buyer-001")
		rec3 := httptest.NewRecorder()
		router.ServeHTTP(rec3, req3)

		if rec3.Code != http.StatusOK {
			t.Fatalf("identical open request without key got HTTP %d, want 200", rec3.Code)
		}
		if len(repo.created) != 1 {
			t.Fatalf("expected 1 created refund row after identical open request, got %d", len(repo.created))
		}

		// Partial refund balance check: 500.00 already requested out of 2000.00; requesting 1600.00 -> 422, requesting 1500.00 -> 201
		bodyTooMuch := `{"reason":"NOT_AS_DESCRIBED","details":"Second partial refund exceeds remaining balance.","amount":"1600.00"}`
		req4 := httptest.NewRequest(http.MethodPost, "/api/v1/orders/order-200/refund-requests", bytes.NewBufferString(bodyTooMuch))
		req4.Header.Set("Content-Type", "application/json")
		req4.Header.Set("X-Actor-Role", "buyer")
		req4.Header.Set("X-Actor-ID", "buyer-001")
		rec4 := httptest.NewRecorder()
		router.ServeHTTP(rec4, req4)

		if rec4.Code != http.StatusUnprocessableEntity {
			t.Fatalf("exceeding remaining balance got HTTP %d, want 422", rec4.Code)
		}

		bodyFits := `{"reason":"NOT_AS_DESCRIBED","details":"Second partial refund within remaining balance.","amount":"1500.00"}`
		req5 := httptest.NewRequest(http.MethodPost, "/api/v1/orders/order-200/refund-requests", bytes.NewBufferString(bodyFits))
		req5.Header.Set("Content-Type", "application/json")
		req5.Header.Set("X-Actor-Role", "buyer")
		req5.Header.Set("X-Actor-ID", "buyer-001")
		rec5 := httptest.NewRecorder()
		router.ServeHTTP(rec5, req5)

		if rec5.Code != http.StatusCreated {
			t.Fatalf("second partial refund within remaining balance got HTTP %d, want 201", rec5.Code)
		}
		if len(repo.created) != 2 {
			t.Fatalf("expected 2 created refund rows, got %d", len(repo.created))
		}
	})
}

func TestApproveRefund_AllScenarios(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 16, 11, 0, 0, 0, time.UTC)

	t.Run("PENDING -> APPROVED -> COMPLETED with provider reference, audit recorded, and double approve is idempotent", func(t *testing.T) {
		t.Parallel()

		repo := newFakeRefundRepo()
		attID := "att-301"
		repo.seed(Refund{
			ID:               "ref-301",
			OrderID:          "order-301",
			PaymentAttemptID: &attID,
			RequestedBy:      "buyer-001",
			Reason:           ReasonArrivedDamaged,
			Details:          "Broken ceramic plate.",
			Amount:           45.50,
			AmountCents:      4550,
			Currency:         "USD",
			Status:           RefundStatusPending,
			RequestedAt:      now.Add(-1 * time.Hour),
		})

		payReader := &fakePaymentReader{
			paid: map[string]PaidPayment{
				"order-301": {
					AttemptID:         "att-301",
					OrderID:           "order-301",
					BuyerID:           "buyer-001",
					ProviderReference: "ph-pay-301",
					AmountCents:       4550,
					Currency:          "USD",
				},
			},
		}
		gw := &fakeRefundGateway{outcome: GatewayOutcomeCompleted, ref: "ph-ref-9001"}
		notifier := &fakeRefundNotifier{}
		auditRec := &fakeRefundAudit{}

		svc := NewService(repo, payReader, gw)
		svc.SetNotifier(notifier)
		svc.SetAuditRecorder(auditRec)
		svc.nowFn = func() time.Time { return now }
		router := setupRefundTestRouter(svc)

		// First approve -> 200 COMPLETED
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-301/approve", nil)
		req1.Header.Set("X-Actor-Role", "admin")
		req1.Header.Set("X-Actor-ID", "admin-001")
		req1.Header.Set("X-Actor-Name", "System Admin")
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusOK {
			t.Fatalf("first approve got HTTP %d, want 200 (body: %s)", rec1.Code, rec1.Body.String())
		}

		stored, _ := repo.GetByID(context.Background(), "ref-301")
		if stored.Status != RefundStatusCompleted {
			t.Fatalf("got status %q, want COMPLETED", stored.Status)
		}
		if stored.ProviderRefundReference == nil || *stored.ProviderRefundReference != "ph-ref-9001" {
			t.Errorf("got provider_refund_reference %v, want ph-ref-9001", stored.ProviderRefundReference)
		}
		if stored.DecidedAt == nil || stored.ReviewedAt == nil || stored.CompletedAt == nil {
			t.Errorf("expected decided_at, reviewed_at, and completed_at to be set: %+v", stored)
		}
		if len(gw.calls) != 1 || gw.calls[0].IdempotencyKey != "ref-301" {
			t.Errorf("unexpected gateway calls: %+v", gw.calls)
		}
		if len(notifier.calls) != 1 {
			t.Errorf("got %d notifier calls, want 1", len(notifier.calls))
		}
		if len(auditRec.entries) != 1 || auditRec.entries[0] != "REFUND_APPROVED:refund:ref-301:SUCCESS" {
			t.Errorf("unexpected audit entries: %v", auditRec.entries)
		}

		// Second approve on COMPLETED -> 200 OK, no second provider call
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-301/approve", nil)
		req2.Header.Set("X-Actor-Role", "admin")
		req2.Header.Set("X-Actor-ID", "admin-001")
		rec2 := httptest.NewRecorder()
		router.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusOK {
			t.Fatalf("second approve on COMPLETED got HTTP %d, want 200", rec2.Code)
		}
		if len(gw.calls) != 1 {
			t.Errorf("expected gateway call count to remain 1, got %d", len(gw.calls))
		}
	})

	t.Run("provider PENDING sets status PROCESSED; provider failure leaves APPROVED and retry uses SAME idempotency key to reach COMPLETED", func(t *testing.T) {
		t.Parallel()

		repo := newFakeRefundRepo()
		repo.seed(Refund{
			ID:          "ref-pending-gw",
			OrderID:     "order-302",
			RequestedBy: "buyer-001",
			Reason:      ReasonWrongItem,
			Details:     "Wrong item shipped.",
			Amount:      20.00,
			AmountCents: 2000,
			Currency:    "USD",
			Status:      RefundStatusPending,
			RequestedAt: now,
		})
		repo.seed(Refund{
			ID:          "ref-retry-gw",
			OrderID:     "order-303",
			RequestedBy: "buyer-001",
			Reason:      ReasonWrongItem,
			Details:     "Retry provider failure test.",
			Amount:      30.00,
			AmountCents: 3000,
			Currency:    "USD",
			Status:      RefundStatusPending,
			RequestedAt: now,
		})

		gw := &fakeRefundGateway{outcome: GatewayOutcomePending, ref: "ph-ref-pending-1"}
		svc := NewService(repo, &fakePaymentReader{}, gw)
		svc.nowFn = func() time.Time { return now }
		router := setupRefundTestRouter(svc)

		// 1. Provider PENDING -> PROCESSED
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-pending-gw/approve", nil)
		req1.Header.Set("X-Actor-Role", "admin")
		req1.Header.Set("X-Actor-ID", "admin-001")
		rec1 := httptest.NewRecorder()
		router.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusOK {
			t.Fatalf("provider PENDING got HTTP %d, want 200", rec1.Code)
		}
		st1, _ := repo.GetByID(context.Background(), "ref-pending-gw")
		if st1.Status != RefundStatusProcessed {
			t.Errorf("got status %q, want PROCESSED", st1.Status)
		}

		// 2. Provider failure on ref-retry-gw -> 502, leaves refund in APPROVED status
		gw.failErr = errors.New("provider network timeout")
		reqFail := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-retry-gw/approve", nil)
		reqFail.Header.Set("X-Actor-Role", "admin")
		reqFail.Header.Set("X-Actor-ID", "admin-001")
		recFail := httptest.NewRecorder()
		router.ServeHTTP(recFail, reqFail)

		if recFail.Code != http.StatusBadGateway {
			t.Fatalf("provider failure got HTTP %d, want 502", recFail.Code)
		}
		stFail, _ := repo.GetByID(context.Background(), "ref-retry-gw")
		if stFail.Status != RefundStatusApproved {
			t.Fatalf("expected status to remain APPROVED after provider failure, got %q", stFail.Status)
		}

		// 3. Second approve retries with the SAME provider idempotency key ("ref-retry-gw") and completes
		gw.failErr = nil
		gw.outcome = GatewayOutcomeCompleted
		gw.ref = "ph-ref-retry-success"

		reqRetry := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-retry-gw/approve", nil)
		reqRetry.Header.Set("X-Actor-Role", "admin")
		reqRetry.Header.Set("X-Actor-ID", "admin-001")
		recRetry := httptest.NewRecorder()
		router.ServeHTTP(recRetry, reqRetry)

		if recRetry.Code != http.StatusOK {
			t.Fatalf("retry approve got HTTP %d, want 200 (body: %s)", recRetry.Code, recRetry.Body.String())
		}
		stRetry, _ := repo.GetByID(context.Background(), "ref-retry-gw")
		if stRetry.Status != RefundStatusCompleted {
			t.Fatalf("expected status COMPLETED after retry, got %q", stRetry.Status)
		}
		// Both calls for ref-retry-gw must have used IdempotencyKey == "ref-retry-gw"
		if gw.calls[1].IdempotencyKey != "ref-retry-gw" || gw.calls[2].IdempotencyKey != "ref-retry-gw" {
			t.Errorf("expected same idempotency key 'ref-retry-gw' on retry, got %q and %q", gw.calls[1].IdempotencyKey, gw.calls[2].IdempotencyKey)
		}
		if len(gw.byKey) != 2 { // 1 for ref-pending-gw, 1 for ref-retry-gw
			t.Errorf("expected 2 unique provider refunds recorded, got %d", len(gw.byKey))
		}
	})

	t.Run("REJECTED -> 409, unknown id -> 404, non-admin -> 403, concurrent double approve -> only one wins", func(t *testing.T) {
		t.Parallel()

		repo := newFakeRefundRepo()
		repo.seed(Refund{
			ID:          "ref-rejected",
			OrderID:     "order-304",
			RequestedBy: "buyer-001",
			Status:      RefundStatusRejected,
			AmountCents: 1000,
			Currency:    "USD",
		})
		repo.seed(Refund{
			ID:          "ref-concurrent",
			OrderID:     "order-305",
			RequestedBy: "buyer-001",
			Status:      RefundStatusPending,
			AmountCents: 1000,
			Currency:    "USD",
		})

		gw := &fakeRefundGateway{outcome: GatewayOutcomeCompleted}
		svc := NewService(repo, &fakePaymentReader{}, gw)
		svc.nowFn = func() time.Time { return now }
		router := setupRefundTestRouter(svc)

		// REJECTED -> 409
		reqRej := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-rejected/approve", nil)
		reqRej.Header.Set("X-Actor-Role", "admin")
		reqRej.Header.Set("X-Actor-ID", "admin-001")
		recRej := httptest.NewRecorder()
		router.ServeHTTP(recRej, reqRej)
		if recRej.Code != http.StatusConflict {
			t.Errorf("REJECTED refund approve got HTTP %d, want 409", recRej.Code)
		}

		// Unknown ID -> 404
		reqMissing := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-missing/approve", nil)
		reqMissing.Header.Set("X-Actor-Role", "admin")
		reqMissing.Header.Set("X-Actor-ID", "admin-001")
		recMissing := httptest.NewRecorder()
		router.ServeHTTP(recMissing, reqMissing)
		if recMissing.Code != http.StatusNotFound {
			t.Errorf("unknown refund approve got HTTP %d, want 404", recMissing.Code)
		}

		// Non-admin -> 403
		reqBuyer := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-concurrent/approve", nil)
		reqBuyer.Header.Set("X-Actor-Role", "buyer")
		reqBuyer.Header.Set("X-Actor-ID", "buyer-001")
		recBuyer := httptest.NewRecorder()
		router.ServeHTTP(recBuyer, reqBuyer)
		if recBuyer.Code != http.StatusForbidden {
			t.Errorf("non-admin approve got HTTP %d, want 403", recBuyer.Code)
		}

		// Concurrent double approve: simulate two simultaneous calls
		var wg sync.WaitGroup
		wg.Add(2)
		codes := make([]int, 2)
		for idx := 0; idx < 2; idx++ {
			go func(i int) {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/refunds/ref-concurrent/approve", nil)
				req.Header.Set("X-Actor-Role", "admin")
				req.Header.Set("X-Actor-ID", "admin-001")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				codes[i] = rec.Code
			}(idx)
		}
		wg.Wait()

		if len(gw.calls) != 1 {
			t.Errorf("expected provider refund to be called exactly once under concurrent approve, got %d", len(gw.calls))
		}
	})
}

func TestBuyerRefundViews_ListDetailAndTimelines(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	idemSecret := "buyer-001:secret-idem-key"
	provSecret := "ph-secret-refund-ref-999"

	repo := newFakeRefundRepo()
	repo.seed(Refund{
		ID:                      "ref-b1-completed",
		OrderID:                 "order-001",
		RequestedBy:             "buyer-001",
		Reason:                  ReasonArrivedDamaged,
		Details:                 "Completed refund details.",
		Amount:                  50.00,
		AmountCents:             5000,
		Currency:                "USD",
		Status:                  RefundStatusCompleted,
		ProviderRefundReference: &provSecret,
		IdempotencyKey:          &idemSecret,
		RequestedAt:             t1,
		DecidedAt:               &t2,
		CompletedAt:             &t3,
	})
	repo.seed(Refund{
		ID:          "ref-b1-pending",
		OrderID:     "order-002",
		RequestedBy: "buyer-001",
		Reason:      ReasonChangedMind,
		Details:     "Pending refund details.",
		Amount:      25.00,
		AmountCents: 2500,
		Currency:    "USD",
		Status:      RefundStatusPending,
		RequestedAt: t3,
	})
	repo.seed(Refund{
		ID:          "ref-b2-other",
		OrderID:     "order-999",
		RequestedBy: "buyer-002",
		Reason:      ReasonOther,
		Details:     "Other buyer's refund.",
		Amount:      10.00,
		AmountCents: 1000,
		Currency:    "USD",
		Status:      RefundStatusPending,
		RequestedAt: t3,
	})

	payReader := &fakePaymentReader{
		paid: map[string]PaidPayment{
			"order-001": {OrderID: "order-001", OrderNumber: "ORD-001", BuyerID: "buyer-001"},
			"order-002": {OrderID: "order-002", OrderNumber: "ORD-002", BuyerID: "buyer-001"},
		},
	}

	svc := NewService(repo, payReader, &fakeRefundGateway{})
	router := setupRefundTestRouter(svc)

	// 1. List caller's own refunds with pagination and verify no provider/idempotency fields in JSON
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/me/refunds?page=1&page_size=10", nil)
	reqList.Header.Set("X-Actor-Role", "buyer")
	reqList.Header.Set("X-Actor-ID", "buyer-001")
	recList := httptest.NewRecorder()
	router.ServeHTTP(recList, reqList)

	if recList.Code != http.StatusOK {
		t.Fatalf("list buyer refunds got HTTP %d, want 200", recList.Code)
	}
	rawBody := recList.Body.String()
	for _, forbidden := range []string{idemSecret, provSecret, "idempotency_key", "provider_refund_reference", "ref-b2-other"} {
		if strings.Contains(rawBody, forbidden) {
			t.Fatalf("buyer refund JSON leaked forbidden field/value %q: %s", forbidden, rawBody)
		}
	}

	var listResp struct {
		Data []BuyerRefundView `json:"data"`
		Meta PaginationMeta    `json:"meta"`
	}
	if err := json.Unmarshal(recList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Meta.Total != 2 || len(listResp.Data) != 2 {
		t.Fatalf("got %d items (total %d), want 2", len(listResp.Data), listResp.Meta.Total)
	}

	// Verify timeline steps for PENDING (item 0, newest) and COMPLETED (item 1)
	pendingTimeline := listResp.Data[0].Timeline
	if len(pendingTimeline) != 4 || !pendingTimeline[0].Done || !pendingTimeline[1].Done || pendingTimeline[2].Done || pendingTimeline[3].Done {
		t.Errorf("unexpected PENDING timeline: %+v", pendingTimeline)
	}
	completedTimeline := listResp.Data[1].Timeline
	if len(completedTimeline) != 4 || !completedTimeline[0].Done || !completedTimeline[1].Done || !completedTimeline[2].Done || !completedTimeline[3].Done {
		t.Errorf("unexpected COMPLETED timeline: %+v", completedTimeline)
	}

	// Verify timeline for APPROVED, PROCESSED, and REJECTED
	approvedSteps := BuildRefundTimeline(Refund{Status: RefundStatusApproved, RequestedAt: t1, DecidedAt: &t2})
	if !approvedSteps[2].Done || approvedSteps[3].Done {
		t.Errorf("unexpected APPROVED timeline: %+v", approvedSteps)
	}
	processedSteps := BuildRefundTimeline(Refund{Status: RefundStatusProcessed, RequestedAt: t1, DecidedAt: &t2})
	if !processedSteps[2].Done || processedSteps[3].Done {
		t.Errorf("unexpected PROCESSED timeline: %+v", processedSteps)
	}
	rejectedSteps := BuildRefundTimeline(Refund{Status: RefundStatusRejected, RequestedAt: t1, DecidedAt: &t2})
	if rejectedSteps[2].Done || rejectedSteps[3].Done {
		t.Errorf("unexpected REJECTED timeline: %+v", rejectedSteps)
	}

	// 2. Detail endpoint for other user's refund -> 404
	reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/me/refunds/ref-b2-other", nil)
	reqOther.Header.Set("X-Actor-Role", "buyer")
	reqOther.Header.Set("X-Actor-ID", "buyer-001")
	recOther := httptest.NewRecorder()
	router.ServeHTTP(recOther, reqOther)
	if recOther.Code != http.StatusNotFound {
		t.Errorf("other user's refund detail got HTTP %d, want 404", recOther.Code)
	}
}
