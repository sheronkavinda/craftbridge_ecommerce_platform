package admin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"craftbridge/backend/internal/modules/audit"
	"craftbridge/backend/internal/modules/report"
)

type fakeAuditRepo struct {
	mu      sync.Mutex
	entries []audit.AuditLog
	listErr error
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
	if f.listErr != nil {
		return nil, f.listErr
	}
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

type fakeRevenueProvider struct {
	summary *report.RevenueSummary
	err     error
}

func (f *fakeRevenueProvider) GetRevenueSummary(_ context.Context, _, _ string) (*report.RevenueSummary, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.summary != nil {
		return f.summary, nil
	}
	return &report.RevenueSummary{}, nil
}

type fakeAdminRepo struct {
	lastUserRole   string
	lastUserStatus string
	lastUserSearch string
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

	listUsersFn              func(ctx context.Context, role, status, search string, limit, offset int) ([]UserSummary, error)
	updateUserStatusFn       func(ctx context.Context, userID, status string) (*UserSummary, error)
	updateUserRoleFn         func(ctx context.Context, userID, role string) (*UserSummary, error)
	listSellerApplicationsFn func(ctx context.Context, status string, limit, offset int) ([]SellerApplication, error)
	updateSellerStatusFn     func(ctx context.Context, sellerID, status, notes string) (*SellerApplication, error)
	listRefundsFn            func(ctx context.Context, status string, limit, offset int) ([]RefundRecord, error)
	updateRefundStatusFn     func(ctx context.Context, refundID, status, reason string) (*RefundRecord, error)
	listCategoriesFn         func(ctx context.Context) ([]Category, error)
	createCategoryFn         func(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error)
	updateCategoryFn         func(ctx context.Context, categoryID, name, slug, description string, isActive bool) (*Category, error)
	deleteCategoryFn         func(ctx context.Context, categoryID string) error
	getDashboardCountsFn     func(ctx context.Context) (*DashboardCounts, error)
}

func (r *fakeAdminRepo) ListUsers(ctx context.Context, role, status, search string, limit, offset int) ([]UserSummary, error) {
	r.lastUserRole = role
	r.lastUserStatus = status
	r.lastUserSearch = search
	r.lastLimit = limit
	r.lastOffset = offset
	if r.listUsersFn != nil {
		return r.listUsersFn(ctx, role, status, search, limit, offset)
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

func (r *fakeAdminRepo) UpdateUserRole(ctx context.Context, userID, role string) (*UserSummary, error) {
	r.lastUserRole = role
	if r.updateUserRoleFn != nil {
		return r.updateUserRoleFn(ctx, userID, role)
	}
	return &UserSummary{ID: userID, Name: "Test User", Email: "user@test.local", Role: role, Status: "ACTIVE"}, nil
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

func (r *fakeAdminRepo) GetDashboardCounts(ctx context.Context) (*DashboardCounts, error) {
	if r.getDashboardCountsFn != nil {
		return r.getDashboardCountsFn(ctx)
	}
	return &DashboardCounts{}, nil
}

func TestService_ListUsers_NormalizesFiltersAndBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		inRole     string
		inStatus   string
		inSearch   string
		inLimit    int
		inOffset   int
		wantRole   string
		wantStatus string
		wantSearch string
		wantLimit  int
		wantOffset int
	}{
		{
			name:       "normalizes lowercase role and status and keeps valid pagination",
			inRole:     "  buyer ",
			inStatus:   " active ",
			inSearch:   "  jane@craftbridge.test ",
			inLimit:    25,
			inOffset:   10,
			wantRole:   "BUYER",
			wantStatus: "ACTIVE",
			wantSearch: "jane@craftbridge.test",
			wantLimit:  25,
			wantOffset: 10,
		},
		{
			name:       "clamps zero limit to 50 and negative offset to 0",
			inRole:     "seller",
			inStatus:   "suspended",
			inSearch:   "",
			inLimit:    0,
			inOffset:   -5,
			wantRole:   "SELLER",
			wantStatus: "SUSPENDED",
			wantSearch: "",
			wantLimit:  50,
			wantOffset: 0,
		},
		{
			name:       "clamps excessive limit above 200 to 50",
			inRole:     "",
			inStatus:   "",
			inSearch:   "",
			inLimit:    500,
			inOffset:   0,
			wantRole:   "",
			wantStatus: "",
			wantSearch: "",
			wantLimit:  50,
			wantOffset: 0,
		},
		{
			name:       "escapes LIKE wildcards %, _, and \\ in q and truncates to 100 runes before escaping",
			inRole:     "buyer",
			inStatus:   "active",
			inSearch:   `  100% _\admin  `,
			inLimit:    20,
			inOffset:   0,
			wantRole:   "BUYER",
			wantStatus: "ACTIVE",
			wantSearch: `100\% \_\\admin`,
			wantLimit:  20,
			wantOffset: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeAdminRepo{}
			svc := NewService(repo, nil)

			_, err := svc.ListUsers(context.Background(), tc.inRole, tc.inStatus, tc.inSearch, tc.inLimit, tc.inOffset)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.lastUserRole != tc.wantRole {
				t.Errorf("got role %q, want %q", repo.lastUserRole, tc.wantRole)
			}
			if repo.lastUserStatus != tc.wantStatus {
				t.Errorf("got status %q, want %q", repo.lastUserStatus, tc.wantStatus)
			}
			if repo.lastUserSearch != tc.wantSearch {
				t.Errorf("got search %q, want %q", repo.lastUserSearch, tc.wantSearch)
			}
			if repo.lastLimit != tc.wantLimit {
				t.Errorf("got limit %d, want %d", repo.lastLimit, tc.wantLimit)
			}
			if repo.lastOffset != tc.wantOffset {
				t.Errorf("got offset %d, want %d", repo.lastOffset, tc.wantOffset)
			}
		})
	}

	t.Run("truncates q to 100 runes before escaping wildcards", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		svc := NewService(repo, nil)

		longSearch := strings.Repeat("a", 150)
		_, err := svc.ListUsers(context.Background(), "", "", longSearch, 50, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len([]rune(repo.lastUserSearch)) != 100 {
			t.Errorf("got search length %d runes, want 100", len([]rune(repo.lastUserSearch)))
		}
	})
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

func TestService_UpdateUserRole(t *testing.T) {
	t.Parallel()

	t.Run("rejects invalid role with ErrInvalidInput", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		svc := NewService(repo, nil)

		_, err := svc.UpdateUserRole(context.Background(), "admin-001", "Admin", "req-role-1", "buyer-001", UpdateUserRoleInput{
			Role: "SUPERUSER",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("got err %v, want ErrInvalidInput", err)
		}
	})

	t.Run("prevents self-role change with ErrForbiddenIDOR and records FAILURE audit", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		_, err := svc.UpdateUserRole(context.Background(), "admin-001", "Admin", "req-role-2", "ADMIN-001", UpdateUserRoleInput{
			Role: "buyer",
		})
		if !errors.Is(err, ErrForbiddenIDOR) {
			t.Fatalf("got err %v, want ErrForbiddenIDOR", err)
		}

		logs := auditRepo.recorded()
		if len(logs) != 1 || logs[0].Outcome != "FAILURE" || logs[0].Action != "USER_ROLE_UPDATE" {
			t.Fatalf("expected 1 FAILURE USER_ROLE_UPDATE audit log, got %+v", logs)
		}
	})

	t.Run("propagates ErrLastAdmin when demoting the last remaining administrator and records FAILURE audit", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			updateUserRoleFn: func(_ context.Context, _, _ string) (*UserSummary, error) {
				return nil, ErrLastAdmin
			},
		}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		_, err := svc.UpdateUserRole(context.Background(), "admin-001", "Admin", "req-role-3", "admin-002", UpdateUserRoleInput{
			Role: "buyer",
		})
		if !errors.Is(err, ErrLastAdmin) {
			t.Fatalf("got err %v, want ErrLastAdmin", err)
		}

		logs := auditRepo.recorded()
		if len(logs) != 1 || logs[0].Outcome != "FAILURE" || logs[0].Action != "USER_ROLE_UPDATE" || logs[0].Target != "user:admin-002" {
			t.Fatalf("expected 1 FAILURE USER_ROLE_UPDATE audit log, got %+v", logs)
		}
	})

	t.Run("normalizes lowercase role and writes SUCCESS USER_ROLE_UPDATE audit entry", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		auditRepo := &fakeAuditRepo{}
		svc := NewService(repo, audit.NewService(auditRepo))

		updated, err := svc.UpdateUserRole(context.Background(), "admin-001", "System Admin", "req-role-4", "buyer-001", UpdateUserRoleInput{
			Role: " seller ",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Role != "SELLER" || repo.lastUserRole != "SELLER" {
			t.Errorf("got role %q, want SELLER", updated.Role)
		}

		logs := auditRepo.recorded()
		if len(logs) != 1 || logs[0].Outcome != "SUCCESS" || logs[0].Action != "USER_ROLE_UPDATE" || logs[0].Details != "role set to SELLER" {
			t.Errorf("unexpected audit entry: %+v", logs)
		}
	})
}

func TestService_GetDashboardSummary(t *testing.T) {
	t.Parallel()

	t.Run("assembles dashboard pieces from repository, revenue provider, and audit service", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			getDashboardCountsFn: func(_ context.Context) (*DashboardCounts, error) {
				return &DashboardCounts{
					Users: DashboardUserStats{
						Total:     12,
						Active:    10,
						Suspended: 2,
						ByRole: DashboardUserRoleCounts{
							Buyer:  8,
							Seller: 3,
							Admin:  1,
						},
					},
					Sellers: DashboardSellerStats{
						Pending:   2,
						Approved:  5,
						Rejected:  1,
						Suspended: 0,
					},
					Refunds: DashboardRefundStats{
						PendingCount:  2,
						PendingAmount: 6350.50,
					},
					Categories: DashboardCategoryStats{
						Total:  6,
						Active: 5,
					},
				}, nil
			},
		}
		auditRepo := &fakeAuditRepo{
			entries: []audit.AuditLog{
				{ID: 10, ActorID: "admin-001", ActorName: "System Admin", Action: "USER_ROLE_UPDATE", Target: "user:usr-1", Outcome: "SUCCESS"},
				{ID: 9, ActorID: "admin-001", ActorName: "System Admin", Action: "CATEGORY_CREATE", Target: "category:cat-1", Outcome: "SUCCESS"},
			},
		}
		revProvider := &fakeRevenueProvider{
			summary: &report.RevenueSummary{
				GrossVerifiedPayments: 45000.00,
				TotalRefunds:          2500.00,
				NetRevenue:            42500.00,
				VerifiedPaymentCount:  18,
				RefundCount:           2,
			},
		}

		svc := NewService(repo, audit.NewService(auditRepo), revProvider)
		summary, err := svc.GetDashboardSummary(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary.Users.Total != 12 || summary.Users.ByRole.Buyer != 8 || summary.Users.ByRole.Seller != 3 || summary.Users.ByRole.Admin != 1 {
			t.Errorf("unexpected user stats: %+v", summary.Users)
		}
		if summary.Sellers.Pending != 2 || summary.Sellers.Approved != 5 {
			t.Errorf("unexpected seller stats: %+v", summary.Sellers)
		}
		if summary.Refunds.PendingCount != 2 || summary.Refunds.PendingAmount != 6350.50 {
			t.Errorf("unexpected refund stats: %+v", summary.Refunds)
		}
		if summary.Categories.Total != 6 || summary.Categories.Active != 5 {
			t.Errorf("unexpected category stats: %+v", summary.Categories)
		}
		if summary.Revenue.NetRevenue != 42500.00 || summary.Revenue.GrossVerifiedPayments != 45000.00 {
			t.Errorf("unexpected revenue summary: %+v", summary.Revenue)
		}
		if len(summary.RecentActivity) != 2 || summary.RecentActivity[0].ID != 10 {
			t.Errorf("unexpected recent activity: %+v", summary.RecentActivity)
		}
		if summary.GeneratedAt.IsZero() {
			t.Error("expected non-zero generated_at")
		}
	})

	t.Run("returns error when any dashboard dependency fails", func(t *testing.T) {
		t.Parallel()

		// 1. Repository aggregate query fails
		repoErr := errors.New("db aggregate failure")
		svc1 := NewService(&fakeAdminRepo{
			getDashboardCountsFn: func(_ context.Context) (*DashboardCounts, error) {
				return nil, repoErr
			},
		}, nil, &fakeRevenueProvider{})
		if _, err := svc1.GetDashboardSummary(context.Background()); !errors.Is(err, repoErr) {
			t.Errorf("got err %v, want %v", err, repoErr)
		}

		// 2. Revenue provider fails
		revErr := errors.New("revenue query failure")
		svc2 := NewService(&fakeAdminRepo{}, nil, &fakeRevenueProvider{err: revErr})
		if _, err := svc2.GetDashboardSummary(context.Background()); !errors.Is(err, revErr) {
			t.Errorf("got err %v, want %v", err, revErr)
		}

		// 3. Audit service fails
		auditErr := errors.New("audit query failure")
		svc3 := NewService(&fakeAdminRepo{}, audit.NewService(&fakeAuditRepo{listErr: auditErr}), &fakeRevenueProvider{})
		if _, err := svc3.GetDashboardSummary(context.Background()); !errors.Is(err, auditErr) {
			t.Errorf("got err %v, want %v", err, auditErr)
		}
	})
}

