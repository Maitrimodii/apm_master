package investigation

import (
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewModule creates the public investigation route registrar (mounted under /api/v1).
func NewModule(deps middleware.CommonModuleDeps) server.RouteRegistrar {
	store := NewStore(deps.DB)
	service := NewService(store, deps.Logger)
	return NewHandler(service, deps.PermissionChecker, deps.TokenParser)
}

// NewInternalModule creates the internal evidence route registrar (mounted under /internal/v1).
func NewInternalModule(deps middleware.CommonModuleDeps, cfg *config.Config) server.RouteRegistrar {
	store := NewStore(deps.DB)
	metricStore := metric.NewStore(deps.DB)
	traceStore := trace.NewStore(deps.DB)
	logStore := log.NewStore(deps.DB)

	return NewEvidenceHandler(store, metricStore, traceStore, logStore, cfg, deps.Logger)
}

// NewPublisher returns an initialized OutboxPublisher.
func NewPublisher(pool *pgxpool.Pool, writer KafkaWriter, cfg *config.Config, log *slog.Logger) *OutboxPublisher {
	return NewOutboxPublisher(pool, writer, cfg, log)
}
