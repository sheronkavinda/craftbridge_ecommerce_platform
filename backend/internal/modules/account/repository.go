package account

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetProfile(ctx context.Context, userID int64) (Profile, error) {
	var profile Profile
	err := r.db.QueryRow(ctx, `
		SELECT id, COALESCE(name, ''), email, created_at::text
		FROM users WHERE id = $1 AND status = 'active'
	`, userID).Scan(&profile.ID, &profile.Name, &profile.Email, &profile.CreatedAt)
	if err != nil {
		return Profile{}, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT roles.name FROM roles
		JOIN user_roles ON user_roles.role_id = roles.id
		WHERE user_roles.user_id = $1
		ORDER BY roles.name
	`, userID)
	if err != nil {
		return Profile{}, err
	}
	defer rows.Close()
	profile.Roles = make([]string, 0)
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return Profile{}, err
		}
		profile.Roles = append(profile.Roles, role)
	}
	return profile, rows.Err()
}

func (r *Repository) UpdateProfile(ctx context.Context, userID int64, name string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET name = $1, updated_at = NOW()
		WHERE id = $2 AND status = 'active'
	`, name, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) ListAddresses(ctx context.Context, userID int64) ([]Address, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, COALESCE(label,''), full_name, COALESCE(phone,''),
			address_line1, COALESCE(address_line2,''), city, state,
			postal_code, country_code, is_default
		FROM addresses WHERE user_id=$1 ORDER BY is_default DESC, id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	addresses := make([]Address, 0)
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.ID, &a.Label, &a.FullName, &a.Phone, &a.AddressLine1,
			&a.AddressLine2, &a.City, &a.State, &a.PostalCode, &a.CountryCode, &a.IsDefault); err != nil {
			return nil, err
		}
		addresses = append(addresses, a)
	}
	return addresses, rows.Err()
}

func (r *Repository) CreateAddress(ctx context.Context, userID int64, a Address) (Address, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer tx.Rollback(ctx)
	var lockedUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		return a, err
	}
	var hasDefault bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM addresses WHERE user_id = $1 AND is_default)`, userID).Scan(&hasDefault); err != nil {
		return a, err
	}
	a.IsDefault = a.IsDefault || !hasDefault
	if a.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE addresses SET is_default=false WHERE user_id=$1`, userID); err != nil {
			return a, err
		}
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO addresses
			(user_id,label,full_name,phone,address_line1,address_line2,city,state,postal_code,country_code,is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id
	`, userID, a.Label, a.FullName, a.Phone, a.AddressLine1, a.AddressLine2,
		a.City, a.State, a.PostalCode, a.CountryCode, a.IsDefault).Scan(&a.ID)
	if err != nil {
		return a, err
	}
	if err := tx.Commit(ctx); err != nil {
		return a, err
	}
	return a, nil
}

func (r *Repository) UpdateAddress(ctx context.Context, userID, addressID int64, a Address) (Address, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return a, err
	}
	defer tx.Rollback(ctx)

	var lockedUserID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		return a, err
	}
	var wasDefault bool
	err = tx.QueryRow(ctx, `
		SELECT is_default FROM addresses WHERE id = $1 AND user_id = $2 FOR UPDATE
	`, addressID, userID).Scan(&wasDefault)
	if err != nil {
		return a, err
	}
	if a.IsDefault {
		if _, err := tx.Exec(ctx, `UPDATE addresses SET is_default=false WHERE user_id=$1`, userID); err != nil {
			return a, err
		}
	}
	err = tx.QueryRow(ctx, `
		UPDATE addresses SET label=$1, full_name=$2, phone=$3, address_line1=$4,
			address_line2=$5, city=$6, state=$7, postal_code=$8,
			country_code=$9, is_default=$10, updated_at=NOW()
		WHERE id=$11 AND user_id=$12
		RETURNING id
	`, a.Label, a.FullName, a.Phone, a.AddressLine1, a.AddressLine2, a.City,
		a.State, a.PostalCode, a.CountryCode, a.IsDefault, addressID, userID).Scan(&a.ID)
	if err != nil {
		return a, err
	}
	if wasDefault && !a.IsDefault {
		if _, err := tx.Exec(ctx, `
			UPDATE addresses SET is_default = TRUE
			WHERE id = (
				SELECT id FROM addresses WHERE user_id = $1 AND id <> $2
				ORDER BY id LIMIT 1
			)
		`, userID, addressID); err != nil {
			return a, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return a, err
	}
	return a, nil
}

func (r *Repository) DeleteAddress(ctx context.Context, userID, addressID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		return err
	}
	var wasDefault bool
	err = tx.QueryRow(ctx, `
		DELETE FROM addresses WHERE id = $1 AND user_id = $2
		RETURNING is_default
	`, addressID, userID).Scan(&wasDefault)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return err
	}
	if wasDefault {
		if _, err := tx.Exec(ctx, `
			UPDATE addresses SET is_default = TRUE
			WHERE id = (
				SELECT id FROM addresses WHERE user_id = $1 ORDER BY id LIMIT 1
			)
		`, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
