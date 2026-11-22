package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateUser(ctx context.Context, name, email, passwordHash string) (User, error) {
	var u User
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return u, err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, COALESCE(name, ''), status, created_at, updated_at
	`, name, email, passwordHash).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Status, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return u, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		VALUES ($1, (SELECT id FROM roles WHERE name = 'buyer'))
	`, u.ID); err != nil {
		return u, err
	}
	if err := tx.Commit(ctx); err != nil {
		return u, err
	}
	u.Roles = []string{"buyer"}
	return u, nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, COALESCE(name, ''), status, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return u, err
	}
	u.Roles, err = r.findRoles(ctx, u.ID)
	return u, err
}

func (r *Repository) FindByID(ctx context.Context, id int64) (User, error) {
	var u User
	err := r.db.QueryRow(ctx, `
		SELECT id, email, password_hash, COALESCE(name, ''), status, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return u, err
	}
	u.Roles, err = r.findRoles(ctx, id)
	return u, err
}

func (r *Repository) findRoles(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.name FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY r.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *Repository) UpdatePassword(ctx context.Context, userID int64, passwordHash string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2
	`, passwordHash, userID)
	return err
}

func (r *Repository) CreateSession(ctx context.Context, userID int64, expiresAt time.Time) (string, error) {
	var sessionID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO sessions (id, user_id, expires_at)
		VALUES (gen_random_uuid(), $1, $2)
		RETURNING id::text
	`, userID, expiresAt).Scan(&sessionID)
	return sessionID, err
}

func (r *Repository) RevokeSession(ctx context.Context, userID int64, sessionID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE sessions SET revoked_at = NOW()
		WHERE id = $1::uuid AND user_id = $2 AND revoked_at IS NULL
	`, sessionID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

var ErrUserNotFound = pgx.ErrNoRows
