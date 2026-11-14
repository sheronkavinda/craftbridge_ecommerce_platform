package admin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"craftbridge/backend/internal/modules/audit"
)

type fakeAuditRepo struct {
	mu      sync.Mutex
	entries []audit.AuditLog
}

func (f *fakeAuditRepo) Create(_ context.Context, input audit.CreateAuditLogInput) (*audit.AuditLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entry := audit.AuditLog{
		ID:        int64(len(f.entries) + 1),
		ActorID:   input.ActorID,
		ActorName: input.ActorName,
		Action:    input.Action,
		Target:    input.Target,
		Outcome:   input.Outcome,
		RequestID: input.RequestID,
		Details:   input.Details,
		CreatedAt: time.Now().UTC(),
	}
	f.entries = append(f.entries, entry)
	return &entry, nil
}

func (f *fakeAuditRepo) List(_ context.Context, _ audit.ListAuditLogsFilter) ([]audit.AuditLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]audit.AuditLog, len(f.entries))
	copy(out, f.entries)
	return out, nil
}

func (f *fakeAuditRepo) recorded() []audit.AuditLog {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]audit.AuditLog, len(f.entries))
	copy(out, f.entries)
	return out
}

type fakeAdminRepo struct {
	lastUserRole   string
	lastUserStatus string
	lastLimit      int
	lastOffset     int

	lastSellerID     string
	lastSellerStatus string
	lastSellerNotes  string

	lastRefundID     string
	lastRefundStatus string
	lastRefundReason string

	lastCategoryID          string
	lastCategoryName        string
	lastCategorySlug        string
	lastCategoryDescription string
	lastCategoryIsActive    bool

	listUsersFn              func(ctx context.Context, role, status string, limit, offset int) ([]UserSummary, error)
	updateUserStatusFn       func(ctx context.Context, userID, status string) (*UserSummary, error)
	listSellerApplicationsFn func(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error)
	updateSellerStatusFn     func(ctx context.Context, sellerID, status, notes string) (*SellerApplication, error)
	listRefundsFn            func(ctx context.Context, status string, limit, offset int) ([]RefundRecord, error)
	updateRefundStatusFn     func(ctx context.Context, refundID, status, reason string) (*RefundRecord, error)
	listCategoriesFn         func(ctx context.Context) ([]Category, error)
	createCategoryFn         func(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error)
	updateCategoryFn         func(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error)
	deleteCategoryFn         func(ctx context.Context, categoryID string) error
}

func (r *fakeAdminRepo) ListUsers(ctx context.Context, role, status string, limit, offset int) ([]UserSummary, error) {
	r.lastUserRole = role
	r.lastUserStatus = status
	r.lastLimit = limit
	r.lastOffset = offset
	if r.listUsersFn != nil {
		return r.listUsersFn(ctx, role, status, limit, offset)
	}
	return []UserSummary{}, nil
}

func (r *fakeAdminRepo) UpdateUserStatus(ctx context.Context, userID, status string) (*UserSummary, error) {
	r.lastUserStatus = status
	if r.updateUserStatusFn != nil {
		return r.updateUserStatusFn(ctx, userID, status)
	}
	return &UserSummary{ID: userID, Name: "Test User", Email: "user@test.local", Role: "BUYER", Status: status}, nil
}

func (r *fakeAdminRepo) ListSellerApplications(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error) {
	r.lastSellerStatus = status
	r.lastLimit = limit
	r.lastOffset = offset
	if r.listSellerApplicationsFn != nil {
		return r.listSellerApplicationsFn(ctx, status, limit, offset)
	}
	return []SellerApplication{}, nil
}

func (r *fakeAdminRepo) UpdateSellerStatus(ctx context.Context, sellerID, status, notes string) (*SellerApplication, error) {
	r.lastSellerID = sellerID
	r.lastSellerStatus = status
	r.lastSellerNotes = notes
	if r.updateSellerStatusFn != nil {
		return r.updateSellerStatusFn(ctx, sellerID, status, notes)
	}
	return &SellerApplication{ID: sellerID, UserID: "usr-1", StoreName: "Craft Store", Status: status, Notes: notes}, nil
}

func (r *fakeAdminRepo) ListRefunds(ctx context.Context, status string, limit, offset int) ([]RefundRecord, error) {
	r.lastRefundStatus = status
	r.lastLimit = limit
	r.lastOffset = offset
	if r.listRefundsFn != nil {
		return r.listRefundsFn(ctx, status, limit, offset)
	}
	return []RefundRecord{}, nil
}

func (r *fakeAdminRepo) UpdateRefundStatus(ctx context.Context, refundID, status, reason string) (*RefundRecord, error) {
	r.lastRefundID = refundID
	r.lastRefundStatus = status
	r.lastRefundReason = reason
	if r.updateRefundStatusFn != nil {
		return r.updateRefundStatusFn(ctx, refundID, status, reason)
	}
	return &RefundRecord{ID: refundID, OrderID: "order-001", Amount: 2500, Status: status, Reason: reason}, nil
}

func (r *fakeAdminRepo) ListCategories(ctx context.Context) ([]Category, error) {
	if r.listCategoriesFn != nil {
		return r.listCategoriesFn(ctx)
	}
	return []Category{}, nil
}

func (r *fakeAdminRepo) CreateCategory(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error) {
	r.lastCategoryID = categoryID
	r.lastCategoryName = name
	r.lastCategorySlug = slug
	r.lastCategoryDescription = description
	r.lastCategoryIsActive = isActive
	if r.createCategoryFn != nil {
		return r.createCategoryFn(ctx, categoryID, name, slug, description, isActive)
	}
	return &Category{ID: categoryID, Name: name, Slug: slug, Description: description, IsActive: isActive}, nil
}

func (r *fakeAdminRepo) UpdateCategory(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error) {
	r.lastCategoryID = categoryID
	r.lastCategoryName = name
	r.lastCategorySlug = slug
	r.lastCategoryDescription = description
	r.lastCategoryIsActive = isActive
	if r.updateCategoryFn != nil {
		return r.updateCategoryFn(ctx, categoryID, name, slug, description, isActive)
	}
	return &Category{ID: categoryID, Name: name, Slug: slug, Description: description, IsActive: isActive}, nil
}

func (r *fakeAdminRepo) DeleteCategory(ctx context.Context, categoryID string) error {
	r.lastCategoryID = categoryID
	if r.deleteCategoryFn != nil {
		return r.deleteCategoryFn(ctx, categoryID)
	}
	return nil
}

func TestService_ListUsers_NormalizesFiltersAndBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		inRole     string
		inStatus   string
		inLimit    int
		inOffset   int
		wantRole   string
		wantStatus string
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "normalizes lowercase role and status and keeps valid pagination",
			inRole:     "  buyer ",
			inStatus:   " active ",
			inLimit:    25,
			inOffset:   10,
			wantRole:   "BUYER",
			wantStatus: "ACTIVE",
			wantLimit:  25,
			wantOffset: 10,
		},
		{
			name:       "clamps zero limit to 50 and negative offset to 0",
			inRole:     "seller",
			inStatus:   "suspended",
			inLimit:    0,
			inOffset:   -5,
			wantRole:   "SELLER",
			wantStatus: "SUSPENDED",
			wantLimit:  50,
			wantOffset: 0,
		},
		{
			name:       "clamps excessive limit above 200 to 50",
			inRole:     "",
			inStatus:   "",
			inLimit:    500,
			inOffset:   0,
			wantRole:   "",
			wantStatus: "",
			wantLimit:  50,
			wantOffset: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeAdminRepo{}
			svc := NewService(repo, nil)

			_, err := svc.ListUsers(context.Background(), tc.inRole, tc.inStatus, tc.inLimit, tc.inOffset)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.lastUserRole != tc.wantRole {
				t.Errorf("got role %q, want %q", repo.lastUserRole, tc.wantRole)
			}
			if repo.lastUserStatus != tc.wantStatus {
				t.Errorf("got status %q, want %q", repo.lastUserStatus, tc.wantStatus)
			}
			if repo.lastLimit != tc.wantLimit {
				t.Errorf("got limit %d, want %d", repo.lastLimit, tc.wantLimit)
			}
			if repo.lastOffset != tc.wantOffset {
				t.Errorf("got offset %d, want %d", repo.lastOffset, tc.wantOffset)
			}
		})
	}
}

func TestService_UpdateUserStatus(t *testing.T) {
	t.Parallel()

	t.Run("rejects invalid status with ErrInvalidInput", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		_, err := svc.UpdateUserStatus(context.Background(), "admin-001", "Admin", "req-1", "buyer-001", UpdateUserStatusInput{
			Status: "DELETED",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("got err %v, want ErrInvalidInput", err)
		}
	})

	t.Run("prevents self-modification with ErrForbiddenIDOR and records FAILURE audit", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		_, err := svc.UpdateUserStatus(context.Background(), "admin-001", "Admin", "req-2", "ADMIN-001", UpdateUserStatusInput{
			Status: "SUSPENDED",
		})
		if !errors.Is(err, ErrForbiddenIDOR) {
			t.Fatalf("got err %v, want ErrForbiddenIDOR", err)
		}

		logs := auditRepo.recorded()
		if len(logs) != 1 || logs[0].Outcome != "FAILURE" || logs[0].Action != "USER_STATUS_UPDATE" {
			t.Fatalf("expected 1 FAILURE USER_STATUS_UPDATE audit log, got %+v", logs)
		}
	})

	t.Run("updates status and records SUCCESS audit entry", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		updated, err := svc.UpdateUserStatus(context.Background(), "admin-001", "System Admin", "req-3", "buyer-002", UpdateUserStatusInput{
			Status: " suspended ",
			Reason: "Policy review",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Status != "SUSPENDED" {
			t.Errorf("got status %q, want SUSPENDED", updated.Status)
		}

		logs := auditRepo.recorded()
		if len(logs) != 1 {
			t.Fatalf("expected 1 audit entry, got %d", len(logs))
		}
		if logs[0].Outcome != "SUCCESS" || logs[0].Target != "user:buyer-002" {
			t.Errorf("unexpected audit entry: %+v", logs[0])
		}
		if !strings.Contains(logs[0].Details, "Policy review") {
			t.Errorf("expected details to contain reason, got %q", logs[0].Details)
		}
	})
}

func TestService_UpdateSellerStatus(t *testing.T) {
	t.Parallel()

	t.Run("rejects invalid seller status", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		svc := NewService(repo, nil)

		_, err := svc.UpdateSellerStatus(context.Background(), "admin-001", "Admin", "req-1", "seller-001", UpdateSellerStatusInput{
			Status: "ARCHIVED",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("got err %v, want ErrInvalidInput", err)
		}
	})

	t.Run("truncates notes to 512 runes and records audit", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		longNotes := strings.Repeat("a", 600)
		_, err := svc.UpdateSellerStatus(context.Background(), "admin-001", "Admin", "req-2", "seller-001", UpdateSellerStatusInput{
			Status: " approved ",
			Notes:  longNotes,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len([]rune(repo.lastSellerNotes)) != 512 {
			t.Errorf("got notes length %d, want 512", len([]rune(repo.lastSellerNotes)))
		}
		if repo.lastSellerStatus != "APPROVED" {
			t.Errorf("got status %q, want APPROVED", repo.lastSellerStatus)
		}
	})
}

func TestService_UpdateRefundStatus(t *testing.T) {
	t.Parallel()

	t.Run("accepts APPROVED, REJECTED, PROCESSED case-insensitively and rejects others", func(t *testing.T) {
		t.Parallel()
		for _, valid := range []string{" approved ", "rejected", "Processed"} {
			repo := &fakeAdminRepo{}
			svc := NewService(repo, nil)
			res, err := svc.UpdateRefundStatus(context.Background(), "admin-001", "Admin", "req-ok", "refund-001", UpdateRefundStatusInput{
				Status: valid,
			})
			if err != nil {
				t.Fatalf("status %q returned unexpected err: %v", valid, err)
			}
			want := strings.ToUpper(strings.TrimSpace(valid))
			if res.Status != want {
				t.Errorf("got status %q, want %q", res.Status, want)
			}
		}

		repo := &fakeAdminRepo{}
		svc := NewService(repo, nil)
		_, err := svc.UpdateRefundStatus(context.Background(), "admin-001", "Admin", "req-bad", "refund-001", UpdateRefundStatusInput{
			Status: "PENDING",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("got err %v, want ErrInvalidInput", err)
		}
	})

	t.Run("propagates ErrRefundAlreadyDecided and records FAILURE audit entry", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			updateRefundStatusFn: func(_ context.Context, _, _, _ string) (*RefundRecord, error) {
				return nil, ErrRefundAlreadyDecided
			},
		}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		_, err := svc.UpdateRefundStatus(context.Background(), "admin-001", "System Admin", "req-decided", "refund-003", UpdateRefundStatusInput{
			Status: "REJECTED",
			Reason: "Duplicate claim",
		})
		if !errors.Is(err, ErrRefundAlreadyDecided) {
			t.Fatalf("got err %v, want ErrRefundAlreadyDecided", err)
		}

		logs := auditRepo.recorded()
		if len(logs) != 1 {
			t.Fatalf("expected 1 audit entry, got %d", len(logs))
		}
		if logs[0].Outcome != "FAILURE" || logs[0].Action != "REFUND_STATUS_UPDATE" || logs[0].Target != "refund:refund-003" {
			t.Errorf("unexpected audit entry: %+v", logs[0])
		}
	})
}

func TestService_CategoryCreateAndUpdate(t *testing.T) {
	t.Parallel()

	t.Run("stores Arts & Crafts verbatim without HTML escaping and generates arts-crafts slug", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		cat, err := svc.CreateCategory(context.Background(), "admin-001", "Admin & Co", "req-arts", CategoryInput{
			Name:        "Arts & Crafts",
			Slug:        "",
			Description: "Fine <handmade> items & gifts",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cat.Name != "Arts & Crafts" || repo.lastCategoryName != "Arts & Crafts" {
			t.Errorf("got category name %q, want %q", cat.Name, "Arts & Crafts")
		}
		if cat.Slug != "arts-crafts" || repo.lastCategorySlug != "arts-crafts" {
			t.Errorf("got category slug %q, want %q", cat.Slug, "arts-crafts")
		}
		if cat.Description != "Fine <handmade> items & gifts" {
			t.Errorf("got description %q, want unescaped description", cat.Description)
		}
		logs := auditRepo.recorded()
		if len(logs) != 1 || logs[0].Details != "created category Arts & Crafts" || logs[0].ActorName != "Admin & Co" {
			t.Errorf("unexpected audit entry: %+v", logs)
		}
	})

	t.Run("stores name containing <script> as typed while keeping slug safe", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		svc := NewService(repo, nil)

		rawName := "<script>alert('xss')</script> Masks & Wood"
		cat, err := svc.CreateCategory(context.Background(), "admin-001", "Admin", "req-script", CategoryInput{
			Name: rawName,
			Slug: "",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cat.Name != rawName {
			t.Errorf("got stored name %q, want exact %q", cat.Name, rawName)
		}
		if cat.Slug != "script-alert-xss-script-masks-wood" {
			t.Errorf("got slug %q, want %q", cat.Slug, "script-alert-xss-script-masks-wood")
		}
		if strings.ContainsAny(cat.Slug, "<>'\"& ") {
			t.Errorf("slug contains unsafe characters: %q", cat.Slug)
		}
	})

	t.Run("120-rune name containing &, <, > is truncated to at most 120 runes", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		svc := NewService(repo, nil)

		longName := "Handcrafted & <Organic> > Sri Lankan Woodwork Collection " + strings.Repeat("&<>X", 30)
		cat, err := svc.CreateCategory(context.Background(), "admin-001", "Admin", "req-cat-1", CategoryInput{
			Name: longName,
			Slug: "",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotRunes := len([]rune(repo.lastCategoryName)); gotRunes != 120 {
			t.Errorf("got name length %d runes, want exactly 120 runes", gotRunes)
		}
		if strings.Contains(cat.Name, "&amp;") || strings.Contains(cat.Name, "&lt;") || strings.Contains(cat.Name, "&gt;") {
			t.Errorf("expected unescaped name, got %q", cat.Name)
		}
	})

	t.Run("slug generation handles &, accents, and multiple spaces", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			inName   string
			wantSlug string
		}{
			{inName: "Arts & Crafts", wantSlug: "arts-crafts"},
			{inName: "  Café   &   Résumé  Décor  ", wantSlug: "cafe-resume-decor"},
			{inName: "Papier-Mâché   &   Céramique   Artisanale", wantSlug: "papier-mache-ceramique-artisanale"},
			{inName: " Tea,   Spices   &   Herbs ", wantSlug: "tea-spices-herbs"},
		}

		for _, tc := range cases {
			repo := &fakeAdminRepo{}
			svc := NewService(repo, nil)
			cat, err := svc.CreateCategory(context.Background(), "admin-001", "Admin", "req-slug", CategoryInput{
				Name: tc.inName,
			})
			if err != nil {
				t.Fatalf("CreateCategory(%q) unexpected error: %v", tc.inName, err)
			}
			if cat.Slug != tc.wantSlug {
				t.Errorf("CreateCategory(%q) slug = %q, want %q", tc.inName, cat.Slug, tc.wantSlug)
			}
		}
	})

	t.Run("propagates ErrCategorySlugExists on create and update", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			createCategoryFn: func(_ context.Context, _, _, _, _ string, _ bool) (*Category, error) {
				return nil, ErrCategorySlugExists
			},
			updateCategoryFn: func(_ context.Context, _, _, _, _ string, _ bool) (*Category, error) {
				return nil, ErrCategorySlugExists
			},
		}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		_, err := svc.CreateCategory(context.Background(), "admin-001", "Admin", "req-dup-1", CategoryInput{
			Name: "Ceramics",
			Slug: "ceramics",
		})
		if !errors.Is(err, ErrCategorySlugExists) {
			t.Fatalf("CreateCategory got err %v, want ErrCategorySlugExists", err)
		}

		_, err = svc.UpdateCategory(context.Background(), "admin-001", "Admin", "req-dup-2", "cat-01", CategoryInput{
			Name: "Ceramics",
			Slug: "ceramics",
		})
		if !errors.Is(err, ErrCategorySlugExists) {
			t.Fatalf("UpdateCategory got err %v, want ErrCategorySlugExists", err)
		}

		logs := auditRepo.recorded()
		if len(logs) != 2 || logs[0].Outcome != "FAILURE" || logs[1].Outcome != "FAILURE" {
			t.Errorf("expected 2 FAILURE audit entries, got %+v", logs)
		}
	})
}

