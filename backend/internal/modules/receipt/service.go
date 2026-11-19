package receipt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"strings"
	"time"
)

var (
	// ErrInvalidReceiptInput indicates malformed or missing parameters.
	ErrInvalidReceiptInput = errors.New("invalid receipt request")
	// ErrUnauthorized indicates that the caller is not authenticated.
	ErrUnauthorized = errors.New("authentication required")
	// ErrForbidden indicates that the authenticated caller does not have the required buyer role.
	ErrForbidden = errors.New("forbidden: buyer role required")
	// ErrOrderNotFound indicates that the order does not exist or does not belong to the caller.
	ErrOrderNotFound = errors.New("order not found")
	// ErrNoVerifiedPayment indicates that the order has no verified SUCCESS payment attempt.
	ErrNoVerifiedPayment = errors.New("no verified payment for this order")
)

// RepositoryStore defines the persistence contract required by the receipt service.
type RepositoryStore interface {
	GetOrCreateByOrder(ctx context.Context, candidate Receipt) (*Receipt, error)
	SumCompletedRefundsForOrder(ctx context.Context, orderID string) (int64, error)
}

// PaymentReader defines the contract for loading an order's newest SUCCESS payment attempt and buyer metadata.
type PaymentReader interface {
	GetVerifiedPaymentForOrder(ctx context.Context, orderID string) (PaidOrderPayment, error)
}

// OrderSummaryReader is an optional interface for loading line items and subtotal/shipping/tax breakdowns when available.
type OrderSummaryReader interface {
	GetOrderSummary(ctx context.Context, orderID string) (OrderSummary, error)
}

// Service implements owner-restricted, lazily-issued digital payment receipts and printable HTML rendering.
type Service struct {
	repo           RepositoryStore
	payments       PaymentReader
	orderSummaries OrderSummaryReader
	nowFn          func() time.Time
}

// NewService creates a new receipt service.
func NewService(repo RepositoryStore, payments PaymentReader, orderSummaries OrderSummaryReader) *Service {
	return &Service{
		repo:           repo,
		payments:       payments,
		orderSummaries: orderSummaries,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// GetOrderReceipt lazily issues (or retrieves) the digital receipt for a buyer's paid order.
// It enforces strict owner-only access (404 for another buyer's order, 403 for non-buyers) and returns 409 when unpaid.
func (s *Service) GetOrderReceipt(ctx context.Context, actor Actor, orderID string) (*ReceiptView, error) {
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		return nil, ErrUnauthorized
	}
	if !strings.EqualFold(strings.TrimSpace(actor.Role), "BUYER") {
		return nil, ErrForbidden
	}

	trimmedOrderID := strings.TrimSpace(orderID)
	if trimmedOrderID == "" || len(trimmedOrderID) > 64 {
		return nil, ErrOrderNotFound
	}

	if s.payments == nil {
		return nil, ErrNoVerifiedPayment
	}

	paid, err := s.payments.GetVerifiedPaymentForOrder(ctx, trimmedOrderID)
	if err != nil {
		if errors.Is(err, ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		// Check ownership before returning ErrNoVerifiedPayment so another user's unpaid order never leaks existence.
		if strings.TrimSpace(paid.BuyerID) != "" && !strings.EqualFold(strings.TrimSpace(paid.BuyerID), actorID) {
			return nil, ErrOrderNotFound
		}
		if errors.Is(err, ErrNoVerifiedPayment) {
			return nil, ErrNoVerifiedPayment
		}
		return nil, err
	}

	// Owner restriction: return 404 if the order does not belong to the calling buyer.
	if strings.TrimSpace(paid.BuyerID) == "" || !strings.EqualFold(strings.TrimSpace(paid.BuyerID), actorID) {
		return nil, ErrOrderNotFound
	}

	if strings.TrimSpace(paid.AttemptID) == "" || paid.AmountCents <= 0 {
		return nil, ErrNoVerifiedPayment
	}

	now := s.nowFn()
	refDate := paid.CompletedAt
	if refDate.IsZero() {
		refDate = now
	}

	currency := strings.ToUpper(strings.TrimSpace(paid.Currency))
	if currency == "" {
		currency = "USD"
	}

	candidate := Receipt{
		ID:               generateReceiptID(),
		OrderID:          paid.OrderID,
		PaymentAttemptID: paid.AttemptID,
		UserID:           actorID,
		ReceiptNumber:    GenerateReceiptNumber(paid.OrderID, refDate),
		AmountCents:      paid.AmountCents,
		Currency:         currency,
		IssuedAt:         now,
	}

	stored, err := s.repo.GetOrCreateByOrder(ctx, candidate)
	if err != nil {
		return nil, err
	}

	refundedCents, err := s.repo.SumCompletedRefundsForOrder(ctx, paid.OrderID)
	if err != nil {
		return nil, err
	}

	items := make([]ReceiptLineItem, 0)
	if s.orderSummaries != nil {
		if summary, sumErr := s.orderSummaries.GetOrderSummary(ctx, paid.OrderID); sumErr == nil && len(summary.Items) > 0 {
			items = summary.Items
		}
	}

	netPaidCents := stored.AmountCents - refundedCents
	status := "PAID"
	if refundedCents >= stored.AmountCents {
		status = "REFUNDED"
	} else if refundedCents > 0 {
		status = "PARTIALLY_REFUNDED"
	}

	orderNumber := strings.TrimSpace(paid.OrderNumber)
	if orderNumber == "" {
		orderNumber = stored.OrderID
	}

	buyerName := strings.TrimSpace(paid.BuyerName)
	if buyerName == "" {
		buyerName = strings.TrimSpace(actor.Name)
	}

	providerRef := strings.TrimSpace(paid.ProviderReference)
	if providerRef == "" {
		providerRef = stored.PaymentAttemptID
	}

	return &ReceiptView{
		ReceiptNumber:     stored.ReceiptNumber,
		IssuedAt:          stored.IssuedAt,
		OrderID:           stored.OrderID,
		OrderNumber:       orderNumber,
		BuyerName:         buyerName,
		BuyerEmail:        strings.TrimSpace(paid.BuyerEmail),
		PaymentMethod:     FormatPaymentMethodLabel(paid.Provider),
		ProviderReference: providerRef,
		AmountCents:       stored.AmountCents,
		Amount:            FormatCentsDecimal(stored.AmountCents),
		Currency:          stored.Currency,
		Items:             items,
		RefundedCents:     refundedCents,
		NetPaidCents:      netPaidCents,
		Status:            status,
	}, nil
}

type printableReceiptTemplateData struct {
	Receipt       *ReceiptView
	IssuedAtStr   string
	RefundedStr   string
	NetPaidStr    string
	HasRefunds    bool
	HasOrderItems bool
}

var printableReceiptTmpl = template.Must(template.New("receipt").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Receipt {{.Receipt.ReceiptNumber}} - CraftBridge</title>
  <style>
    body { font-family: system-ui, -apple-system, sans-serif; color: #1e293b; max-width: 720px; margin: 2rem auto; padding: 0 1.5rem; line-height: 1.5; }
    .card { border: 1px solid #cbd5e1; border-radius: 8px; padding: 2rem; background: #ffffff; }
    .header { display: flex; justify-content: space-between; border-bottom: 2px solid #e2e8f0; padding-bottom: 1rem; margin-bottom: 1.5rem; }
    .brand { font-size: 1.5rem; font-weight: 700; color: #0f172a; }
    .badge { display: inline-block; padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.75rem; font-weight: 600; background: #dcfce7; color: #166534; }
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1.5rem; font-size: 0.9rem; }
    .label { color: #64748b; font-size: 0.75rem; text-transform: uppercase; letter-spacing: 0.05em; }
    table { width: 100%; border-collapse: collapse; margin-top: 1rem; }
    th, td { text-align: left; padding: 0.75rem 0.5rem; border-bottom: 1px solid #e2e8f0; }
    .right { text-align: right; }
    .totals { margin-top: 1.5rem; border-top: 2px solid #0f172a; padding-top: 1rem; }
    .row { display: flex; justify-content: space-between; margin: 0.35rem 0; }
    .net { font-size: 1.15rem; font-weight: 700; color: #0f172a; }
    .footer { margin-top: 2rem; font-size: 0.8rem; color: #64748b; text-align: center; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <div>
        <div class="brand">CraftBridge</div>
        <div>Official Payment Receipt</div>
      </div>
      <div class="right">
        <div><strong>{{.Receipt.ReceiptNumber}}</strong></div>
        <div><span class="badge">{{.Receipt.Status}}</span></div>
      </div>
    </div>

    <div class="grid">
      <div>
        <div class="label">Billed To</div>
        <div><strong>{{.Receipt.BuyerName}}</strong></div>
        <div>{{.Receipt.BuyerEmail}}</div>
      </div>
      <div class="right">
        <div class="label">Order &amp; Payment Details</div>
        <div>Order: <strong>{{.Receipt.OrderNumber}}</strong> ({{.Receipt.OrderID}})</div>
        <div>Issued: {{.IssuedAtStr}}</div>
        <div>Method: {{.Receipt.PaymentMethod}}</div>
        <div>Reference: {{.Receipt.ProviderReference}}</div>
      </div>
    </div>

    {{if .HasOrderItems}}
    <table>
      <thead>
        <tr>
          <th>Item</th>
          <th class="right">Qty</th>
          <th class="right">Unit Price</th>
          <th class="right">Total</th>
        </tr>
      </thead>
      <tbody>
        {{range .Receipt.Items}}
        <tr>
          <td>{{.Title}}</td>
          <td class="right">{{.Quantity}}</td>
          <td class="right">{{$.Receipt.Currency}} {{.UnitPrice}}</td>
          <td class="right">{{$.Receipt.Currency}} {{.LineTotal}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
    {{end}}

    <div class="totals">
      <div class="row">
        <span>Gross Paid Amount</span>
        <span>{{.Receipt.Currency}} {{.Receipt.Amount}}</span>
      </div>
      {{if .HasRefunds}}
      <div class="row">
        <span>Completed Refunds</span>
        <span>-{{.Receipt.Currency}} {{.RefundedStr}}</span>
      </div>
      {{end}}
      <div class="row net">
        <span>Net Paid</span>
        <span>{{.Receipt.Currency}} {{.NetPaidStr}}</span>
      </div>
    </div>

    <div class="footer">
      Thank you for supporting independent artisans on CraftBridge.
    </div>
  </div>
</body>
</html>
`))

// RenderReceiptHTML renders a ReceiptView as a self-contained printable HTML document using html/template.
// All dynamic fields are automatically HTML-escaped by html/template.
func (s *Service) RenderReceiptHTML(view *ReceiptView) ([]byte, error) {
	if view == nil {
		return nil, ErrInvalidReceiptInput
	}

	data := printableReceiptTemplateData{
		Receipt:       view,
		IssuedAtStr:   view.IssuedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		RefundedStr:   FormatCentsDecimal(view.RefundedCents),
		NetPaidStr:    FormatCentsDecimal(view.NetPaidCents),
		HasRefunds:    view.RefundedCents > 0,
		HasOrderItems: len(view.Items) > 0,
	}

	var buf bytes.Buffer
	if err := printableReceiptTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to render receipt HTML template: %w", err)
	}
	return buf.Bytes(), nil
}

func generateReceiptID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("rcp-%d", time.Now().UnixNano())
	}
	return "rcp-" + hex.EncodeToString(b)
}
