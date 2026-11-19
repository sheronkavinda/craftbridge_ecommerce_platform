package receipt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeReceiptRepo struct {
	mu               sync.Mutex
	byOrder          map[string]*Receipt
	completedRefunds map[string]int64
	insertRows       int
}

func newFakeReceiptRepo() *fakeReceiptRepo {
	return &fakeReceiptRepo{
		byOrder:          make(map[string]*Receipt),
		completedRefunds: make(map[string]int64),
	}
}

func (f *fakeReceiptRepo) GetOrCreateByOrder(_ context.Context, candidate Receipt) (*Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.byOrder[candidate.OrderID]; ok && existing != nil {
		cp := *existing
		return &cp, nil
	}
	cp := candidate
	cp.CreatedAt = candidate.IssuedAt
	f.byOrder[candidate.OrderID] = &cp
	f.insertRows++
	out := cp
	return &out, nil
}

func (f *fakeReceiptRepo) SumCompletedRefundsForOrder(_ context.Context, orderID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.completedRefunds[orderID], nil
}

type fakePaymentReader struct {
	paid map[string]PaidOrderPayment
	errs map[string]error
}

func (f *fakePaymentReader) GetVerifiedPaymentForOrder(_ context.Context, orderID string) (PaidOrderPayment, error) {
	if f.errs != nil {
		if err, ok := f.errs[orderID]; ok && err != nil {
			if p, exists := f.paid[orderID]; exists {
				return p, err
			}
			return PaidOrderPayment{}, err
		}
	}
	if p, ok := f.paid[orderID]; ok {
		return p, nil
	}
	return PaidOrderPayment{}, ErrOrderNotFound
}

func setupReceiptTestRouter(svc *Service) *gin.Engine {
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	h := NewHandler(svc)
	RegisterRoutes(v1, h)
	return engine
}

func TestReceipt_ViewDownloadIdempotencyAndSecurity(t *testing.T) {
	t.Parallel()

	fixedNow := time.Date(2026, 11, 17, 13, 0, 0, 0, time.UTC)
	paidAt := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	t.Run("owner gets receipt, other buyer 404, unpaid/PENDING/FAILED order 409, admin/seller 403", func(t *testing.T) {
		t.Parallel()

		repo := newFakeReceiptRepo()
		payReader := &fakePaymentReader{
			paid: map[string]PaidOrderPayment{
				"order-001": {
					AttemptID:         "att-001",
					OrderID:           "order-001",
					OrderNumber:       "ORD-2026-001",
					BuyerID:           "buyer-001",
					BuyerName:         "Nimal Perera",
					BuyerEmail:        "nimal@craftbridge.test",
					AmountCents:       1250000,
					Currency:          "USD",
					Provider:          "payhere_sandbox",
					ProviderReference: "ph-pay-001",
					CompletedAt:       paidAt,
				},
				"order-pending": {
					OrderID: "order-pending",
					BuyerID: "buyer-001",
				},
				"order-failed": {
					OrderID: "order-failed",
					BuyerID: "buyer-001",
				},
			},
			errs: map[string]error{
				"order-pending": ErrNoVerifiedPayment,
				"order-failed":  ErrNoVerifiedPayment,
			},
		}

		svc := NewService(repo, payReader, nil)
		svc.nowFn = func() time.Time { return fixedNow }
		router := setupReceiptTestRouter(svc)

		// 1. Owner gets receipt -> 200 OK
		reqOwner := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/order-001/receipt", nil)
		reqOwner.Header.Set("X-Actor-Role", "buyer")
		reqOwner.Header.Set("X-Actor-ID", "buyer-001")
		recOwner := httptest.NewRecorder()
		router.ServeHTTP(recOwner, reqOwner)

		if recOwner.Code != http.StatusOK {
			t.Fatalf("owner receipt got HTTP %d, want 200 (body: %s)", recOwner.Code, recOwner.Body.String())
		}

		var okResp struct {
			Data ReceiptView `json:"data"`
		}
		if err := json.Unmarshal(recOwner.Body.Bytes(), &okResp); err != nil {
			t.Fatalf("unmarshal receipt: %v", err)
		}
		if !strings.HasPrefix(okResp.Data.ReceiptNumber, "RCP-20261115-") {
			t.Errorf("got receipt_number %q, want prefix RCP-20261115-", okResp.Data.ReceiptNumber)
		}
		if okResp.Data.PaymentMethod != "PayHere Sandbox" || okResp.Data.Amount != "12500.00" || okResp.Data.NetPaidCents != 1250000 {
			t.Errorf("unexpected receipt fields: %+v", okResp.Data)
		}

		// 2. Other buyer -> 404 (no existence leak)
		reqOther := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/order-001/receipt", nil)
		reqOther.Header.Set("X-Actor-Role", "buyer")
		reqOther.Header.Set("X-Actor-ID", "buyer-002")
		recOther := httptest.NewRecorder()
		router.ServeHTTP(recOther, reqOther)
		if recOther.Code != http.StatusNotFound {
			t.Errorf("other buyer got HTTP %d, want 404", recOther.Code)
		}

		// 3. Unpaid / PENDING / FAILED order -> 409 "no verified payment for this order"
		for _, ordID := range []string{"order-pending", "order-failed"} {
			reqUnpaid := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/"+ordID+"/receipt", nil)
			reqUnpaid.Header.Set("X-Actor-Role", "buyer")
			reqUnpaid.Header.Set("X-Actor-ID", "buyer-001")
			recUnpaid := httptest.NewRecorder()
			router.ServeHTTP(recUnpaid, reqUnpaid)
			if recUnpaid.Code != http.StatusConflict {
				t.Errorf("%s got HTTP %d, want 409", ordID, recUnpaid.Code)
			}
			if !strings.Contains(recUnpaid.Body.String(), "no verified payment for this order") {
				t.Errorf("unexpected 409 body: %s", recUnpaid.Body.String())
			}
		}

		// 4. Admin and seller roles -> 403 Forbidden
		for _, role := range []string{"admin", "seller"} {
			reqRole := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/order-001/receipt", nil)
			reqRole.Header.Set("X-Actor-Role", role)
			reqRole.Header.Set("X-Actor-ID", role+"-001")
			recRole := httptest.NewRecorder()
			router.ServeHTTP(recRole, reqRole)
			if recRole.Code != http.StatusForbidden {
				t.Errorf("role %s got HTTP %d, want 403", role, recRole.Code)
			}
		}
	})

	t.Run("two sequential calls and concurrent calls return the exact same receipt_number and create only one row", func(t *testing.T) {
		t.Parallel()

		repo := newFakeReceiptRepo()
		payReader := &fakePaymentReader{
			paid: map[string]PaidOrderPayment{
				"order-idem": {
					AttemptID:         "att-idem",
					OrderID:           "order-idem",
					OrderNumber:       "ORD-IDEM",
					BuyerID:           "buyer-001",
					BuyerName:         "Nimal Perera",
					AmountCents:       200000,
					Currency:          "USD",
					Provider:          "payhere_sandbox",
					ProviderReference: "ph-idem",
					CompletedAt:       paidAt,
				},
			},
		}

		svc := NewService(repo, payReader, nil)
		svc.nowFn = func() time.Time { return fixedNow }
		router := setupReceiptTestRouter(svc)

		var wg sync.WaitGroup
		numbers := make([]string, 5)
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/order-idem/receipt", nil)
				req.Header.Set("X-Actor-Role", "buyer")
				req.Header.Set("X-Actor-ID", "buyer-001")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code == http.StatusOK {
					var r struct {
						Data ReceiptView `json:"data"`
					}
					_ = json.Unmarshal(rec.Body.Bytes(), &r)
					numbers[idx] = r.Data.ReceiptNumber
				}
			}(i)
		}
		wg.Wait()

		for i := 1; i < len(numbers); i++ {
			if numbers[i] == "" || numbers[i] != numbers[0] {
				t.Fatalf("receipt number mismatch across concurrent calls: %v", numbers)
			}
		}
		if repo.insertRows != 1 {
			t.Errorf("expected exactly 1 receipt row inserted, got %d", repo.insertRows)
		}
	})

	t.Run("refunded_cents and net_paid_cents reflect COMPLETED refunds only", func(t *testing.T) {
		t.Parallel()

		repo := newFakeReceiptRepo()
		// Only COMPLETED refund of 4550 cents is summed; PENDING/APPROVED are ignored by SumCompletedRefundsForOrder
		repo.completedRefunds["order-ref"] = 4550

		payReader := &fakePaymentReader{
			paid: map[string]PaidOrderPayment{
				"order-ref": {
					AttemptID:         "att-ref",
					OrderID:           "order-ref",
					OrderNumber:       "ORD-REF",
					BuyerID:           "buyer-001",
					AmountCents:       20000,
					Currency:          "USD",
					Provider:          "payhere_sandbox",
					ProviderReference: "ph-ref",
					CompletedAt:       paidAt,
				},
			},
		}

		svc := NewService(repo, payReader, nil)
		router := setupReceiptTestRouter(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/order-ref/receipt", nil)
		req.Header.Set("X-Actor-Role", "buyer")
		req.Header.Set("X-Actor-ID", "buyer-001")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var resp struct {
			Data ReceiptView `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.Data.RefundedCents != 4550 || resp.Data.NetPaidCents != 15450 || resp.Data.Status != "PARTIALLY_REFUNDED" {
			t.Errorf("unexpected refund math on receipt: %+v", resp.Data)
		}
	})

	t.Run("download endpoint sets attachment header and HTML-escapes <script>alert(1)</script> in buyer name", func(t *testing.T) {
		t.Parallel()

		repo := newFakeReceiptRepo()
		payReader := &fakePaymentReader{
			paid: map[string]PaidOrderPayment{
				"order-xss": {
					AttemptID:         "att-xss",
					OrderID:           "order-xss",
					OrderNumber:       "ORD-XSS",
					BuyerID:           "buyer-001",
					BuyerName:         "<script>alert(1)</script>",
					BuyerEmail:        "xss@craftbridge.test",
					AmountCents:       5000,
					Currency:          "USD",
					Provider:          "payhere_sandbox",
					ProviderReference: "ph-xss",
					CompletedAt:       paidAt,
				},
			},
		}

		svc := NewService(repo, payReader, nil)
		router := setupReceiptTestRouter(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders/order-xss/receipt/download", nil)
		req.Header.Set("X-Actor-Role", "buyer")
		req.Header.Set("X-Actor-ID", "buyer-001")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("download receipt got HTTP %d, want 200", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("got Content-Type %q, want text/html; charset=utf-8", ct)
		}
		expectedNum := GenerateReceiptNumber("order-xss", paidAt)
		if disp := rec.Header().Get("Content-Disposition"); disp != `attachment; filename="receipt-`+expectedNum+`.html"` {
			t.Errorf("got Content-Disposition %q", disp)
		}

		htmlBody := rec.Body.String()
		if strings.Contains(htmlBody, "<script>alert(1)</script>") {
			t.Fatalf("unescaped <script> tag found in downloaded HTML receipt: %s", htmlBody)
		}
		if !strings.Contains(htmlBody, "&lt;script&gt;alert(1)&lt;/script&gt;") {
			t.Errorf("expected escaped buyer name in HTML output, got: %s", htmlBody)
		}
	})
}
