package trace

import (
	"context"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/google/uuid"
)

type Service struct {
	store *Store
	log   *slog.Logger
}

type ListParams struct {
	From          time.Time
	To            time.Time
	ServiceName   string
	TraceID       string
	Endpoint      string
	StatusCode    string
	MinDurationMS float64
	MaxDurationMS float64
	Page          int
	Limit         int
	Offset        int
}

func NewService(store *Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}

	return &Service{store: store, log: log}
}

func (service *Service) GetTrace(ctx context.Context, orgID uuid.UUID, traceID string) ([]Span, error) {
	spans, err := service.store.GetTrace(ctx, orgID, traceID)
	if err != nil {
		service.log.ErrorContext(ctx, "get trace failed", "org_id", orgID, "trace_id", traceID, "error", err)
		return nil, server.Internal(err)
	}
	if len(spans) == 0 {
		return nil, server.NotFound(server.CodeNotFound, "trace not found")
	}
	service.log.DebugContext(ctx, "trace fetched", "org_id", orgID, "trace_id", traceID, "spans", len(spans))
	return spans, nil
}

func (service *Service) ListSpans(ctx context.Context, orgID uuid.UUID, params ListParams) ([]TraceSummary, int, error) {
	query := ListSpansQuery{
		OrgID:         orgID,
		From:          params.From,
		To:            params.To,
		ServiceName:   params.ServiceName,
		TraceID:       params.TraceID,
		Endpoint:      params.Endpoint,
		StatusCode:    params.StatusCode,
		MinDurationMS: params.MinDurationMS,
		MaxDurationMS: params.MaxDurationMS,
		Limit:         params.Limit,
		Offset:        params.Offset,
	}

	summaries, total, err := service.store.ListTraceSummaries(ctx, query)
	if err != nil {
		service.log.ErrorContext(ctx, "list trace summaries failed", "org_id", orgID, "error", err)
		return nil, 0, server.Internal(err)
	}

	service.log.DebugContext(ctx, "trace summaries listed", "org_id", orgID, "total", total, "limit", params.Limit, "offset", params.Offset)

	return summaries, total, nil
}
