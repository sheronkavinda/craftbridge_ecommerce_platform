package report

import "time"

// RevenueSummary represents aggregated platform sales and net revenue metrics (verified payments minus refunds).
type RevenueSummary struct {
	GrossVerifiedPayments float64   `json:"gross_verified_payments"`
	TotalRefunds          float64   `json:"total_refunds"`
	NetRevenue            float64   `json:"net_revenue"`
	VerifiedPaymentCount  int64     `json:"verified_payment_count"`
	RefundCount           int64     `json:"refund_count"`
	StartDate             string    `json:"start_date,omitempty"`
	EndDate               string    `json:"end_date,omitempty"`
	GeneratedAt           time.Time `json:"generated_at"`
}

// DailyRevenuePoint represents daily aggregated verified payments, refunds, and net revenue.
type DailyRevenuePoint struct {
	Date             string  `json:"date"`
	VerifiedPayments float64 `json:"verified_payments"`
	Refunds          float64 `json:"refunds"`
	NetRevenue       float64 `json:"net_revenue"`
}

// SellerSalesReport represents aggregated sales and refund metrics per seller.
type SellerSalesReport struct {
	SellerID         string  `json:"seller_id"`
	StoreName        string  `json:"store_name"`
	OrderCount       int64   `json:"order_count"`
	VerifiedPayments float64 `json:"verified_payments"`
	Refunds          float64 `json:"refunds"`
	NetRevenue       float64 `json:"net_revenue"`
}

// DateRangeFilter holds optional start and end timestamps for report queries.
type DateRangeFilter struct {
	StartDate *time.Time
	EndDate   *time.Time
	Limit     int
}
