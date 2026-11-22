package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
	if err := os.Setenv("ENABLE_LOCAL_DEV_AUTH", "1"); err != nil {
		panic(err)
	}
}

func setupAdminTestRouter(repo RepositoryStore) *gin.Engine {
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	svc := NewService(repo, nil)
	h := NewHandler(svc)
	RegisterRoutes(v1, h)
	return engine
}

func addAdminDevHeaders(req *http.Request) {
	req.Header.Set("X-Actor-Role", "admin")
	req.Header.Set("X-Actor-ID", "admin-001")
	req.Header.Set("X-Actor-Name", "System Admin")
}

func TestHandler_RequireAdminRoleMiddleware(t *testing.T) {
	t.Parallel()

	router := setupAdminTestRouter(&fakeAdminRepo{})

	t.Run("missing auth headers returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("non-admin role header returns 403", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		req.Header.Set("X-Actor-Role", "buyer")
		req.Header.Set("X-Actor-ID", "buyer-001")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("malformed Bearer token returns 401", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		req.Header.Set("Authorization", "Bearer invalid.jwt.token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("valid admin dev headers return 200 with data envelope", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
		addAdminDevHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid json: %v", err)
		}
		if _, ok := body["data"]; !ok {
			t.Errorf("expected response to contain 'data' envelope, got %v", body)
		}
	})
}

func TestHandler_ErrorMappingsAndSuccessEnvelopes(t *testing.T) {
	t.Parallel()

	t.Run("ErrCategorySlugExists maps to 409 Conflict with exact message", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			createCategoryFn: func(_ context.Context, _, _, _, _ string, _ bool) (*Category, error) {
				return nil, ErrCategorySlugExists
			},
		}
		router := setupAdminTestRouter(repo)

		payload := []byte(`{"name":"Ceramics","slug":"ceramics"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusConflict)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		want := "A category with this slug already exists"
		if body["error"] != want {
			t.Errorf("got error %q, want %q", body["error"], want)
		}
	})

	t.Run("ErrRefundAlreadyDecided maps to 409 Conflict with exact message", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			updateRefundStatusFn: func(_ context.Context, _, _, _ string) (*RefundRecord, error) {
				return nil, ErrRefundAlreadyDecided
			},
		}
		router := setupAdminTestRouter(repo)

		payload := []byte(`{"status":"APPROVED"}`)
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/refunds/refund-003/status", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusConflict {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusConflict)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		want := "refund has already been decided"
		if body["error"] != want {
			t.Errorf("got error %q, want %q", body["error"], want)
		}
	})

	t.Run("ErrNotFound maps to 404 and ErrInvalidInput maps to 400", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			deleteCategoryFn: func(_ context.Context, _ string) error {
				return ErrNotFound
			},
		}
		router := setupAdminTestRouter(repo)

		// 404 test
		req404 := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/cat-missing", nil)
		addAdminDevHeaders(req404)
		rec404 := httptest.NewRecorder()
		router.ServeHTTP(rec404, req404)
		if rec404.Code != http.StatusNotFound {
			t.Fatalf("got status %d, want %d", rec404.Code, http.StatusNotFound)
		}

		// 400 test (invalid refund status)
		req400 := httptest.NewRequest(
			http.MethodPatch,
			"/api/v1/admin/refunds/refund-001/status",
			bytes.NewReader([]byte(`{"status":"UNKNOWN_STATUS"}`)),
		)
		req400.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req400)
		rec400 := httptest.NewRecorder()
		router.ServeHTTP(rec400, req400)
		if rec400.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want %d", rec400.Code, http.StatusBadRequest)
		}
	})

	t.Run("ListSellerApplications returns 200 with user_name and user_email in data array", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			listSellerApplicationsFn: func(_ context.Context, _ string, _, _ int) ([]SellerApplication, error) {
				return []SellerApplication{
					{
						ID:        "seller-002",
						UserID:    "seller-u2",
						UserName:  "Amali Weaves",
						UserEmail: "amali@craftbridge.test",
						StoreName: "Amali Handlooms",
						Status:    "PENDING",
					},
				}, nil
			},
		}
		router := setupAdminTestRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/sellers/applications", nil)
		addAdminDevHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
		}

		var resp struct {
			Data []SellerApplication `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if len(resp.Data) != 1 || resp.Data[0].UserName != "Amali Weaves" || resp.Data[0].UserEmail != "amali@craftbridge.test" {
			t.Errorf("unexpected seller application payload: %+v", resp.Data)
		}
	})

	t.Run("GET /api/v1/admin/users forwards q search parameter with escaped wildcards", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{}
		router := setupAdminTestRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?role=buyer&status=active&q=100%25_test", nil)
		addAdminDevHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		if repo.lastUserSearch != `100\%\_test` {
			t.Errorf("got lastUserSearch %q, want %q", repo.lastUserSearch, `100\%\_test`)
		}
	})

	t.Run("PATCH and PUT /api/v1/admin/users/:id/roles status codes (200, 400, 403, 404, 409)", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			updateUserRoleFn: func(_ context.Context, userID, role string) (*UserSummary, error) {
				switch userID {
				case "last-admin":
					return nil, ErrLastAdmin
				case "missing-user":
					return nil, ErrNotFound
				default:
					return &UserSummary{ID: userID, Name: "Buyer One", Email: "buyer1@craftbridge.test", Role: role, Status: "ACTIVE"}, nil
				}
			},
		}
		router := setupAdminTestRouter(repo)

		// 1. 200 OK on valid role update
		req200 := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/buyer-001/roles", bytes.NewReader([]byte(`{"role":"seller"}`)))
		req200.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req200)
		rec200 := httptest.NewRecorder()
		router.ServeHTTP(rec200, req200)
		if rec200.Code != http.StatusOK {
			t.Fatalf("got status %d, want 200", rec200.Code)
		}
		var okBody struct {
			Data UserSummary `json:"data"`
		}
		if err := json.Unmarshal(rec200.Body.Bytes(), &okBody); err != nil {
			t.Fatalf("unmarshal 200 error: %v", err)
		}
		if okBody.Data.Role != "SELLER" {
			t.Errorf("got role %q, want SELLER", okBody.Data.Role)
		}

		// 2. 400 Bad Request on invalid role
		req400 := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/buyer-001/roles", bytes.NewReader([]byte(`{"role":"MODERATOR"}`)))
		req400.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req400)
		rec400 := httptest.NewRecorder()
		router.ServeHTTP(rec400, req400)
		if rec400.Code != http.StatusBadRequest {
			t.Fatalf("got status %d, want 400", rec400.Code)
		}

		// 3. 403 Forbidden on self-role modification
		req403 := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/admin-001/roles", bytes.NewReader([]byte(`{"role":"BUYER"}`)))
		req403.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req403)
		rec403 := httptest.NewRecorder()
		router.ServeHTTP(rec403, req403)
		if rec403.Code != http.StatusForbidden {
			t.Fatalf("got status %d, want 403", rec403.Code)
		}

		// 4. 404 Not Found on missing user
		req404 := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/missing-user/roles", bytes.NewReader([]byte(`{"role":"SELLER"}`)))
		req404.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req404)
		rec404 := httptest.NewRecorder()
		router.ServeHTTP(rec404, req404)
		if rec404.Code != http.StatusNotFound {
			t.Fatalf("got status %d, want 404", rec404.Code)
		}

		// 5. 409 Conflict when demoting the last remaining ADMIN
		req409 := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/users/last-admin/roles", bytes.NewReader([]byte(`{"role":"BUYER"}`)))
		req409.Header.Set("Content-Type", "application/json")
		addAdminDevHeaders(req409)
		rec409 := httptest.NewRecorder()
		router.ServeHTTP(rec409, req409)
		if rec409.Code != http.StatusConflict {
			t.Fatalf("got status %d, want 409", rec409.Code)
		}
		var conflictBody map[string]string
		if err := json.Unmarshal(rec409.Body.Bytes(), &conflictBody); err != nil {
			t.Fatalf("unmarshal 409 error: %v", err)
		}
		if conflictBody["error"] != "cannot remove the last administrator" {
			t.Errorf("got 409 error %q, want %q", conflictBody["error"], "cannot remove the last administrator")
		}
	})

	t.Run("GET /api/v1/admin/dashboard returns 200 with data envelope", func(t *testing.T) {
		t.Parallel()
		repo := &fakeAdminRepo{
			getDashboardCountsFn: func(_ context.Context) (*DashboardCounts, error) {
				return &DashboardCounts{
					Users: DashboardUserStats{
						Total:  5,
						Active: 4,
						ByRole: DashboardUserRoleCounts{Buyer: 3, Seller: 1, Admin: 1},
					},
					Categories: DashboardCategoryStats{Total: 3, Active: 3},
				}, nil
			},
		}
		router := setupAdminTestRouter(repo)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
		addAdminDevHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		var resp struct {
			Data DashboardSummary `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal dashboard error: %v", err)
		}
		if resp.Data.Users.Total != 5 || resp.Data.Users.ByRole.Admin != 1 || resp.Data.Categories.Total != 3 {
			t.Errorf("unexpected dashboard payload: %+v", resp.Data)
		}
	})

	// Ensure unused import warning is not triggered for platformhttp reference
	_ = platformhttp.ContextKeyActorID
}
