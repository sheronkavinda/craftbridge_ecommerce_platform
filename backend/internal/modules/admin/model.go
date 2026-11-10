package admin

import "time"

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

// SellerApplication represents a seller onboarding/verification application.
type SellerApplication struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
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
