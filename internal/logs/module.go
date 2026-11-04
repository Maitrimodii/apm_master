package logs

import (
	"github.com/Maitrimodii/apm_master/internal/platform/module"
	"github.com/Maitrimodii/apm_master/internal/server"
)

func NewModule(deps module.Deps) server.RouteRegistrar {
	service := NewService(NewStore(deps.DB), deps.Logger)
	return NewHandler(service, deps.PermissionChecker, deps.TokenParser, deps.RateLimit)
}
