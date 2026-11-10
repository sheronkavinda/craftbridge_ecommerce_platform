package audit

import (
	"context"
	"errors"
	"html"
	"strings"
)

var ErrInvalidAuditInput = errors.New("invalid audit log input")

// Service provides business logic for recording and querying append-only audit logs.
type Service struct {
	repo *Repository
}

// NewService creates a new audit service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// LogAction sanitizes and validates inputs before persisting an immutable audit trail entry.
func (s *Service) LogAction(ctx context.Context, input CreateAuditLogInput) (*AuditLog, error) {
	input.ActorID = sanitizeText(input.ActorID, 128)
	input.ActorName = sanitizeText(input.ActorName, 128)
	input.Action = strings.ToUpper(sanitizeText(input.Action, 64))
	input.Target = sanitizeText(input.Target, 256)
	input.Outcome = strings.ToUpper(sanitizeText(input.Outcome, 32))
	input.RequestID = sanitizeText(input.RequestID, 64)
	input.Details = sanitizeText(input.Details, 1024)

	if input.Action == "" || input.Target == "" {
		return nil, ErrInvalidAuditInput
	}
	if input.ActorID == "" {
		input.ActorID = "system"
	}
	if input.ActorName == "" {
		input.ActorName = "System"
	}
	if input.Outcome != "SUCCESS" && input.Outcome != "FAILURE" {
		input.Outcome = "SUCCESS"
	}

	return s.repo.Create(ctx, input)
}

// ListLogs sanitizes filter parameters and returns append-only audit log entries.
func (s *Service) ListLogs(ctx context.Context, filter ListAuditLogsFilter) ([]AuditLog, error) {
	filter.ActorID = sanitizeText(filter.ActorID, 128)
	filter.Action = strings.ToUpper(sanitizeText(filter.Action, 64))
	filter.Outcome = strings.ToUpper(sanitizeText(filter.Outcome, 32))
	return s.repo.List(ctx, filter)
}

func sanitizeText(val string, maxRunes int) string {
	trimmed := strings.TrimSpace(val)
	if maxRunes > 0 {
		runes := []rune(trimmed)
		if len(runes) > maxRunes {
			trimmed = string(runes[:maxRunes])
		}
	}
	return html.EscapeString(trimmed)
}
