package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"craftbridge/backend/internal/platform/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

type capturedResetEmail struct {
	recipient string
	token     string
	err       error
}

func (s *capturedResetEmail) SendPasswordReset(_ context.Context, recipient, token string) error {
	s.recipient = recipient
	s.token = token
	return s.err
}

func TestPostgresPasswordResetDeliveryAndTokenConsumption(t *testing.T) {
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("set RUN_POSTGRES_INTEGRATION=1 to run against PostgreSQL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := config.Load()
	cfg.JWTSecret = "member-01-postgres-integration-test-secret"
	pool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	email := fmt.Sprintf("member01-reset-%d@example.test", time.Now().UnixNano())
	repo := NewRepository(pool)
	service := NewService(repo, pool, cfg)
	sender := &capturedResetEmail{}
	service.SetPasswordResetEmailSender(sender)
	registered, err := service.Register(ctx, RegisterInput{
		Name:     "Member 01 Integration",
		Email:    email,
		Password: "initial-password-2026",
	})
	if err != nil {
		t.Fatalf("register integration user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM users WHERE id = $1`, registered.User.ID); err != nil {
			t.Errorf("clean up integration user: %v", err)
		}
	})

	sender.err = errors.New("SMTP unavailable")
	if err := service.ForgotPassword(ctx, ForgotPasswordInput{Email: email}); !errors.Is(err, ErrPasswordResetDeliveryFailed) {
		t.Fatalf("expected SMTP delivery error, got %v", err)
	}
	var unusedTokens int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL
	`, registered.User.ID).Scan(&unusedTokens); err != nil {
		t.Fatalf("count undelivered tokens: %v", err)
	}
	if unusedTokens != 0 {
		t.Fatalf("failed delivery left %d unusable reset tokens", unusedTokens)
	}

	sender.err = nil
	if err := service.ForgotPassword(ctx, ForgotPasswordInput{Email: email}); err != nil {
		t.Fatalf("request password reset: %v", err)
	}
	if sender.recipient != email || sender.token == "" {
		t.Fatalf("reset email was not delivered to the account: recipient=%q tokenPresent=%t",
			sender.recipient, sender.token != "")
	}

	sum := sha256.Sum256([]byte(sender.token))
	var storedHash string
	var expiresAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT token_hash, expires_at
		FROM password_reset_tokens
		WHERE user_id = $1 AND used_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, registered.User.ID).Scan(&storedHash, &expiresAt); err != nil {
		t.Fatalf("read stored reset token: %v", err)
	}
	if storedHash != hex.EncodeToString(sum[:]) {
		t.Fatal("database stored the raw reset token instead of its hash")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatal("reset token was not given a future expiry")
	}

	if err := service.ResetPassword(ctx, ResetPasswordInput{
		Token:       sender.token,
		NewPassword: "replacement-password-2026",
	}); err != nil {
		t.Fatalf("consume reset token: %v", err)
	}
	var activeSessions int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sessions WHERE user_id = $1 AND revoked_at IS NULL
	`, registered.User.ID).Scan(&activeSessions); err != nil {
		t.Fatalf("count active sessions: %v", err)
	}
	if activeSessions != 0 {
		t.Fatalf("password reset left %d active sessions", activeSessions)
	}
	if err := service.ResetPassword(ctx, ResetPasswordInput{
		Token:       sender.token,
		NewPassword: "another-replacement-2026",
	}); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("expected a reset token to be single-use, got %v", err)
	}
	if _, err := service.Login(ctx, LoginInput{Email: email, Password: "initial-password-2026"}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected old password to fail after reset, got %v", err)
	}
	if _, err := service.Login(ctx, LoginInput{Email: email, Password: "replacement-password-2026"}); err != nil {
		t.Fatalf("expected new password to work after reset: %v", err)
	}
}
