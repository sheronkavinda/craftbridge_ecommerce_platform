package admin

import (
	"time"

	"craftbridge/backend/internal/modules/audit"
	"craftbridge/backend/internal/modules/report"
)

// UserSummary represents a platform user record for administrative management.
type UserSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// UpdateUserStatusInput represents the request payload to update a user's account status.
type UpdateUserStatusInput struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason"`
}

// UpdateUserRoleInput represents the request payload to update a user's role.
type UpdateUserRoleInput struct {
	Role string `json:"role" binding:"required"`
}

// SellerApplication represents a seller onboarding/verification application.
type SellerApplication struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	UserName    string     `json:"user_name"`
	UserEmail   string     `json:"user_email"`
	StoreName   string     `json:"store_name"`
	Status      string     `json:"status"`
	Notes       string     `json:"notes"`
	SubmittedAt time.Time  `json:"submitted_at"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
}

// UpdateSellerStatusInput represents the request payload to approve, reject, or suspend a seller.
type UpdateSellerStatusInput struct {
	Status string `json:"status" binding:"required"`
	Notes  string `json:"notes"`
}

// RefundRecord represents a refund request managed by administrators.
type RefundRecord struct {
	ID         string     `json:"id"`
	OrderID    string     `json:"order_id"`
	Amount     float64    `json:"amount"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}

// UpdateRefundStatusInput represents the request payload to approve or reject a refund.
type UpdateRefundStatusInput struct {
	Status string `json:"status" binding:"required"`
	Reason string `json:"reason"`
}

// Category represents a product catalog category managed by administrators.
type Category struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
}

// CategoryInput represents the payload to create or update a catalog category.
type CategoryInput struct {
	Name        string `json:"name" binding:"required"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

// DashboardUserRoleCounts holds user counts broken down by role.
type DashboardUserRoleCounts struct {
	Buyer  int64 `json:"BUYER"`
	Seller int64 `json:"SELLER"`
	Admin  int64 `json:"ADMIN"`
}

// DashboardUserStats holds aggregated user counts for the admin dashboard.
type DashboardUserStats struct {
	Total     int64                   `json:"total"`
	Active    int64                   `json:"active"`
	Suspended int64                   `json:"suspended"`
	ByRole    DashboardUserRoleCounts `json:"by_role"`
}

// DashboardSellerStats holds aggregated seller application counts by status.
type DashboardSellerStats struct {
	Pending   int64 `json:"pending"`
	Approved  int64 `json:"approved"`
	Rejected  int64 `json:"rejected"`
	Suspended int64 `json:"suspended"`
}

// DashboardRefundStats holds aggregated pending refund metrics.
type DashboardRefundStats struct {
	PendingCount  int64   `json:"pending_count"`
	PendingAmount float64 `json:"pending_amount"`
}

// DashboardCategoryStats holds aggregated product category counts.
type DashboardCategoryStats struct {
	Total  int64 `json:"total"`
	Active int64 `json:"active"`
}

// DashboardCounts bundles the database aggregate counts fetched by the admin repository.
type DashboardCounts struct {
	Users      DashboardUserStats
	Sellers    DashboardSellerStats
	Refunds    DashboardRefundStats
	Categories DashboardCategoryStats
}

// DashboardSummary represents the complete response payload for GET /api/v1/admin/dashboard.
type DashboardSummary struct {
	Users          DashboardUserStats     `json:"users"`
	Sellers        DashboardSellerStats   `json:"sellers"`
	Refunds        DashboardRefundStats   `json:"refunds"`
	Categories     DashboardCategoryStats `json:"categories"`
	Revenue        report.RevenueSummary  `json:"revenue"`
	RecentActivity []audit.AuditLog       `json:"recent_activity"`
	GeneratedAt    time.Time              `json:"generated_at"`
}
