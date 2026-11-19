package refund

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// RefundStatus represents the review and processing state of a refund request.
type RefundStatus string

const (
	RefundStatusPending   RefundStatus = "PENDING"
	RefundStatusApproved  RefundStatus = "APPROVED"
	RefundStatusProcessed RefundStatus = "PROCESSED"
	RefundStatusCompleted RefundStatus = "COMPLETED"
	RefundStatusRejected  RefundStatus = "REJECTED"
)

// IsValidRefundStatus reports whether s is a valid RefundStatus value.
func IsValidRefundStatus(s RefundStatus) bool {
	switch s {
	case RefundStatusPending, RefundStatusApproved, RefundStatusProcessed, RefundStatusCompleted, RefundStatusRejected:
		return true
	default:
		return false
	}
}

// Allowed buyer refund reason codes.
const (
	ReasonArrivedDamaged = "ARRIVED_DAMAGED"
	ReasonNotAsDescribed = "NOT_AS_DESCRIBED"
	ReasonWrongItem      = "WRONG_ITEM"
	ReasonLateDelivery   = "LATE_DELIVERY"
	ReasonChangedMind    = "CHANGED_MIND"
	ReasonOther          = "OTHER"
)

// IsValidRefundReason reports whether reason is one of the allowed buyer refund reasons.
func IsValidRefundReason(reason string) bool {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case ReasonArrivedDamaged,
		ReasonNotAsDescribed,
		ReasonWrongItem,
		ReasonLateDelivery,
		ReasonChangedMind,
		ReasonOther:
		return true
	default:
		return false
	}
}

// Refund represents a buyer or seller-initiated refund request linked to an order and payment attempt.
type Refund struct {
	ID                      string       `json:"id"`
	OrderID                 string       `json:"order_id"`
	PaymentAttemptID        *string      `json:"payment_attempt_id,omitempty"`
	RequestedBy             string       `json:"requested_by"`
	SellerID                *string      `json:"seller_id,omitempty"`
	Reason                  string       `json:"reason"`
	Details                 string       `json:"details"`
	Amount                  float64      `json:"amount"`
	AmountCents             int64        `json:"amount_cents"`
	Currency                string       `json:"currency"`
	Status                  RefundStatus `json:"status"`
	ProviderRefundReference *string      `json:"provider_refund_reference,omitempty"`
	IdempotencyKey          *string      `json:"-"`
	RequestedAt             time.Time    `json:"requested_at"`
	ReviewedAt              *time.Time   `json:"reviewed_at,omitempty"`
	DecidedAt               *time.Time   `json:"decided_at,omitempty"`
	DecidedBy               *string      `json:"decided_by,omitempty"`
	CompletedAt             *time.Time   `json:"completed_at,omitempty"`
	CreatedAt               time.Time    `json:"created_at"`
	UpdatedAt               time.Time    `json:"updated_at"`
}

// Actor represents the authenticated caller initiating a refund operation.
type Actor struct {
	ID   string
	Name string
	Role string
}

// PaidPayment holds the verified payment attempt and order ownership metadata for an order.
type PaidPayment struct {
	AttemptID         string
	OrderID           string
	OrderNumber       string
	BuyerID           string
	SellerID          string
	ProviderReference string
	AmountCents       int64
	Currency          string
	CompletedAt       time.Time
}

// GatewayRefundOutcome represents the status returned by a payment provider's refund API.
type GatewayRefundOutcome string

const (
	GatewayOutcomeCompleted GatewayRefundOutcome = "COMPLETED"
	GatewayOutcomePending   GatewayRefundOutcome = "PENDING"
	GatewayOutcomeFailed    GatewayRefundOutcome = "FAILED"
)

// GatewayRefundInput contains the parameters passed to the refund gateway adapter.
type GatewayRefundInput struct {
	ProviderReference string
	AmountCents       int64
	Currency          string
	Reason            string
	IdempotencyKey    string
}

// GatewayRefundOutput contains the normalized outcome returned by the refund gateway adapter.
type GatewayRefundOutput struct {
	ProviderRefundReference string
	Status                  GatewayRefundOutcome
	Raw                     string
}

// CreateRefundRequestInput represents the request body for POST /api/v1/orders/:id/refund-requests.
type CreateRefundRequestInput struct {
	Reason         string `json:"reason"`
	Details        string `json:"details"`
	Amount         string `json:"amount"`
	IdempotencyKey string `json:"-"`
}

// ListBuyerRefundsFilter holds pagination and status filtering for GET /api/v1/me/refunds.
type ListBuyerRefundsFilter struct {
	UserID   string
	Status   RefundStatus
	Page     int
	PageSize int
}

// PaginationMeta holds pagination metadata for list responses.
type PaginationMeta struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}

// RefundTimelineStep represents a single step in the buyer-facing refund progress timeline.
type RefundTimelineStep struct {
	Step string     `json:"step"`
	At   *time.Time `json:"at,omitempty"`
	Done bool       `json:"done"`
}

// BuyerRefundView represents a buyer-facing refund record with a derived status timeline.
// Internal provider responses, provider refund references, and idempotency keys are never exposed.
type BuyerRefundView struct {
	ID          string               `json:"id"`
	OrderID     string               `json:"order_id"`
	OrderNumber string               `json:"order_number"`
	Status      RefundStatus         `json:"status"`
	Reason      string               `json:"reason"`
	Details     string               `json:"details"`
	AmountCents int64                `json:"amount_cents"`
	Amount      string               `json:"amount"`
	Currency    string               `json:"currency"`
	RequestedAt time.Time            `json:"requested_at"`
	DecidedAt   *time.Time           `json:"decided_at,omitempty"`
	CompletedAt *time.Time           `json:"completed_at,omitempty"`
	Timeline    []RefundTimelineStep `json:"timeline"`
}

// BuildRefundTimeline derives the 4 buyer-facing timeline steps (submitted, under_review, approved, refund_issued)
// from a refund's status and timestamps.
func BuildRefundTimeline(r Refund) []RefundTimelineStep {
	submittedAt := r.RequestedAt
	if submittedAt.IsZero() {
		submittedAt = r.CreatedAt
	}

	decidedAt := r.DecidedAt
	if decidedAt == nil {
		decidedAt = r.ReviewedAt
	}

	underReviewAt := &submittedAt
	if decidedAt != nil {
		underReviewAt = decidedAt
	}

	approvedDone := r.Status == RefundStatusApproved ||
		r.Status == RefundStatusProcessed ||
		r.Status == RefundStatusCompleted
	var approvedAt *time.Time
	if approvedDone {
		approvedAt = decidedAt
	}

	issuedDone := r.Status == RefundStatusCompleted
	var issuedAt *time.Time
	if issuedDone {
		issuedAt = r.CompletedAt
	}

	return []RefundTimelineStep{
		{
			Step: "submitted",
			At:   &submittedAt,
			Done: true,
		},
		{
			Step: "under_review",
			At:   underReviewAt,
			Done: true,
		},
		{
			Step: "approved",
			At:   approvedAt,
			Done: approvedDone,
		},
		{
			Step: "refund_issued",
			At:   issuedAt,
			Done: issuedDone,
		},
	}
}

// FormatCentsDecimal formats integer cents as a 2-decimal string (e.g. 200000 -> "2000.00") without floating-point math.
func FormatCentsDecimal(cents int64) string {
	if cents < 0 {
		abs := -cents
		return fmt.Sprintf("-%d.%02d", abs/100, abs%100)
	}
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

// ParseDecimalToCents parses a decimal string (with up to 2 fractional digits) into integer cents without float rounding.
func ParseDecimalToCents(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, fmt.Errorf("empty decimal amount")
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid decimal amount %q", raw)
	}

	wholeStr := parts[0]
	if wholeStr == "" || !isAllASCIIDigits(wholeStr) {
		return 0, fmt.Errorf("invalid whole part in decimal amount %q", raw)
	}

	wholeVal, err := strconv.ParseInt(wholeStr, 10, 64)
	if err != nil || wholeVal < 0 || wholeVal > (math.MaxInt64-99)/100 {
		return 0, fmt.Errorf("decimal amount out of range %q", raw)
	}

	var fracVal int64
	if len(parts) == 2 {
		fracStr := parts[1]
		if len(fracStr) == 0 || len(fracStr) > 2 || !isAllASCIIDigits(fracStr) {
			return 0, fmt.Errorf("invalid fractional part in decimal amount %q", raw)
		}
		if len(fracStr) == 1 {
			fracStr += "0"
		}
		fracVal, err = strconv.ParseInt(fracStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid fractional part in decimal amount %q", raw)
		}
	}

	return wholeVal*100 + fracVal, nil
}

func isAllASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}
