package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"craftbridge/backend/internal/platform/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type fakeAuthRepository struct {
	user          User
	sessions      map[string]int64
	revoked       string
	createUserErr error
}

func (r *fakeAuthRepository) CreateUser(_ context.Context, name, email, passwordHash string) (User, error) {
	if r.createUserErr != nil {
		return User{}, r.createUserErr
	}
	r.user = User{
		ID:           17,
		Name:         name,
		Email:        email,
		PasswordHash: passwordHash,
		Status:       "active",
		Roles:        []string{"buyer"},
	}
	return r.user, nil
}

func (r *fakeAuthRepository) FindByEmail(_ context.Context, email string) (User, error) {
	if r.user.ID == 0 || !strings.EqualFold(r.user.Email, email) {
		return User{}, pgx.ErrNoRows
	}
	return r.user, nil
}

func (r *fakeAuthRepository) FindByID(_ context.Context, id int64) (User, error) {
	if r.user.ID != id {
		return User{}, pgx.ErrNoRows
	}
	return r.user, nil
}

func (r *fakeAuthRepository) CreateSession(_ context.Context, userID int64, _ time.Time) (string, error) {
	if r.sessions == nil {
		r.sessions = make(map[string]int64)
	}
	r.sessions["11111111-2222-4333-8444-555555555555"] = userID
	return "11111111-2222-4333-8444-555555555555", nil
}

func (r *fakeAuthRepository) RevokeSession(_ context.Context, userID int64, sessionID string) error {
	if r.sessions[sessionID] != userID {
		return pgx.ErrNoRows
	}
	r.revoked = sessionID
	delete(r.sessions, sessionID)
	return nil
}

func TestRegisterCreatesBuyerOnlyAndIssuesRevocableToken(t *testing.T) {
	repo := &fakeAuthRepository{}
	service := NewService(repo, nil, &config.Config{JWTSecret: "test-secret", JWTExpiryHours: 1})

	response, err := service.Register(context.Background(), RegisterInput{
		Name:     " Adeesha Perera ",
		Email:    " ADEESHA@example.test ",
		Password: "correct-horse",
	})
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if repo.user.Name != "Adeesha Perera" || repo.user.Email != "adeesha@example.test" {
		t.Fatalf("registration fields were not normalized: %+v", repo.user)
	}
	if len(repo.user.Roles) != 1 || repo.user.Roles[0] != "buyer" {
		t.Fatalf("public registration granted unexpected roles: %v", repo.user.Roles)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(repo.user.PasswordHash), []byte("correct-horse")); err != nil {
		t.Fatal("stored password is not a valid hash")
	}

	parts := strings.Split(response.AccessToken, ".")
	if len(parts) != 3 {
		t.Fatalf("expected a signed JWT, got %q", response.AccessToken)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode token claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("parse token claims: %v", err)
	}
	if claims["sub"] != "17" || claims["role"] != "buyer" ||
		claims["jti"] != "11111111-2222-4333-8444-555555555555" {
		t.Fatalf("unexpected token claims: %#v", claims)
	}
	roles, ok := claims["roles"].([]any)
	if !ok || len(roles) != 1 || roles[0] != "buyer" {
		t.Fatalf("expected buyer roles in JWT, got %#v", claims["roles"])
	}
}

func TestRegisterRejectsDuplicateEmailByPostgresErrorCode(t *testing.T) {
	repo := &fakeAuthRepository{createUserErr: &pgconn.PgError{Code: "23505"}}
	service := NewService(repo, nil, &config.Config{JWTSecret: "test-secret"})

	_, err := service.Register(context.Background(), RegisterInput{
		Name:     "Adeesha",
		Email:    "adeesha@example.test",
		Password: "correct-horse",
	})
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("expected duplicate email error, got %v", err)
	}
}

func TestLogoutRevokesOnlyTheAuthenticatedUsersSession(t *testing.T) {
	repo := &fakeAuthRepository{
		sessions: map[string]int64{"11111111-2222-4333-8444-555555555555": 17},
	}
	service := NewService(repo, nil, &config.Config{JWTSecret: "test-secret"})

	if err := service.Logout(context.Background(), 17, "11111111-2222-4333-8444-555555555555"); err != nil {
		t.Fatalf("Logout returned error: %v", err)
	}
	if repo.revoked == "" {
		t.Fatal("expected session to be revoked")
	}
	if err := service.Logout(context.Background(), 18, repo.revoked); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expected another user's session to remain inaccessible, got %v", err)
	}
}

func TestForgotPasswordReturnsDeliveryErrorWhenSenderIsMissing(t *testing.T) {
	repo := &fakeAuthRepository{
		user: User{ID: 17, Email: "adeesha@example.test"},
	}
	service := NewService(repo, nil, &config.Config{})

	err := service.ForgotPassword(context.Background(), ForgotPasswordInput{Email: repo.user.Email})
	if !errors.Is(err, ErrPasswordResetDeliveryFailed) {
		t.Fatalf("expected explicit missing email sender error, got %v", err)
	}
}
