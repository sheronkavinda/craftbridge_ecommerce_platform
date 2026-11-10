package report

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository executes parameterized SQL aggregation queries for sales and revenue reporting.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new report repository backed by a pgx connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// GetRevenueSummary calculates gross verified payments minus approved/completed refunds over an optional date range.
func (r *Repository) GetRevenueSummary(ctx context.Context, filter DateRangeFilter) (*RevenueSummary, error) {
	const query = `
		WITH payment_totals AS (
			SELECT
				COALESCE(SUM(amount), 0)::float8 AS gross_payments,
				COUNT(*)::bigint AS payment_count
			FROM payments
			WHERE status IN ('VERIFIED', 'COMPLETED', 'PAID')
			  AND ($1::timestamptz IS NULL OR created_at >= $1)
			  AND ($2::timestamptz IS NULL OR created_at <= $2)
		),
		refund_totals AS (
			SELECT
				COALESCE(SUM(amount), 0)::float8 AS total_refunds,
				COUNT(*)::bigint AS refund_count
			FROM refunds
			WHERE status IN ('APPROVED', 'PROCESSED', 'COMPLETED')
			  AND ($1::timestamptz IS NULL OR created_at >= $1)
			  AND ($2::timestamptz IS NULL OR created_at <= $2)
		)
		SELECT
			p.gross_payments,
			r.total_refunds,
			(p.gross_payments - r.total_refunds)::float8 AS net_revenue,
			p.payment_count,
			r.refund_count
		FROM payment_totals p
		CROSS JOIN refund_totals r
	`

	var summary RevenueSummary
	err := r.db.QueryRow(ctx, query, filter.StartDate, filter.EndDate).Scan(
		&summary.GrossVerifiedPayments,
		&summary.TotalRefunds,
		&summary.NetRevenue,
		&summary.VerifiedPaymentCount,
		&summary.RefundCount,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query revenue summary: %w", err)
	}

	if filter.StartDate != nil {
		summary.StartDate = filter.StartDate.Format("2006-01-02")
	}
	if filter.EndDate != nil {
		summary.EndDate = filter.EndDate.Format("2006-01-02")
	}
	summary.GeneratedAt = time.Now().UTC()

	return &summary, nil
}

// GetDailyRevenueBreakdown aggregates daily verified payments minus daily refunds.
func (r *Repository) GetDailyRevenueBreakdown(ctx context.Context, filter DateRangeFilter) ([]DailyRevenuePoint, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 365 {
		limit = 30
	}

	const query = `
		WITH daily_payments AS (
			SELECT
				DATE(created_at) AS day,
				COALESCE(SUM(amount), 0)::float8 AS verified_payments
			FROM payments
			WHERE status IN ('VERIFIED', 'COMPLETED', 'PAID')
			  AND ($1::timestamptz IS NULL OR created_at >= $1)
			  AND ($2::timestamptz IS NULL OR created_at <= $2)
			GROUP BY DATE(created_at)
		),
		daily_refunds AS (
			SELECT
				DATE(created_at) AS day,
				COALESCE(SUM(amount), 0)::float8 AS refunds
			FROM refunds
			WHERE status IN ('APPROVED', 'PROCESSED', 'COMPLETED')
			  AND ($1::timestamptz IS NULL OR created_at >= $1)
			  AND ($2::timestamptz IS NULL OR created_at <= $2)
			GROUP BY DATE(created_at)
		)
		SELECT
			TO_CHAR(COALESCE(p.day, r.day), 'YYYY-MM-DD') AS report_date,
			COALESCE(p.verified_payments, 0)::float8 AS verified_payments,
			COALESCE(r.refunds, 0)::float8 AS refunds,
			(COALESCE(p.verified_payments, 0) - COALESCE(r.refunds, 0))::float8 AS net_revenue
		FROM daily_payments p
		FULL OUTER JOIN daily_refunds r ON p.day = r.day
		ORDER BY COALESCE(p.day, r.day) DESC
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, filter.StartDate, filter.EndDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily revenue breakdown: %w", err)
	}
	defer rows.Close()

	points := make([]DailyRevenuePoint, 0)
	for rows.Next() {
		var pt DailyRevenuePoint
		if err := rows.Scan(&pt.Date, &pt.VerifiedPayments, &pt.Refunds, &pt.NetRevenue); err != nil {
			return nil, fmt.Errorf("failed to scan daily revenue row: %w", err)
		}
		points = append(points, pt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily revenue rows: %w", err)
	}

	return points, nil
}

// GetSellerRevenueReport aggregates verified payments minus refunds grouped by seller using pre-aggregated CTEs.
func (r *Repository) GetSellerRevenueReport(ctx context.Context, filter DateRangeFilter) ([]SellerSalesReport, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	const query = `
		WITH order_payments AS (
			SELECT
				order_id,
				COALESCE(SUM(amount), 0)::float8 AS verified_amount
			FROM payments
			WHERE status IN ('VERIFIED', 'COMPLETED', 'PAID')
			  AND ($1::timestamptz IS NULL OR created_at >= $1)
			  AND ($2::timestamptz IS NULL OR created_at <= $2)
			GROUP BY order_id
		),
		order_refunds AS (
			SELECT
				order_id,
				COALESCE(SUM(amount), 0)::float8 AS refund_amount
			FROM refunds
			WHERE status IN ('APPROVED', 'PROCESSED', 'COMPLETED')
			  AND ($1::timestamptz IS NULL OR created_at >= $1)
			  AND ($2::timestamptz IS NULL OR created_at <= $2)
			GROUP BY order_id
		)
		SELECT
			s.id::text AS seller_id,
			s.store_name,
			COUNT(DISTINCT o.id)::bigint AS order_count,
			COALESCE(SUM(op.verified_amount), 0)::float8 AS verified_payments,
			COALESCE(SUM(orf.refund_amount), 0)::float8 AS refunds,
			(
				COALESCE(SUM(op.verified_amount), 0) -
				COALESCE(SUM(orf.refund_amount), 0)
			)::float8 AS net_revenue
		FROM sellers s
		LEFT JOIN orders o ON o.seller_id = s.id
		LEFT JOIN order_payments op ON op.order_id = o.id
		LEFT JOIN order_refunds orf ON orf.order_id = o.id
		GROUP BY s.id, s.store_name
		ORDER BY net_revenue DESC
		LIMIT $3
	`

	rows, err := r.db.Query(ctx, query, filter.StartDate, filter.EndDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query seller revenue report: %w", err)
	}
	defer rows.Close()

	reports := make([]SellerSalesReport, 0)
	for rows.Next() {
		var item SellerSalesReport
		if err := rows.Scan(
			&item.SellerID,
			&item.StoreName,
			&item.OrderCount,
			&item.VerifiedPayments,
			&item.Refunds,
			&item.NetRevenue,
		); err != nil {
			return nil, fmt.Errorf("failed to scan seller revenue row: %w", err)
		}
		reports = append(reports, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating seller revenue rows: %w", err)
	}

	return reports, nil
}
