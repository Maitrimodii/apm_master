package alert

import (
	"context"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/Maitrimodii/apm_master/internal/trace"
)

func NewModule(
	deps middleware.CommonModuleDeps,
	notificationService *notification.Service,
	cfg *config.Config,
	ctx context.Context,
) server.RouteRegistrar {
	store := NewStore(deps.DB)
	service := NewService(store, notificationService, deps.Logger)

	if ctx != nil && cfg != nil {
		scheduler := NewScheduler(
			store,
			metric.NewStore(deps.DB),
			trace.NewStore(deps.DB),
			log.NewStore(deps.DB),
			notificationService,
			cfg,
			deps.Logger,
		)
		scheduler.Start(ctx)
	}

	return NewHandler(service, deps.PermissionChecker, deps.TokenParser)
}
