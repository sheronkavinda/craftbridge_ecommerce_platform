package report

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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

type fakeReportRepo struct {
	lastFilter DateRangeFilter
	payments   []float64
	refunds    []float64
}

func (f *fakeReportRepo) GetRevenueSummary(_ context.Context, filter DateRangeFilter) (*RevenueSummary, error) {
	f.lastFilter = filter

	var gross float64
	for _, p := range f.payments {
		gross += p
	}
	var totalRefunds float64
	for _, r := range f.refunds {
		totalRefunds += r
	}

	summary := &RevenueSummary{
		GrossVerifiedPayments: gross,
		TotalRefunds:          totalRefunds,
		NetRevenue:            gross - totalRefunds,
		VerifiedPaymentCount:  int64(len(f.payments)),
		RefundCount:           int64(len(f.refunds)),
		GeneratedAt:           time.Now().UTC(),
	}
	if filter.StartDate != nil {
		summary.StartDate = filter.StartDate.Format("2006-01-02")
	}
	if filter.EndDate != nil {
		summary.EndDate = filter.EndDate.Format("2006-01-02")
	}
	return summary, nil
}

func (f *fakeReportRepo) GetDailyRevenueBreakdown(_ context.Context, filter DateRangeFilter) ([]DailyRevenuePoint, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 365 {
		limit = 30
	}
	f.lastFilter = filter
	f.lastFilter.Limit = limit

	return []DailyRevenuePoint{
		{
			Date:             "2026-11-13",
			VerifiedPayments: 12500,
			Refunds:          500,
			NetRevenue:       12000,
		},
	}, nil
}

func (f *fakeReportRepo) GetSellerRevenueReport(_ context.Context, filter DateRangeFilter) ([]SellerSalesReport, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	f.lastFilter = filter
	f.lastFilter.Limit = limit

	return []SellerSalesReport{
		{
			SellerID:         "seller-001",
			StoreName:        "Sunil Wood Crafts",
			OrderCount:       3,
			VerifiedPayments: 26100,
			Refunds:          500,
			NetRevenue:       25600,
		},
	}, nil
}

func setupReportTestRouter(repo RepositoryStore) *gin.Engine {
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	svc := NewService(repo)
	h := NewHandler(svc)
	RegisterRoutes(v1, h)
	return engine
}

func addReportAdminHeaders(req *http.Request) {
	req.Header.Set("X-Actor-Role", "admin")
	req.Header.Set("X-Actor-ID", "admin-001")
	req.Header.Set("X-Actor-Name", "System Admin")
}

func TestService_DateRangeValidationAndRevenueCalculation(t *testing.T) {
	t.Parallel()

	t.Run("calculates net revenue as gross verified payments minus approved refunds", func(t *testing.T) {
		t.Parallel()
		repo := &fakeReportRepo{
			payments: []float64{12500, 8400, 5200, 3100},
			refunds:  []float64{500},
		}
		svc := NewService(repo)

		summary, err := svc.GetRevenueSummary(context.Background(), "2026-11-01", "2026-11-14")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary.GrossVerifiedPayments != 29200 {
			t.Errorf("got gross %.2f, want 29200", summary.GrossVerifiedPayments)
		}
		if summary.TotalRefunds != 500 {
			t.Errorf("got refunds %.2f, want 500", summary.TotalRefunds)
		}
		if summary.NetRevenue != 28700 {
			t.Errorf("got net %.2f, want 28700", summary.NetRevenue)
		}
	})

	t.Run("rejects invalid date formats and start_date after end_date", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name      string
			startDate string
			endDate   string
		}{
			{
				name:      "invalid start_date format",
				startDate: "14-11-2026",
				endDate:   "2026-11-14",
			},
			{
				name:      "invalid end_date format",
				startDate: "2026-11-01",
				endDate:   "2026/11/14",
			},
			{
				name:      "start_date after end_date",
				startDate: "2026-11-15",
				endDate:   "2026-11-10",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				svc := NewService(&fakeReportRepo{})
				_, err := svc.GetRevenueSummary(context.Background(), tc.startDate, tc.endDate)
				if !errors.Is(err, ErrInvalidDateRange) {
					t.Fatalf("got err %v, want ErrInvalidDateRange", err)
				}
			})
		}
	})
}

func TestHandler_ReportEndpointsAndLimitBounds(t *testing.T) {
	t.Parallel()

	t.Run("returns 400 Bad Request on invalid date range across all report endpoints", func(t *testing.T) {
		t.Parallel()
		router := setupReportTestRouter(&fakeReportRepo{})

		endpoints := []string{
			"/api/v1/reports/revenue?start_date=2026-11-20&end_date=2026-11-10",
			"/api/v1/reports/revenue/daily?start_date=not-a-date",
			"/api/v1/reports/sellers?end_date=2026-13-45",
		}

		for _, path := range endpoints {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			addReportAdminHeaders(req)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("GET %s got status %d, want 400", path, rec.Code)
			}
		}
	})

	t.Run("applies default and bounded limits for daily (1..365 default 30) and sellers (1..200 default 50)", func(t *testing.T) {
		t.Parallel()
		repo := &fakeReportRepo{}
		router := setupReportTestRouter(repo)

		// Daily default limit = 30
		reqDailyDefault := httptest.NewRequest(http.MethodGet, "/api/v1/reports/revenue/daily", nil)
		addReportAdminHeaders(reqDailyDefault)
		recDailyDefault := httptest.NewRecorder()
		router.ServeHTTP(recDailyDefault, reqDailyDefault)
		if recDailyDefault.Code != http.StatusOK || repo.lastFilter.Limit != 30 {
			t.Errorf("daily default: got status %d, limit %d; want 200, 30", recDailyDefault.Code, repo.lastFilter.Limit)
		}

		// Daily out-of-range limit (>365) clamped to 30
		reqDailyHigh := httptest.NewRequest(http.MethodGet, "/api/v1/reports/revenue/daily?limit=999", nil)
		addReportAdminHeaders(reqDailyHigh)
		recDailyHigh := httptest.NewRecorder()
		router.ServeHTTP(recDailyHigh, reqDailyHigh)
		if recDailyHigh.Code != http.StatusOK || repo.lastFilter.Limit != 30 {
			t.Errorf("daily >365: got status %d, limit %d; want 200, 30", recDailyHigh.Code, repo.lastFilter.Limit)
		}

		// Sellers default limit = 50
		reqSellersDefault := httptest.NewRequest(http.MethodGet, "/api/v1/reports/sellers", nil)
		addReportAdminHeaders(reqSellersDefault)
		recSellersDefault := httptest.NewRecorder()
		router.ServeHTTP(recSellersDefault, reqSellersDefault)
		if recSellersDefault.Code != http.StatusOK || repo.lastFilter.Limit != 50 {
			t.Errorf("sellers default: got status %d, limit %d; want 200, 50", recSellersDefault.Code, repo.lastFilter.Limit)
		}

		// Sellers out-of-range limit (<=0) clamped to 50
		reqSellersZero := httptest.NewRequest(http.MethodGet, "/api/v1/reports/seller-revenue?limit=0", nil)
		addReportAdminHeaders(reqSellersZero)
		recSellersZero := httptest.NewRecorder()
		router.ServeHTTP(recSellersZero, reqSellersZero)
		if recSellersZero.Code != http.StatusOK || repo.lastFilter.Limit != 50 {
			t.Errorf("sellers <=0: got status %d, limit %d; want 200, 50", recSellersZero.Code, repo.lastFilter.Limit)
		}

		var body map[string]any
		if err := json.Unmarshal(recSellersZero.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if _, ok := body["data"]; !ok {
			t.Errorf("expected 'data' envelope in response")
		}
	})
}
