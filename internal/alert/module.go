package alert

import (
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
)

func NewModule(
	deps middleware.CommonModuleDeps,
	notificationService *notification.Service,
) server.RouteRegistrar {
	store := NewStore(deps.DB)
	service := NewService(store, notificationService, deps.Logger)
	return NewHandler(service, deps.PermissionChecker, deps.TokenParser)
}
