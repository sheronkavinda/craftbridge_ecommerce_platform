
package seller

import "time"

// SellerApplication represents a request
// to become an approved CraftBridge seller.
type SellerApplication struct {
    ID             int64      `json:"id"`
    UserID         int64      `json:"user_id"`
    StoreName      string     `json:"store_name"`
    CountryCode    string     `json:"country_code"`
    Description    string     `json:"description"`
    Status         string     `json:"status"`
    DecisionReason *string    `json:"decision_reason"`
    ReviewedBy     *int64     `json:"reviewed_by"`
    ReviewedAt     *time.Time `json:"reviewed_at"`
    CreatedAt      time.Time  `json:"created_at"`
    UpdatedAt      time.Time  `json:"updated_at"`
}

// CreateSellerApplicationRequest contains
// data submitted by a customer.
type CreateSellerApplicationRequest struct {
    StoreName   string `json:"store_name" binding:"required"`
    CountryCode string `json:"country_code" binding:"required,len=2"`
    Description string `json:"description"`
}

// Shop represents an approved seller's shop.
type Shop struct {
    ID              int64     `json:"id"`
    OwnerUserID     int64     `json:"owner_user_id"`
    ApplicationID   int64     `json:"application_id"`
    Name            string    `json:"name"`
    Slug            string    `json:"slug"`
    Description     string    `json:"description"`
    CountryCode     string    `json:"country_code"`
    ContactEmail    *string   `json:"contact_email"`
    ProfileImageURL *string   `json:"profile_image_url"`
    BannerImageURL  *string   `json:"banner_image_url"`
    Status          string    `json:"status"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}

// UpdateShopRequest is used for
// updating shop information.
type UpdateShopRequest struct {
    Name         string `json:"name" binding:"required"`
    Description  string `json:"description"`
    ContactEmail string `json:"contact_email"`
}
