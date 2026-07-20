package log

import (
	"context"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/server"
)

type Service struct {
	store *Store
	log   *slog.Logger
}

func NewService(store *Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}

	return &Service{store: store, log: log}
}

type ListParams struct {
	From        *time.Time
	To          *time.Time
	ServiceName string
	MinSeverity int
	TraceID     string
	Search      string
	Limit       int
	Offset      int
	Page        int
}

func (service *Service) ListLogs(ctx context.Context, orgID string, params ListParams) ([]LogRecord, int, error) {
	query := ListLogsQuery{
		OrgID:       orgID,
		From:        params.From,
		To:          params.To,
		ServiceName: params.ServiceName,
		MinSeverity: params.MinSeverity,
		TraceID:     params.TraceID,
		Search:      params.Search,
		Limit:       params.Limit,
		Offset:      params.Offset,
	}

	records, total, err := service.store.ListLogs(ctx, query)
	if err != nil {
		service.log.Error("list logs failed", "org_id", orgID, "error", err)
		return nil, 0, server.Internal(err)
	}

	service.log.Debug("logs listed", "org_id", orgID, "total", total, "limit", params.Limit, "offset", params.Offset)

	return records, total, nil
}

func (service *Service) GetTraceLogs(ctx context.Context, orgID, traceID string) ([]LogRecord, error) {
	records, err := service.store.GetTraceLogs(ctx, orgID, traceID)
	if err != nil {
		service.log.Error("get trace logs failed", "org_id", orgID, "trace_id", traceID, "error", err)
		return nil, server.Internal(err)
	}
	service.log.Debug("trace logs fetched", "org_id", orgID, "trace_id", traceID, "count", len(records))
	return records, nil
}
