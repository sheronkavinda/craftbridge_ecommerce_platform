package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"craftbridge/backend/internal/modules/audit"
	"craftbridge/backend/internal/modules/report"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrForbiddenIDOR = errors.New("forbidden operation on target resource")
)

// RepositoryStore defines the persistence contract required by the admin service.
type RepositoryStore interface {
	ListUsers(ctx context.Context, role, status, search string, limit, offset int) ([]UserSummary, error)
	UpdateUserStatus(ctx context.Context, userID, status string) (*UserSummary, error)
	UpdateUserRole(ctx context.Context, userID, role string) (*UserSummary, error)
	ListSellerApplications(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error)
	UpdateSellerStatus(ctx context.Context, sellerID, status, notes string) (*SellerApplication, error)
	ListRefunds(ctx context.Context, status string, limit, offset int) ([]RefundRecord, error)
	UpdateRefundStatus(ctx context.Context, refundID, status, reason string) (*RefundRecord, error)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateCategory(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error)
	UpdateCategory(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error)
	DeleteCategory(ctx context.Context, categoryID string) error
	GetDashboardCounts(ctx context.Context) (*DashboardCounts, error)
}

// RevenueSummaryProvider defines the report module contract used by the admin service for dashboard revenue metrics.
type RevenueSummaryProvider interface {
	GetRevenueSummary(ctx context.Context, startStr, endStr string) (*report.RevenueSummary, error)
}

// Service implements business rules, input sanitization, IDOR checks, and automated audit logging.
type Service struct {
	repo           RepositoryStore
	auditService   *audit.Service
	revenueService RevenueSummaryProvider
}

// NewService constructs a new admin service.
func NewService(repo RepositoryStore, auditService *audit.Service, revenueProviders ...RevenueSummaryProvider) *Service {
	var rev RevenueSummaryProvider
	if len(revenueProviders) > 0 && revenueProviders[0] != nil {
		rev = revenueProviders[0]
	} else if rp, ok := repo.(RevenueSummaryProvider); ok {
		rev = rp
	}
	return &Service{
		repo:           repo,
		auditService:   auditService,
		revenueService: rev,
	}
}

// ListUsers returns a paginated list of platform users filtered by role, status, and search term.
func (s *Service) ListUsers(ctx context.Context, role, status, search string, limit, offset int) ([]UserSummary, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	cleanSearch := escapeLikePattern(sanitizeInput(search, 100))
	return s.repo.ListUsers(
		ctx,
		strings.ToUpper(sanitizeInput(role, 32)),
		strings.ToUpper(sanitizeInput(status, 32)),
		cleanSearch,
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

// UpdateUserRole validates inputs, prevents self-role changes and removal of the last ADMIN, updates the user's role, and writes an immutable audit log.
// NOTE: Per the project plan, roles are owned by the identity module and this should call that module's service once it exists.
func (s *Service) UpdateUserRole(ctx context.Context, actorID, actorName, requestID, userID string, input UpdateUserRoleInput) (*UserSummary, error) {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(userID); err != nil {
		return nil, err
	}

	normalizedRole := strings.ToUpper(strings.TrimSpace(input.Role))
	switch normalizedRole {
	case "BUYER", "SELLER", "ADMIN":
	default:
		return nil, fmt.Errorf("%w: role must be BUYER, SELLER, or ADMIN", ErrInvalidInput)
	}

	// IDOR & Self-Lockout Protection: An administrator cannot change their own role.
	if strings.EqualFold(strings.TrimSpace(actorID), strings.TrimSpace(userID)) {
		s.recordAudit(ctx, actorID, actorName, "USER_ROLE_UPDATE", "user:"+userID, "FAILURE", requestID, "blocked self-role modification attempt")
		return nil, fmt.Errorf("%w: administrators cannot modify their own role", ErrForbiddenIDOR)
	}

	user, err := s.repo.UpdateUserRole(ctx, userID, normalizedRole)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "USER_ROLE_UPDATE", "user:"+userID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	details := fmt.Sprintf("role set to %s", normalizedRole)
	s.recordAudit(ctx, actorID, actorName, "USER_ROLE_UPDATE", "user:"+userID, "SUCCESS", requestID, details)

	return user, nil
}

// GetDashboardSummary aggregates users, sellers, refunds, categories, revenue summary, and the latest 5 audit log entries.
func (s *Service) GetDashboardSummary(ctx context.Context) (*DashboardSummary, error) {
	counts, err := s.repo.GetDashboardCounts(ctx)
	if err != nil {
		return nil, err
	}

	var revenueSummary report.RevenueSummary
	if s.revenueService != nil {
		rev, revErr := s.revenueService.GetRevenueSummary(ctx, "", "")
		if revErr != nil {
			return nil, revErr
		}
		if rev != nil {
			revenueSummary = *rev
		}
	}

	recentActivity := make([]audit.AuditLog, 0)
	if s.auditService != nil {
		logs, auditErr := s.auditService.ListLogs(ctx, audit.ListAuditLogsFilter{
			Limit:  5,
			Offset: 0,
		})
		if auditErr != nil {
			return nil, auditErr
		}
		if logs != nil {
			recentActivity = logs
		}
	}

	return &DashboardSummary{
		Users:          counts.Users,
		Sellers:        counts.Sellers,
		Refunds:        counts.Refunds,
		Categories:     counts.Categories,
		Revenue:        revenueSummary,
		RecentActivity: recentActivity,
		GeneratedAt:    time.Now().UTC(),
	}, nil
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

	cleanNotes := sanitizeInput(input.Notes, 512)
	seller, err := s.repo.UpdateSellerStatus(ctx, sellerID, normalizedStatus, cleanNotes)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "SELLER_STATUS_UPDATE", "seller:"+sellerID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	details := fmt.Sprintf("seller status transitioned to %s", normalizedStatus)
	if cleanNotes != "" {
		details += " | notes: " + cleanNotes
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

	cleanReason := sanitizeInput(input.Reason, 512)
	refund, err := s.repo.UpdateRefundStatus(ctx, refundID, normalizedStatus, cleanReason)
	if err != nil {
		s.recordAudit(ctx, actorID, actorName, "REFUND_STATUS_UPDATE", "refund:"+refundID, "FAILURE", requestID, err.Error())
		return nil, err
	}

	details := fmt.Sprintf("refund %s (order %s, amount %.2f)", normalizedStatus, refund.OrderID, refund.Amount)
	if cleanReason != "" {
		details += " | reason: " + cleanReason
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
	name := sanitizeInput(input.Name, 120)
	if name == "" {
		return nil, fmt.Errorf("%w: category name is required", ErrInvalidInput)
	}

	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(name)
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

	s.recordAudit(ctx, actorID, actorName, "CATEGORY_CREATE", "category:"+category.ID, "SUCCESS", requestID, "created category "+name)
	return category, nil
}

// UpdateCategory sanitizes inputs, updates an existing category, and records an immutable audit log entry.
func (s *Service) UpdateCategory(ctx context.Context, actorID, actorName, requestID, categoryID string, input CategoryInput) (*Category, error) {
	ctx = WithRequestID(ctx, requestID)
	if err := validateResourceID(categoryID); err != nil {
		return nil, err
	}

	name := sanitizeInput(input.Name, 120)
	if name == "" {
		return nil, fmt.Errorf("%w: category name is required", ErrInvalidInput)
	}

	slug := slugify(input.Slug)
	if slug == "" {
		slug = slugify(name)
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

	s.recordAudit(ctx, actorID, actorName, "CATEGORY_UPDATE", "category:"+category.ID, "SUCCESS", requestID, "updated category "+name)
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

func stripControlChars(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func truncateRunes(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(stripControlChars(value))
	if maxRunes > 0 {
		runes := []rune(trimmed)
		if len(runes) > maxRunes {
			return string(runes[:maxRunes])
		}
	}
	return trimmed
}

func sanitizeInput(value string, maxRunes int) string {
	return truncateRunes(value, maxRunes)
}

func escapeLikePattern(value string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return r.Replace(value)
}

func foldLatinRune(r rune) rune {
	switch r {
	case 'à', 'á', 'â', 'ã', 'ä', 'å', 'ā', 'ă', 'ą':
		return 'a'
	case 'ç', 'ć', 'č', 'ĉ', 'ċ':
		return 'c'
	case 'ď', 'đ':
		return 'd'
	case 'è', 'é', 'ê', 'ë', 'ē', 'ĕ', 'ė', 'ę', 'ě':
		return 'e'
	case 'ĝ', 'ğ', 'ġ', 'ģ':
		return 'g'
	case 'ĥ', 'ħ':
		return 'h'
	case 'ì', 'í', 'î', 'ï', 'ĩ', 'ī', 'ĭ', 'į', 'ı':
		return 'i'
	case 'ĵ':
		return 'j'
	case 'ķ':
		return 'k'
	case 'ĺ', 'ļ', 'ľ', 'ŀ', 'ł':
		return 'l'
	case 'ñ', 'ń', 'ņ', 'ň':
		return 'n'
	case 'ò', 'ó', 'ô', 'õ', 'ö', 'ø', 'ō', 'ŏ', 'ő':
		return 'o'
	case 'ŕ', 'ŗ', 'ř':
		return 'r'
	case 'ś', 'ŝ', 'ş', 'š':
		return 's'
	case 'ţ', 'ť', 'ŧ':
		return 't'
	case 'ù', 'ú', 'û', 'ü', 'ũ', 'ū', 'ŭ', 'ů', 'ű', 'ų':
		return 'u'
	case 'ŵ':
		return 'w'
	case 'ý', 'ÿ', 'ŷ':
		return 'y'
	case 'ź', 'ż', 'ž':
		return 'z'
	default:
		return r
	}
}

func slugify(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevHyphen := false
	for _, r := range lower {
		r = foldLatinRune(r)
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
