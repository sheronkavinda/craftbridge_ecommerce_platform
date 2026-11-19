package payment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSellerSalesReader struct {
	sellerMap map[string]string
	orders    map[string][]SellerPaidOrderRow
	payouts   map[string][]SellerPayoutRecord
	paidSums  map[string]int64
}

func (f *fakeSellerSalesReader) ResolveSellerID(_ context.Context, actorID string) (string, error) {
	if f.sellerMap != nil {
		if mapped, ok := f.sellerMap[actorID]; ok && mapped != "" {
			return mapped, nil
		}
	}
	return actorID, nil
}

func (f *fakeSellerSalesReader) ListSellerPaidOrders(_ context.Context, sellerID string) ([]SellerPaidOrderRow, error) {
	if f.orders != nil {
		if rows, ok := f.orders[sellerID]; ok {
			return rows, nil
		}
	}
	return []SellerPaidOrderRow{}, nil
}

func (f *fakeSellerSalesReader) ListSellerPayouts(_ context.Context, sellerID string) ([]SellerPayoutRecord, int64, error) {
	if f.payouts != nil {
		if p, ok := f.payouts[sellerID]; ok {
			return p, f.paidSums[sellerID], nil
		}
	}
	return []SellerPayoutRecord{}, 0, nil
}

func TestComputeCommissionCents_Rounding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		merchandise int64
		percent     int
		want        int64
	}{
		{merchandise: 333, percent: 10, want: 33},
		{merchandise: 335, percent: 10, want: 34},
		{merchandise: 334, percent: 10, want: 33},
		{merchandise: 10000, percent: 10, want: 1000},
		{merchandise: 1999, percent: 10, want: 200},
		{merchandise: 0, percent: 10, want: 0},
		{merchandise: 5000, percent: 0, want: 0},
	}

	for _, tc := range cases {
		got := ComputeCommissionCents(tc.merchandise, tc.percent)
		if got != tc.want {
			t.Errorf("ComputeCommissionCents(%d, %d) = %d, want %d", tc.merchandise, tc.percent, got, tc.want)
		}
	}
}

func TestSellerEarnings_CalculationIsolationRangeAndRBAC(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)

	// Hold boundary is 7 days (holdCutoff = now - 7d):
	// - Order A (paid 2 days ago): within hold (< 7d) -> Pending
	//   merchandise = 10000, shipping = 500, commission(10%) = 1000, completed_refund = 0 -> net = 9500 (COMPLETED)
	// - Order B (paid 10 days ago): older than hold (>= 7d) -> Available
	//   merchandise = 20000, shipping = 0, commission(10%) = 2000, completed_refund = 5000 -> net = 13000 (COMPLETED)
	// - Order C (paid 45 days ago): older than 30d range, but included in all-time summary Available!
	//   merchandise = 5000, shipping = 0, commission(10%) = 500, completed_refund = 5000 (full refund) -> net = -500 (REFUNDED clawback)
	salesReader := &fakeSellerSalesReader{
		sellerMap: map[string]string{
			"seller-u1": "seller-001",
			"seller-u2": "seller-002",
		},
		orders: map[string][]SellerPaidOrderRow{
			"seller-001": {
				{
					OrderID:          "order-A",
					OrderNumber:      "ORD-A",
					ProductTitle:     "Handcarved Teak Mask",
					PaidAt:           now.Add(-2 * 24 * time.Hour),
					MerchandiseCents: 10000,
					ShippingCents:    500,
					CompletedRefunds: 0,
					Currency:         "USD",
					PaymentAttemptID: "att-A",
				},
				{
					OrderID:          "order-B",
					OrderNumber:      "ORD-B",
					ProductTitle:     "Brass Oil Lamp",
					PaidAt:           now.Add(-10 * 24 * time.Hour),
					MerchandiseCents: 20000,
					ShippingCents:    0,
					CompletedRefunds: 5000,
					Currency:         "USD",
					PaymentAttemptID: "att-B",
				},
				{
					OrderID:          "order-C",
					OrderNumber:      "ORD-C",
					ProductTitle:     "Batik Wall Hanging",
					PaidAt:           now.Add(-45 * 24 * time.Hour),
					MerchandiseCents: 5000,
					ShippingCents:    0,
					CompletedRefunds: 5000,
					Currency:         "USD",
					PaymentAttemptID: "att-C",
				},
			},
			"seller-002": {
				{
					OrderID:          "order-other-seller",
					OrderNumber:      "ORD-OTHER",
					ProductTitle:     "Handloom Sarong",
					PaidAt:           now.Add(-1 * 24 * time.Hour),
					MerchandiseCents: 99000,
					ShippingCents:    0,
					CompletedRefunds: 0,
					Currency:         "USD",
					PaymentAttemptID: "att-other",
				},
			},
		},
	}

	svc := NewService(&fakePaymentRepo{}, &fakeOrderReader{}, validSandboxProvider())
	svc.SetSellerSalesReader(salesReader)
	svc.SetEarningsConfig(10, 7)
	svc.nowFn = func() time.Time { return now }
	router := setupPaymentTestRouter(svc)

	t.Run("default 30d range computes all-time summary (pending vs available, paid=0) and filters transactions to 30d with seller isolation", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/seller/earnings", nil)
		req.Header.Set("X-Actor-Role", "seller")
		req.Header.Set("X-Actor-ID", "seller-u1")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got HTTP %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "order-other-seller") {
			t.Fatalf("seller-u1 response leaked seller-002's order: %s", rec.Body.String())
		}

		var resp struct {
			Data SellerEarningsPayload `json:"data"`
			Meta PaginationMeta        `json:"meta"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal earnings response: %v", err)
		}

		// Summary is all-time:
		// Pending = Order A (9500)
		// Available = Order B (13000) + Order C (-500) = 12500
		// Paid = 0 (no payouts table)
		if resp.Data.Summary.PendingCents != 9500 {
			t.Errorf("got pending_cents %d, want 9500", resp.Data.Summary.PendingCents)
		}
		if resp.Data.Summary.AvailableCents != 12500 {
			t.Errorf("got available_cents %d, want 12500", resp.Data.Summary.AvailableCents)
		}
		if resp.Data.Summary.PaidCents != 0 || len(resp.Data.Payouts) != 0 {
			t.Errorf("expected paid_cents=0 and empty payouts, got %d / %v", resp.Data.Summary.PaidCents, resp.Data.Payouts)
		}

		// Transactions in 30d range: Order A and Order B (2 items; Order C is 45d old)
		if resp.Meta.Total != 2 || len(resp.Data.Transactions) != 2 {
			t.Fatalf("got %d transactions (total %d) for 30d range, want 2", len(resp.Data.Transactions), resp.Meta.Total)
		}
		if resp.Data.Transactions[0].OrderID != "order-A" || resp.Data.Transactions[0].NetCents != 9500 || resp.Data.Transactions[0].Status != "COMPLETED" {
			t.Errorf("unexpected tx[0]: %+v", resp.Data.Transactions[0])
		}
		if resp.Data.Transactions[1].OrderID != "order-B" || resp.Data.Transactions[1].NetCents != 13000 {
			t.Errorf("unexpected tx[1]: %+v", resp.Data.Transactions[1])
		}
	})

	t.Run("range_days=90 includes older order with status REFUNDED and pagination works", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/seller/earnings?range_days=90&page=2&page_size=2", nil)
		req.Header.Set("X-Actor-Role", "seller")
		req.Header.Set("X-Actor-ID", "seller-u1")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got HTTP %d, want 200", rec.Code)
		}
		var resp struct {
			Data SellerEarningsPayload `json:"data"`
			Meta PaginationMeta        `json:"meta"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)

		if resp.Meta.Total != 3 || resp.Meta.Page != 2 || len(resp.Data.Transactions) != 1 {
			t.Fatalf("unexpected page 2 meta/transactions: meta=%+v len=%d", resp.Meta, len(resp.Data.Transactions))
		}
		txC := resp.Data.Transactions[0]
		if txC.OrderID != "order-C" || txC.Status != "REFUNDED" || txC.NetCents != -500 {
			t.Errorf("expected order-C with status REFUNDED and net_cents=-500, got %+v", txC)
		}
	})

	t.Run("invalid range_days returns 400 and non-seller role returns 403", func(t *testing.T) {
		t.Parallel()

		reqInvalidRange := httptest.NewRequest(http.MethodGet, "/api/v1/seller/earnings?range_days=15", nil)
		reqInvalidRange.Header.Set("X-Actor-Role", "seller")
		reqInvalidRange.Header.Set("X-Actor-ID", "seller-u1")
		recInvalid := httptest.NewRecorder()
		router.ServeHTTP(recInvalid, reqInvalidRange)
		if recInvalid.Code != http.StatusBadRequest {
			t.Errorf("range_days=15 got HTTP %d, want 400", recInvalid.Code)
		}

		for _, role := range []string{"buyer", "admin"} {
			reqRole := httptest.NewRequest(http.MethodGet, "/api/v1/seller/earnings", nil)
			reqRole.Header.Set("X-Actor-Role", role)
			reqRole.Header.Set("X-Actor-ID", role+"-001")
			recRole := httptest.NewRecorder()
			router.ServeHTTP(recRole, reqRole)
			if recRole.Code != http.StatusForbidden {
				t.Errorf("role %s got HTTP %d, want 403", role, recRole.Code)
			}
		}
	})
}
