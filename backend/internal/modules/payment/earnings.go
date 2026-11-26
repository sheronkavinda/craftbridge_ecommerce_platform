package payment

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	defaultPlatformCommissionPercent = 10
	defaultPayoutHoldDays            = 7
)

// SellerPaidOrderRow represents a single order with a verified SUCCESS payment attempt belonging to a seller.
type SellerPaidOrderRow struct {
	OrderID          string
	OrderNumber      string
	ProductTitle     string
	PaidAt           time.Time
	MerchandiseCents int64
	ShippingCents    int64
	CompletedRefunds int64
	Currency         string
	PaymentAttemptID string
}

// SellerPayoutRecord represents a recorded seller payout when the seller_payouts table exists.
type SellerPayoutRecord struct {
	ID          string    `json:"id"`
	AmountCents int64     `json:"amount_cents"`
	Amount      string    `json:"amount"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// SellerEarningsSummary holds the all-time calculated earnings balances for a seller in integer cents.
type SellerEarningsSummary struct {
	PendingCents   int64  `json:"pending_cents"`
	AvailableCents int64  `json:"available_cents"`
	PaidCents      int64  `json:"paid_cents"`
	Currency       string `json:"currency"`
}

// SellerEarningsTransaction represents a single paid order's earnings breakdown for a seller.
type SellerEarningsTransaction struct {
	OrderID          string    `json:"order_id"`
	OrderNumber      string    `json:"order_number"`
	Date             time.Time `json:"date"`
	ProductTitle     string    `json:"product_title"`
	GrossCents       int64     `json:"gross_cents"`
	ShippingCents    int64     `json:"shipping_cents"`
	CommissionCents  int64     `json:"commission_cents"`
	RefundCents      int64     `json:"refund_cents"`
	AdjustmentsCents int64     `json:"adjustments_cents"`
	NetCents         int64     `json:"net_cents"`
	Status           string    `json:"status"`
}

// SellerEarningsPayload represents the data envelope returned by GET /api/v1/seller/earnings.
type SellerEarningsPayload struct {
	Summary      SellerEarningsSummary       `json:"summary"`
	Transactions []SellerEarningsTransaction `json:"transactions"`
	Payouts      []SellerPayoutRecord        `json:"payouts"`
}

// SellerSalesReader defines the contract for resolving a seller identity and loading verified paid orders and payouts.
type SellerSalesReader interface {
	ResolveSellerID(ctx context.Context, actorID string) (string, error)
	ListSellerPaidOrders(ctx context.Context, sellerID string) ([]SellerPaidOrderRow, error)
	ListSellerPayouts(ctx context.Context, sellerID string) ([]SellerPayoutRecord, int64, error)
}

// ComputeCommissionCents calculates round-half-up(merchandise_cents * commissionPercent / 100) using integer arithmetic.
func ComputeCommissionCents(merchandiseCents int64, commissionPercent int) int64 {
	if merchandiseCents <= 0 || commissionPercent <= 0 {
		return 0
	}
	return (merchandiseCents*int64(commissionPercent) + 50) / 100
}

// GetSellerEarnings calculates a seller's all-time earnings summary (pending vs available vs paid)
// and returns the paginated transaction breakdown for the requested range_days (7, 30, or 90).
func (s *Service) GetSellerEarnings(
	ctx context.Context,
	actor Actor,
	rangeDays int,
	page int,
	pageSize int,
) (*SellerEarningsPayload, PaginationMeta, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, PaginationMeta{}, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "SELLER") {
		return nil, PaginationMeta{}, ErrForbidden
	}

	if rangeDays != 7 && rangeDays != 30 && rangeDays != 90 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: range_days must be 7, 30, or 90", ErrInvalidPaymentInput)
	}
	if page < 1 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: page must be >= 1", ErrInvalidPaymentInput)
	}
	if pageSize < 1 {
		return nil, PaginationMeta{}, fmt.Errorf("%w: page_size must be >= 1", ErrInvalidPaymentInput)
	}
	if pageSize > 100 {
		pageSize = 100
	}

	if s.salesReader == nil {
		return &SellerEarningsPayload{
			Summary:      SellerEarningsSummary{Currency: "USD"},
			Transactions: []SellerEarningsTransaction{},
			Payouts:      []SellerPayoutRecord{},
		}, PaginationMeta{Page: page, PageSize: pageSize, Total: 0}, nil
	}

	sellerID, err := s.salesReader.ResolveSellerID(ctx, actorID)
	if err != nil {
		return nil, PaginationMeta{}, err
	}
	if strings.TrimSpace(sellerID) == "" {
		sellerID = actorID
	}

	paidOrders, err := s.salesReader.ListSellerPaidOrders(ctx, sellerID)
	if err != nil {
		return nil, PaginationMeta{}, err
	}

	payouts, paidCents, err := s.salesReader.ListSellerPayouts(ctx, sellerID)
	if err != nil {
		return nil, PaginationMeta{}, err
	}
	if payouts == nil {
		payouts = []SellerPayoutRecord{}
	}

	commissionPct := s.commissionPercent
	if commissionPct < 0 {
		commissionPct = defaultPlatformCommissionPercent
	}
	holdDays := s.payoutHoldDays
	if holdDays < 0 {
		holdDays = defaultPayoutHoldDays
	}

	now := s.nowFn()
	holdCutoff := now.Add(-time.Duration(holdDays) * 24 * time.Hour)
	rangeCutoff := now.Add(-time.Duration(rangeDays) * 24 * time.Hour)

	var (
		pendingCents    int64
		settledNetCents int64
		summaryCurrency = "USD"
		rangedTx        = make([]SellerEarningsTransaction, 0)
	)

	// Sort paidOrders newest first by PaidAt DESC, OrderID DESC
	sorted := make([]SellerPaidOrderRow, len(paidOrders))
	copy(sorted, paidOrders)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].PaidAt.After(sorted[i].PaidAt) ||
				(sorted[j].PaidAt.Equal(sorted[i].PaidAt) && sorted[j].OrderID > sorted[i].OrderID) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	for _, row := range sorted {
		if cur := strings.ToUpper(strings.TrimSpace(row.Currency)); cur != "" {
			summaryCurrency = cur
		}

		commCents := ComputeCommissionCents(row.MerchandiseCents, commissionPct)
		netCents := row.MerchandiseCents + row.ShippingCents - commCents - row.CompletedRefunds

		// Pending = net of orders whose payment completed less than PAYOUT_HOLD_DAYS ago.
		// Available = net of older orders (>= PAYOUT_HOLD_DAYS ago) minus Paid.
		if row.PaidAt.After(holdCutoff) {
			pendingCents += netCents
		} else {
			settledNetCents += netCents
		}

		if !row.PaidAt.Before(rangeCutoff) {
			totalOrderGross := row.MerchandiseCents + row.ShippingCents
			status := "COMPLETED"
			if totalOrderGross > 0 && row.CompletedRefunds >= totalOrderGross {
				status = "REFUNDED"
			}

			orderNum := strings.TrimSpace(row.OrderNumber)
			if orderNum == "" {
				orderNum = row.OrderID
			}

			rangedTx = append(rangedTx, SellerEarningsTransaction{
				OrderID:          row.OrderID,
				OrderNumber:      orderNum,
				Date:             row.PaidAt,
				ProductTitle:     row.ProductTitle,
				GrossCents:       row.MerchandiseCents,
				ShippingCents:    row.ShippingCents,
				CommissionCents:  commCents,
				RefundCents:      row.CompletedRefunds,
				AdjustmentsCents: 0,
				NetCents:         netCents,
				Status:           status,
			})
		}
	}

	availableCents := settledNetCents - paidCents
	totalRanged := int64(len(rangedTx))

	offset := (page - 1) * pageSize
	var pageItems []SellerEarningsTransaction
	if offset >= len(rangedTx) {
		pageItems = []SellerEarningsTransaction{}
	} else {
		end := offset + pageSize
		if end > len(rangedTx) {
			end = len(rangedTx)
		}
		pageItems = rangedTx[offset:end]
	}

	return &SellerEarningsPayload{
			Summary: SellerEarningsSummary{
				PendingCents:   pendingCents,
				AvailableCents: availableCents,
				PaidCents:      paidCents,
				Currency:       summaryCurrency,
			},
			Transactions: pageItems,
			Payouts:      payouts,
		}, PaginationMeta{
			Page:     page,
			PageSize: pageSize,
			Total:    totalRanged,
		}, nil
}

// ResolveSellerID maps an acting user ID (from JWT sub) to shops.owner_user_id::text,
// falling back to the actor ID itself if no shop record matches.
func (r *Repository) ResolveSellerID(ctx context.Context, actorID string) (string, error) {
	trimmed := strings.TrimSpace(actorID)
	if trimmed == "" {
		return "", nil
	}

	const query = `
		SELECT sh.owner_user_id::text
		FROM shops sh
		WHERE sh.owner_user_id::text = $1 OR sh.id::text = $1
		ORDER BY CASE WHEN sh.owner_user_id::text = $1 THEN 0 ELSE 1 END
		LIMIT 1
	`

	var resolved string
	err := r.querier(ctx).QueryRow(ctx, query, trimmed).Scan(&resolved)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return trimmed, nil
		}
		log.Printf("[req_id=%s] payment.Repository.ResolveSellerID fallback (actor_id=%s): %v", reqIDFromContext(ctx), trimmed, err)
		return trimmed, nil
	}

	return resolved, nil
}

// ListSellerPaidOrders returns one row per seller_order belonging to sellerID's shops whose parent
// order has a verified SUCCESS payment attempt. Unpaid, PENDING, FAILED, EXPIRED, or CANCELLED
// payment attempts are never included.
//
// Gross is the seller_order's subtotal_cents (commission applies to subtotal_cents) and shipping_cents
// is that seller_order's shipping_cents, matching net = merchandise + shipping - commission - refunds.
// Completed refunds are subtracted only when refunds.seller_id matches sellerID, or when refunds.seller_id
// is empty and the parent order has a single seller_order.
func (r *Repository) ListSellerPaidOrders(ctx context.Context, sellerID string) ([]SellerPaidOrderRow, error) {
	const query = `
		SELECT
			o.id::text AS order_id,
			COALESCE(
				(
					SELECT CASE
						WHEN COUNT(*) <= 1 THEN COALESCE(MAX(oi.product_title_snapshot), '')
						ELSE MIN(oi.product_title_snapshot) || ' +' || (COUNT(*) - 1)::text || ' more'
					END
					FROM order_items oi
					WHERE oi.seller_order_id = so.id
				),
				''
			) AS product_title,
			pa.id::text AS attempt_id,
			so.subtotal_cents,
			so.shipping_cents,
			COALESCE(NULLIF(UPPER(pa.currency), ''), 'USD') AS currency,
			COALESCE(pa.completed_at, pa.updated_at, pa.created_at) AS paid_at,
			COALESCE(
				(
					SELECT SUM(COALESCE(NULLIF(rf.amount_cents, 0), ROUND(COALESCE(rf.amount, 0) * 100)::bigint))
					FROM refunds rf
					WHERE rf.order_id::text = o.id::text
					  AND rf.status = 'COMPLETED'
					  AND (
						  rf.seller_id::text = $1
						  OR (
							  (rf.seller_id IS NULL OR BTRIM(rf.seller_id) = '')
							  AND (SELECT COUNT(*) FROM seller_orders so2 WHERE so2.order_id = o.id) = 1
						  )
					  )
				),
				0
			)::bigint AS completed_refunds
		FROM seller_orders so
		JOIN shops sh ON sh.id = so.shop_id
		JOIN orders o ON o.id = so.order_id
		JOIN LATERAL (
			SELECT
				id,
				currency,
				completed_at,
				updated_at,
				created_at
			FROM payment_attempts p
			WHERE p.order_id::text = o.id::text
			  AND p.status = 'SUCCESS'
			ORDER BY p.completed_at DESC NULLS LAST, p.created_at DESC
			LIMIT 1
		) pa ON TRUE
		WHERE sh.owner_user_id::text = $1
		ORDER BY COALESCE(pa.completed_at, pa.updated_at, pa.created_at) DESC, o.id DESC, so.id DESC
	`

	rows, err := r.querier(ctx).Query(ctx, query, sellerID)
	if err != nil {
		log.Printf("[req_id=%s] payment.Repository.ListSellerPaidOrders error (seller_id=%s): %v", reqIDFromContext(ctx), sellerID, err)
		return nil, fmt.Errorf("failed to list seller paid orders: %w", err)
	}
	defer rows.Close()

	result := make([]SellerPaidOrderRow, 0)
	for rows.Next() {
		var (
			orderID          string
			productTitle     string
			attemptID        string
			merchandiseCents int64
			shippingCents    int64
			currency         string
			paidAt           time.Time
			completedRefunds int64
		)
		if scanErr := rows.Scan(
			&orderID,
			&productTitle,
			&attemptID,
			&merchandiseCents,
			&shippingCents,
			&currency,
			&paidAt,
			&completedRefunds,
		); scanErr != nil {
			return nil, fmt.Errorf("failed to scan seller paid order row: %w", scanErr)
		}

		result = append(result, SellerPaidOrderRow{
			OrderID:          orderID,
			OrderNumber:      orderID,
			ProductTitle:     productTitle,
			PaidAt:           paidAt,
			MerchandiseCents: merchandiseCents,
			ShippingCents:    shippingCents,
			CompletedRefunds: completedRefunds,
			Currency:         currency,
			PaymentAttemptID: attemptID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating seller paid orders: %w", err)
	}

	return result, nil
}

// ListSellerPayouts checks if the optional seller_payouts table exists (via to_regclass).
// Because Member 06 owns payout settlement, if seller_payouts does not exist it returns an empty slice and 0 paid_cents.
func (r *Repository) ListSellerPayouts(ctx context.Context, sellerID string) ([]SellerPayoutRecord, int64, error) {
	q := r.querier(ctx)
	var tableExists bool
	if err := q.QueryRow(ctx, `SELECT to_regclass('public.seller_payouts') IS NOT NULL`).Scan(&tableExists); err != nil || !tableExists {
		return []SellerPayoutRecord{}, 0, nil
	}

	const query = `
		SELECT
			id::text,
			COALESCE(amount_cents, 0)::bigint,
			COALESCE(NULLIF(UPPER(currency), ''), 'USD'),
			COALESCE(status, 'COMPLETED'),
			created_at
		FROM seller_payouts
		WHERE seller_id::text = $1
		ORDER BY created_at DESC
	`

	rows, err := q.Query(ctx, query, sellerID)
	if err != nil {
		return []SellerPayoutRecord{}, 0, nil
	}
	defer rows.Close()

	payouts := make([]SellerPayoutRecord, 0)
	var totalPaidCents int64
	for rows.Next() {
		var rec SellerPayoutRecord
		if scanErr := rows.Scan(&rec.ID, &rec.AmountCents, &rec.Currency, &rec.Status, &rec.CreatedAt); scanErr != nil {
			continue
		}
		rec.Amount = FormatCentsDecimal(rec.AmountCents)
		if strings.EqualFold(rec.Status, "COMPLETED") || strings.EqualFold(rec.Status, "PAID") {
			totalPaidCents += rec.AmountCents
		}
		payouts = append(payouts, rec)
	}

	return payouts, totalPaidCents, nil
}
