package log

import (
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
)

func NewModule(
	deps middleware.CommonModuleDeps,
) server.RouteRegistrar {

	store := NewStore(deps.DB)

	service := NewService(
		store,
		deps.Logger,
	)

	handler := NewHandler(
		service,
		deps.PermissionChecker,
		deps.TokenParser,
	)

	return handler
}
