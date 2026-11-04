package logs

import (
	"context"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/google/uuid"
)

type Service struct {
	store Reader
	log   *slog.Logger
}

func NewService(store Reader, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, log: log}
}

func (s *Service) ListLogs(ctx context.Context, filter ListFilter) ([]LogRecord, int, error) {
	records, total, err := s.store.ListLogs(ctx, filter)
	if err != nil {
		s.log.ErrorContext(ctx, "list logs failed", "org_id", filter.OrgID, "error", err)
		return nil, 0, apperr.Internal(err)
	}
	s.log.DebugContext(ctx, "logs listed", "org_id", filter.OrgID, "total", total, "limit", filter.Limit, "offset", filter.Offset)
	return records, total, nil
}

func (s *Service) GetTraceLogs(ctx context.Context, orgID uuid.UUID, traceID string) ([]LogRecord, error) {
	records, err := s.store.GetTraceLogs(ctx, orgID, traceID)
	if err != nil {
		s.log.ErrorContext(ctx, "get trace logs failed", "org_id", orgID, "trace_id", traceID, "error", err)
		return nil, apperr.Internal(err)
	}
	s.log.DebugContext(ctx, "trace logs fetched", "org_id", orgID, "trace_id", traceID, "count", len(records))
	return records, nil
}
