package audit

import "time"

// AuditLog represents a single audit trail entry stored in the audit_logs table.
type AuditLog struct {
	ID        int64     `json:"id"`
	ActorID   string    `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Outcome   string    `json:"outcome"`
	RequestID string    `json:"request_id"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAuditLogInput represents the payload required to record a new audit log entry.
type CreateAuditLogInput struct {
	ActorID   string `json:"actor_id"`
	ActorName string `json:"actor_name"`
	Action    string `json:"action" binding:"required"`
	Target    string `json:"target" binding:"required"`
	Outcome   string `json:"outcome"`
	RequestID string `json:"request_id"`
	Details   string `json:"details"`
}

// ListAuditLogsFilter defines optional filtering and pagination parameters for querying audit logs.
type ListAuditLogsFilter struct {
	ActorID string
	Action  string
	Outcome string
	Limit   int
	Offset  int
}
