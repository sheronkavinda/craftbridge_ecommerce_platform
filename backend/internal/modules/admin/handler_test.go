package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	platformhttp "craftbridge/backend/internal/platform/http"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
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

	// Ensure unused import warning is not triggered for platformhttp reference
	_ = platformhttp.ContextKeyActorID
}

