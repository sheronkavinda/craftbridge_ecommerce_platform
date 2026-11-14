package audit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository handles database persistence for audit logs.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new audit log repository backed by a PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create inserts a new audit log record into the audit_logs table and returns the created record.
func (r *Repository) Create(ctx context.Context, input CreateAuditLogInput) (*AuditLog, error) {
	const query = `
		INSERT INTO audit_logs (actor_id, actor_name, action, target, outcome, request_id, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, actor_id, actor_name, action, target, outcome, request_id, details, created_at
	`

	var entry AuditLog
	err := r.db.QueryRow(
		ctx,
		query,
		input.ActorID,
		input.ActorName,
		input.Action,
		input.Target,
		input.Outcome,
		input.RequestID,
		input.Details,
	).Scan(
		&entry.ID,
		&entry.ActorID,
		&entry.ActorName,
		&entry.Action,
		&entry.Target,
		&entry.Outcome,
		&entry.RequestID,
		&entry.Details,
		&entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert audit log: %w", err)
	}

	return &entry, nil
}

// List retrieves audit log records ordered by created_at descending with optional filters.
func (r *Repository) List(ctx context.Context, filter ListAuditLogsFilter) ([]AuditLog, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, actor_id, actor_name, action, target, outcome, request_id, details, created_at
		FROM audit_logs
		WHERE ($1 = '' OR actor_id = $1)
		  AND ($2 = '' OR action = $2)
		  AND ($3 = '' OR outcome = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`

	rows, err := r.db.Query(ctx, query, filter.ActorID, filter.Action, filter.Outcome, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close()

	logs := make([]AuditLog, 0)
	for rows.Next() {
		var entry AuditLog
		if err := rows.Scan(
			&entry.ID,
			&entry.ActorID,
			&entry.ActorName,
			&entry.Action,
			&entry.Target,
			&entry.Outcome,
			&entry.RequestID,
			&entry.Details,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log row: %w", err)
		}
		logs = append(logs, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log rows: %w", err)
	}

	return logs, nil
}
