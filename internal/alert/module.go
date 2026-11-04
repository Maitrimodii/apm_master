package alert

import (
	"github.com/Maitrimodii/apm_master/internal/platform/module"
	"github.com/Maitrimodii/apm_master/internal/server"
)

func NewModule(deps module.Deps, notifier Notifier) server.RouteRegistrar {
	service := NewService(NewStore(deps.DB), notifier, deps.Logger)
	return NewHandler(service, deps.PermissionChecker, deps.TokenParser, deps.RateLimit)
}
