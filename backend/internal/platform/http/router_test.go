package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"craftbridge/backend/internal/platform/config"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCORSMiddleware_AllowsIdempotencyKeyHeader(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		AppEnv:      "development",
		CORSOrigins: []string{"http://localhost:4200"},
	}
	router := NewRouter(cfg, nil)

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/orders/order-005/refund-requests", nil)
	req.Header.Set("Origin", "http://localhost:4200")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Idempotency-Key")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 204, got %d", rec.Code)
	}

	allowOrigin := rec.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin != "http://localhost:4200" {
		t.Errorf("got Access-Control-Allow-Origin %q, want http://localhost:4200", allowOrigin)
	}

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "Idempotency-Key") {
		t.Errorf("expected Access-Control-Allow-Headers to include Idempotency-Key, got %q", allowHeaders)
	}
}
