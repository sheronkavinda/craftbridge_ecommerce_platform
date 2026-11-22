package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
	if err := os.Setenv("ENABLE_LOCAL_DEV_AUTH", "1"); err != nil {
		panic(err)
	}
}

type fakeAuditStore struct {
	lastFilter ListAuditLogsFilter
	lastInput  CreateAuditLogInput
}

func (f *fakeAuditStore) Create(_ context.Context, input CreateAuditLogInput) (*AuditLog, error) {
	f.lastInput = input
	return &AuditLog{
		ID:        1,
		ActorID:   input.ActorID,
		ActorName: input.ActorName,
		Action:    input.Action,
		Target:    input.Target,
		Outcome:   input.Outcome,
		RequestID: input.RequestID,
		Details:   input.Details,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (f *fakeAuditStore) List(_ context.Context, filter ListAuditLogsFilter) ([]AuditLog, error) {
	f.lastFilter = filter
	return []AuditLog{
		{
			ID:        1,
			ActorID:   "admin-001",
			ActorName: "System Admin",
			Action:    "USER_STATUS_UPDATE",
			Target:    "user:buyer-001",
			Outcome:   "SUCCESS",
			RequestID: "req-1",
			Details:   "status set to SUSPENDED",
			CreatedAt: time.Now().UTC(),
		},
	}, nil
}

func setupAuditTestRouter(store RepositoryStore) *gin.Engine {
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	svc := NewService(store)
	h := NewHandler(svc)
	RegisterRoutes(v1, h)
	return engine
}

func addAuditAdminHeaders(req *http.Request) {
	req.Header.Set("X-Actor-Role", "admin")
	req.Header.Set("X-Actor-ID", "admin-001")
	req.Header.Set("X-Actor-Name", "System Admin")
}

func TestService_ListAndLogAction(t *testing.T) {
	t.Parallel()

	t.Run("ListLogs normalizes action and outcome to uppercase", func(t *testing.T) {
		t.Parallel()
		store := &fakeAuditStore{}
		svc := NewService(store)

		_, err := svc.ListLogs(context.Background(), ListAuditLogsFilter{
			ActorID: " admin-001 ",
			Action:  " user_status_update ",
			Outcome: " failure ",
			Limit:   25,
			Offset:  0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.lastFilter.ActorID != "admin-001" {
			t.Errorf("got ActorID %q, want admin-001", store.lastFilter.ActorID)
		}
		if store.lastFilter.Action != "USER_STATUS_UPDATE" {
			t.Errorf("got Action %q, want USER_STATUS_UPDATE", store.lastFilter.Action)
		}
		if store.lastFilter.Outcome != "FAILURE" {
			t.Errorf("got Outcome %q, want FAILURE", store.lastFilter.Outcome)
		}
	})

	t.Run("LogAction requires action and target and defaults invalid outcome to SUCCESS", func(t *testing.T) {
		t.Parallel()
		store := &fakeAuditStore{}
		svc := NewService(store)

		_, err := svc.LogAction(context.Background(), CreateAuditLogInput{
			Action: "",
			Target: "user:buyer-001",
		})
		if !errors.Is(err, ErrInvalidAuditInput) {
			t.Fatalf("missing action: got err %v, want ErrInvalidAuditInput", err)
		}

		_, err = svc.LogAction(context.Background(), CreateAuditLogInput{
			Action: "MANUAL_NOTE",
			Target: "   ",
		})
		if !errors.Is(err, ErrInvalidAuditInput) {
			t.Fatalf("missing target: got err %v, want ErrInvalidAuditInput", err)
		}

		entry, err := svc.LogAction(context.Background(), CreateAuditLogInput{
			ActorName: "Admin & Ops <Team>",
			Action:    " manual_note ",
			Target:    "order:ord-001",
			Outcome:   "unknown",
			Details:   "Reviewed Arts & Crafts <batch>",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry.Action != "MANUAL_NOTE" || entry.Outcome != "SUCCESS" || entry.ActorID != "system" {
			t.Errorf("unexpected created entry: %+v", entry)
		}
		if entry.ActorName != "Admin & Ops <Team>" || entry.Details != "Reviewed Arts & Crafts <batch>" {
			t.Errorf("expected unescaped ActorName and Details, got ActorName=%q Details=%q", entry.ActorName, entry.Details)
		}
	})
}

func TestHandler_ListAndCreateAuditLogs(t *testing.T) {
	t.Parallel()

	t.Run("GET /api/v1/admin/audit-logs clamps limit bounds (1..200 default 50) and negative offset", func(t *testing.T) {
		t.Parallel()
		store := &fakeAuditStore{}
		router := setupAuditTestRouter(store)

		req := httptest.NewRequest(
			http.MethodGet,
			"/api/v1/admin/audit-logs?action=category_create&outcome=success&limit=999&offset=-10",
			nil,
		)
		addAuditAdminHeaders(req)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("got status %d, want 200", rec.Code)
		}
		if store.lastFilter.Limit != 50 || store.lastFilter.Offset != 0 {
			t.Errorf("got limit=%d offset=%d, want limit=50 offset=0", store.lastFilter.Limit, store.lastFilter.Offset)
		}
		if store.lastFilter.Action != "CATEGORY_CREATE" || store.lastFilter.Outcome != "SUCCESS" {
			t.Errorf("unexpected normalized filter: %+v", store.lastFilter)
		}
	})

	t.Run("POST /api/v1/audit/logs validates required fields and binds verified actor context", func(t *testing.T) {
		t.Parallel()
		store := &fakeAuditStore{}
		router := setupAuditTestRouter(store)

		// Missing required target field -> 400
		badReq := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/audit/logs",
			bytes.NewReader([]byte(`{"action":"MANUAL_CHECK"}`)),
		)
		badReq.Header.Set("Content-Type", "application/json")
		addAuditAdminHeaders(badReq)
		badRec := httptest.NewRecorder()
		router.ServeHTTP(badRec, badReq)
		if badRec.Code != http.StatusBadRequest {
			t.Fatalf("missing target: got status %d, want 400", badRec.Code)
		}

		// Valid payload -> 201 Created, spoofing actor_id in body is overridden by verified admin context
		goodReq := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/audit/logs",
			bytes.NewReader([]byte(`{"actor_id":"spoofed-actor","action":"manual_check","target":"order:ord-001","outcome":"failure"}`)),
		)
		goodReq.Header.Set("Content-Type", "application/json")
		addAuditAdminHeaders(goodReq)
		goodRec := httptest.NewRecorder()
		router.ServeHTTP(goodRec, goodReq)

		if goodRec.Code != http.StatusCreated {
			t.Fatalf("valid post: got status %d, want 201", goodRec.Code)
		}

		var resp struct {
			Data AuditLog `json:"data"`
		}
		if err := json.Unmarshal(goodRec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		if resp.Data.ActorID != "admin-001" || resp.Data.Action != "MANUAL_CHECK" || resp.Data.Outcome != "FAILURE" {
			t.Errorf("unexpected created audit log: %+v", resp.Data)
		}
	})
}
