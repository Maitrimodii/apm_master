package investigation

import (
	"github.com/Maitrimodii/apm_master/internal/investigation/evidence"
	"github.com/Maitrimodii/apm_master/internal/platform/module"
	"github.com/Maitrimodii/apm_master/internal/server"
)

// NewModule creates the public investigation API (mounted under /api/v1).
func NewModule(deps module.Deps, incidents IncidentReader) server.RouteRegistrar {
	service := NewService(NewStore(deps.DB), incidents, deps.Logger)
	return NewHandler(service, deps.PermissionChecker, deps.TokenParser, deps.RateLimit)
}

// NewInternalModule creates the service-to-service API used by
// apm-ai-service (mounted under /internal/v1).
func NewInternalModule(deps module.Deps, incidents IncidentReader, readers evidence.Readers, serviceToken string) server.RouteRegistrar {
	store := NewStore(deps.DB)
	return NewInternalHandler(
		NewService(store, incidents, deps.Logger),
		NewEvidenceService(store, evidence.NewRegistry(readers), deps.Logger),
		serviceToken,
	)
}
