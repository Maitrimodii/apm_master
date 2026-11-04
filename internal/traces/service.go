package traces

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

func (s *Service) GetTrace(ctx context.Context, orgID uuid.UUID, traceID string) ([]Span, error) {
	spans, err := s.store.GetTrace(ctx, orgID, traceID)
	if err != nil {
		s.log.ErrorContext(ctx, "get trace failed", "org_id", orgID, "trace_id", traceID, "error", err)
		return nil, apperr.Internal(err)
	}
	if len(spans) == 0 {
		return nil, apperr.NotFound(apperr.CodeNotFound, "trace not found")
	}
	s.log.DebugContext(ctx, "trace fetched", "org_id", orgID, "trace_id", traceID, "spans", len(spans))
	return spans, nil
}

func (s *Service) ListTraceSummaries(ctx context.Context, filter ListFilter) ([]TraceSummary, int, error) {
	summaries, total, err := s.store.ListTraceSummaries(ctx, filter)
	if err != nil {
		s.log.ErrorContext(ctx, "list trace summaries failed", "org_id", filter.OrgID, "error", err)
		return nil, 0, apperr.Internal(err)
	}
	s.log.DebugContext(ctx, "trace summaries listed", "org_id", filter.OrgID, "total", total, "limit", filter.Limit, "offset", filter.Offset)
	return summaries, total, nil
}
