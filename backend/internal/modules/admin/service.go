package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"craftbridge/backend/internal/modules/audit"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrForbiddenIDOR = errors.New("forbidden operation on target resource")
)

// Service implements business rules, input sanitization, IDOR checks, and automated audit logging.
type Service struct {
	repo         *Repository
	auditService *audit.Service
}

// NewService constructs a new admin service.
func NewService(repo *Repository, auditService *audit.Service) *Service {
	return &Service{
		repo:         repo,
		auditService: auditService,
	}
}

// ListUsers returns a paginated list of platform users.
func (s *Service) ListUsers(ctx context.Context, role, status string, limit, offset int) ([]UserSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListUsers(
		ctx,
		strings.ToUpper(sanitizeInput(role, 32)),
		strings.ToUpper(sanitizeInput(status, 32)),
		limit,
		offset,
	)
}

// UpdateUserStatus validates inputs, prevents self-lockout IDOR, updates user status, and writes an immutable audit log.
func (s *Service) UpdateUserStatus(ctx context.Context, actorID, actorName, requestID, userID string, input UpdateUserStatusInput) (*UserSummary, error) {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(userID); err != nil {
		return nil, err
	}

	// IDOR & Self-Lockout Protection: An administrator cannot suspend or ban their own account.
	if strings.EqualFold(strings.TrimSpace(actorID), strings.TrimSpace(userID)) {
		s.recordAudit(ctx, actorID, actorName, "USER_STATUS_UPDATE", "user:"+userID, "FAILURE", requestID, "blocked self-modification attempt")
		return nil, fmt.Errorf("%w: administrators cannot modify their own account status", ErrForbiddenIDOR)
	}

	normalizedStatus := strings.ToUpper(strings.TrimSpace(input.Status))
	switch normalizedStatus {
	case "ACTIVE", "SUSPENDED", "BANNED":
	default:
		return nil, fmt.Errorf("%w: status must be ACTIVE, SUSPENDED, or BANNED", ErrInvalidInput)
	}

	rawReason := truncateRunes(input.Reason, 512)
	user, err := s.repo.UpdateUserStatus(ctx, userID, normalizedStatus)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "USER_STATUS_UPDATE", "user:"+userID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	details := fmt.Sprintf("status set to %s", normalizedStatus)
	if rawReason != "" {
		details += " | reason: " + rawReason
	}
	s.recordAudit(ctx, actorID, actorName, "USER_STATUS_UPDATE", "user:"+userID, "SUCCESS", requestID, details)

	return user, nil
}

// ListSellerApplications returns a paginated list of seller onboarding applications.
func (s *Service) ListSellerApplications(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListSellerApplications(ctx, strings.ToUpper(sanitizeInput(status, 32)), limit, offset)
}

// UpdateSellerStatus triggers a seller application status transition and records an immutable audit entry.
func (s *Service) UpdateSellerStatus(ctx context.Context, actorID, actorName, requestID, sellerID string, input UpdateSellerStatusInput) (*SellerApplication, error) {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(sellerID); err != nil {
		return nil, err
	}

	normalizedStatus := strings.ToUpper(strings.TrimSpace(input.Status))
	switch normalizedStatus {
	case "PENDING", "APPROVED", "REJECTED", "SUSPENDED":
	default:
		return nil, fmt.Errorf("%w: status must be PENDING, APPROVED, REJECTED, or SUSPENDED", ErrInvalidInput)
	}

	rawNotes := truncateRunes(input.Notes, 512)
	cleanNotes := html.EscapeString(rawNotes)
	seller, err := s.repo.UpdateSellerStatus(ctx, sellerID, normalizedStatus, cleanNotes)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "SELLER_STATUS_UPDATE", "seller:"+sellerID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	details := fmt.Sprintf("seller status transitioned to %s", normalizedStatus)
	if rawNotes != "" {
		details += " | notes: " + rawNotes
	}
	s.recordAudit(ctx, actorID, actorName, "SELLER_STATUS_UPDATE", "seller:"+sellerID, "SUCCESS", requestID, details)

	return seller, nil
}

// ListRefunds returns a paginated list of refund records filtered by status.
func (s *Service) ListRefunds(ctx context.Context, status string, limit, offset int) ([]RefundRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListRefunds(ctx, strings.ToUpper(sanitizeInput(status, 32)), limit, offset)
}

// UpdateRefundStatus approves or rejects a refund request and records an immutable audit log entry.
func (s *Service) UpdateRefundStatus(ctx context.Context, actorID, actorName, requestID, refundID string, input UpdateRefundStatusInput) (*RefundRecord, error) {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(refundID); err != nil {
		return nil, err
	}

	normalizedStatus := strings.ToUpper(strings.TrimSpace(input.Status))
	switch normalizedStatus {
	case "APPROVED", "REJECTED", "PROCESSED":
	default:
		return nil, fmt.Errorf("%w: refund status must be APPROVED, REJECTED, or PROCESSED", ErrInvalidInput)
	}

	rawReason := truncateRunes(input.Reason, 512)
	cleanReason := html.EscapeString(rawReason)
	refund, err := s.repo.UpdateRefundStatus(ctx, refundID, normalizedStatus, cleanReason)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "REFUND_STATUS_UPDATE", "refund:"+refundID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	details := fmt.Sprintf("refund %s (order %s, amount %.2f)", normalizedStatus, refund.OrderID, refund.Amount)
	if rawReason != "" {
		details += " | reason: " + rawReason
	}
	s.recordAudit(ctx, actorID, actorName, "REFUND_STATUS_UPDATE", "refund:"+refundID, "SUCCESS", requestID, details)

	return refund, nil
}

// ListCategories returns all product categories.
func (s *Service) ListCategories(ctx context.Context) ([]Category, error) {
	return s.repo.ListCategories(ctx)
}

// CreateCategory sanitizes inputs, generates a unique category ID, creates the category, and records an immutable audit log entry.
func (s *Service) CreateCategory(ctx context.Context, actorID, actorName, requestID string, input CategoryInput) (*Category, error) {
	ctx = WithRequestID(ctx, requestID)
	rawName := truncateRunes(input.Name, 120)
	name := html.EscapeString(rawName)
	if name == "" {
		return nil, fmt.Errorf("%w: category name is required", ErrInvalidInput)
	}

	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(rawName)
	}
	if slug == "" {
		return nil, fmt.Errorf("%w: valid category slug is required", ErrInvalidInput)
	}

	description := sanitizeInput(input.Description, 512)
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	categoryID := generateCategoryID()
	category, err := s.repo.CreateCategory(ctx, categoryID, name, slug, description, isActive)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "CATEGORY_CREATE", "category:"+slug, "FAILURE", requestID, err.Error())
		return nil, err
	}

	s.recordAudit(ctx, actorID, actorName, "CATEGORY_CREATE", "category:"+category.ID, "SUCCESS", requestID, "created category "+rawName)
	return category, nil
}

// UpdateCategory sanitizes inputs, updates an existing category, and records an immutable audit log entry.
func (s *Service) UpdateCategory(ctx context.Context, actorID, actorName, requestID, categoryID string, input CategoryInput) (*Category, error) {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(categoryID); err != nil {
		return nil, err
	}

	rawName := truncateRunes(input.Name, 120)
	name := html.EscapeString(rawName)
	if name == "" {
		return nil, fmt.Errorf("%w: category name is required", ErrInvalidInput)
	}

	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(rawName)
	}
	if slug == "" {
		return nil, fmt.Errorf("%w: valid category slug is required", ErrInvalidInput)
	}

	description := sanitizeInput(input.Description, 512)
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	category, err := s.repo.UpdateCategory(ctx, categoryID, name, slug, description, isActive)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "CATEGORY_UPDATE", "category:"+categoryID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	s.recordAudit(ctx, actorID, actorName, "CATEGORY_UPDATE", "category:"+category.ID, "SUCCESS", requestID, "updated category "+rawName)
	return category, nil
}

// DeleteCategory validates the target ID, removes the category, and records an immutable audit log entry.
func (s *Service) DeleteCategory(ctx context.Context, actorID, actorName, requestID, categoryID string) error {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(categoryID); err != nil {
		return err
	}

	if err := s.repo.DeleteCategory(ctx, categoryID); err != nil {
		s.recordAudit(ctx, actorID, actorName, "CATEGORY_DELETE", "category:"+categoryID, "FAILURE", requestID, err.Error())
		return err
	}

	s.recordAudit(ctx, actorID, actorName, "CATEGORY_DELETE", "category:"+categoryID, "SUCCESS", requestID, "deleted category")
	return nil
}

func (s *Service) recordAudit(ctx context.Context, actorID, actorName, action, target, outcome, requestID, details string) {
	if s.auditService == nil {
		return
	}
	_, _ = s.auditService.LogAction(ctx, audit.CreateAuditLogInput{
		ActorID:   actorID,
		ActorName: actorName,
		Action:    action,
		Target:    target,
		Outcome:   outcome,
		RequestID: requestID,
		Details:   details,
	})
}

func generateCategoryID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("cat-%d", time.Now().UnixNano())
	}
	return "cat-" + hex.EncodeToString(b)
}

func validateResourceID(id string) error {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" || len(trimmed) > 64 {
		return fmt.Errorf("%w: resource id must be between 1 and 64 characters", ErrInvalidInput)
	}
	for _, r := range trimmed {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return fmt.Errorf("%w: resource id contains invalid characters", ErrInvalidInput)
		}
	}
	return nil
}

func truncateRunes(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	if maxRunes > 0 {
		runes := []rune(trimmed)
		if len(runes) > maxRunes {
			return string(runes[:maxRunes])
		}
	}
	return trimmed
}

func sanitizeInput(value string, maxRunes int) string {
	return html.EscapeString(truncateRunes(value, maxRunes))
}

func slugify(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevHyphen := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}
