package alert

import (
	"context"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
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
		// Scheduler is now generic and started independently in cmd/scheduler or cmd/apm
	}

	return NewHandler(service, deps.PermissionChecker, deps.TokenParser)
}
