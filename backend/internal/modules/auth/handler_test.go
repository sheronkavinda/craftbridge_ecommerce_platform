package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"craftbridge/backend/internal/platform/config"
	"github.com/gin-gonic/gin"
)

func TestForgotPasswordResponseDoesNotRevealAccountOrDeliveryState(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	repo := &fakeAuthRepository{
		user: User{ID: 17, Email: "known@example.test"},
	}
	handler := NewHandler(NewService(repo, nil, &config.Config{}))

	submit := func(email string) (int, string) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(
			http.MethodPost,
			"/api/v1/auth/forgot-password",
			strings.NewReader(`{"email":"`+email+`"}`),
		)
		handler.ForgotPassword(ctx)
		return recorder.Code, recorder.Body.String()
	}

	knownStatus, knownBody := submit("known@example.test")
	unknownStatus, unknownBody := submit("unknown@example.test")
	if knownStatus != http.StatusOK || unknownStatus != http.StatusOK {
		t.Fatalf("expected generic HTTP 200 responses, got known=%d unknown=%d", knownStatus, unknownStatus)
	}
	if knownBody != unknownBody {
		t.Fatalf("forgot-password response reveals account or delivery state: known=%q unknown=%q",
			knownBody, unknownBody)
	}
}
