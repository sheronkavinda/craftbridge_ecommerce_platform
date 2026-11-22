package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"craftbridge/backend/internal/platform/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials          = errors.New("invalid email or password")
	ErrEmailAlreadyExists          = errors.New("email already exists")
	ErrInvalidEmail                = errors.New("invalid email")
	ErrWeakPassword                = errors.New("password must be at least 8 characters")
	ErrPasswordTooLong             = errors.New("password must be 72 bytes or fewer")
	ErrInvalidName                 = errors.New("name is required and must be 120 characters or fewer")
	ErrInvalidResetToken           = errors.New("invalid or expired reset token")
	ErrPasswordResetDeliveryFailed = errors.New("password reset email delivery failed")
)

type Service struct {
	repo             authRepository
	db               *pgxpool.Pool
	cfg              *config.Config
	resetEmailSender PasswordResetEmailSender
}

type authRepository interface {
	CreateUser(context.Context, string, string, string) (User, error)
	FindByEmail(context.Context, string) (User, error)
	FindByID(context.Context, int64) (User, error)
	CreateSession(context.Context, int64, time.Time) (string, error)
	RevokeSession(context.Context, int64, string) error
}

func NewService(repo authRepository, db *pgxpool.Pool, cfg *config.Config) *Service {
	return &Service{repo: repo, db: db, cfg: cfg}
}

func (s *Service) SetPasswordResetEmailSender(sender PasswordResetEmailSender) {
	s.resetEmailSender = sender
}

type AuthResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	User        struct {
		ID    int64    `json:"id"`
		Name  string   `json:"name"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	} `json:"user"`
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (AuthResponse, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	if input.Name == "" || len([]rune(input.Name)) > 120 {
		return AuthResponse{}, ErrInvalidName
	}
	parsedEmail, err := mail.ParseAddress(input.Email)
	if err != nil || parsedEmail.Address != input.Email || len(input.Email) > 255 {
		return AuthResponse{}, ErrInvalidEmail
	}
	if len(input.Password) < 8 {
		return AuthResponse{}, ErrWeakPassword
	}
	if len(input.Password) > 72 {
		return AuthResponse{}, ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResponse{}, err
	}
	user, err := s.repo.CreateUser(ctx, input.Name, input.Email, string(hash))
	if err != nil {
		if isUniqueViolation(err) {
			return AuthResponse{}, ErrEmailAlreadyExists
		}
		return AuthResponse{}, err
	}
	return s.issueToken(ctx, user)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (AuthResponse, error) {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	user, err := s.repo.FindByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthResponse{}, ErrInvalidCredentials
		}
		return AuthResponse{}, err
	}
	if user.Status != "active" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		return AuthResponse{}, ErrInvalidCredentials
	}
	return s.issueToken(ctx, user)
}

func (s *Service) Logout(ctx context.Context, userID int64, sessionID string) error {
	if userID <= 0 || strings.TrimSpace(sessionID) == "" {
		return errors.New("authenticated session is required")
	}
	return s.repo.RevokeSession(ctx, userID, sessionID)
}

func (s *Service) Me(ctx context.Context, userID int64) (User, error) {
	return s.repo.FindByID(ctx, userID)
}

func (s *Service) ForgotPassword(ctx context.Context, input ForgotPasswordInput) error {
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	user, err := s.repo.FindByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if s.resetEmailSender == nil {
		return ErrPasswordResetDeliveryFailed
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(sum[:])
	_, err = s.db.Exec(ctx, `
		INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
		VALUES (gen_random_uuid(), $1, $2, NOW() + INTERVAL '30 minutes')
	`, user.ID, tokenHash)
	if err != nil {
		return err
	}
	if err := s.resetEmailSender.SendPasswordReset(ctx, user.Email, token); err != nil {
		deliveryErr := fmt.Errorf("%w: %v", ErrPasswordResetDeliveryFailed, err)
		if _, cleanupErr := s.db.Exec(ctx,
			`DELETE FROM password_reset_tokens WHERE token_hash = $1 AND used_at IS NULL`,
			tokenHash,
		); cleanupErr != nil {
			return errors.Join(deliveryErr, fmt.Errorf("remove undelivered reset token: %w", cleanupErr))
		}
		return deliveryErr
	}
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	if len(input.NewPassword) < 8 {
		return ErrWeakPassword
	}
	if len(input.NewPassword) > 72 {
		return ErrPasswordTooLong
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(input.Token)))
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID int64
	err = tx.QueryRow(ctx, `
		SELECT user_id FROM password_reset_tokens
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		FOR UPDATE
	`, hex.EncodeToString(sum[:])).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidResetToken
		}
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, string(hash), userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE password_reset_tokens SET used_at = NOW() WHERE token_hash = $1`, hex.EncodeToString(sum[:])); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) issueToken(ctx context.Context, user User) (AuthResponse, error) {
	expiryHours := 24
	if s.cfg != nil {
		expiryHours = s.cfg.JWTExpiryHours
	}
	if expiryHours <= 0 {
		expiryHours = 24
	}
	role := "buyer"
	for _, candidate := range user.Roles {
		if candidate == "admin" {
			role = "admin"
			break
		}
		if candidate == "seller" {
			role = "seller"
		}
	}
	expires := time.Now().Add(time.Duration(expiryHours) * time.Hour)
	secret := ""
	if s.cfg != nil {
		secret = strings.TrimSpace(s.cfg.JWTSecret)
	}
	if secret == "" {
		return AuthResponse{}, errors.New("JWT secret is not configured")
	}
	sessionID, err := s.repo.CreateSession(ctx, user.ID, expires)
	if err != nil {
		return AuthResponse{}, err
	}
	claims := map[string]any{
		"sub":   strconv.FormatInt(user.ID, 10),
		"name":  user.Name,
		"role":  role,
		"exp":   expires.Unix(),
		"jti":   sessionID,
		"roles": user.Roles,
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return AuthResponse{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(header + "." + payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	var response AuthResponse
	response.AccessToken = fmt.Sprintf("%s.%s.%s", header, payload, signature)
	response.ExpiresIn = expires.Unix() - time.Now().Unix()
	response.User.ID = user.ID
	response.User.Name = user.Name
	response.User.Email = user.Email
	response.User.Roles = user.Roles
	return response, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
