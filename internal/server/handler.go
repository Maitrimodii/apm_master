package server

import "github.com/gin-gonic/gin"

// RouteRegistrar is implemented by every module handler.
type RouteRegistrar interface {
	RegisterRoutes(routerGroup *gin.RouterGroup)
}

// APIHandler coordinates all public domain module handlers.
type APIHandler struct {
	Auth          RouteRegistrar
	Ingest        RouteRegistrar
	Trace         RouteRegistrar
	Log           RouteRegistrar
	Metric        RouteRegistrar
	Alert         RouteRegistrar
	Investigation RouteRegistrar
}

// NewAPIHandler creates APIHandler with explicitly declared domain module registrars.
func NewAPIHandler(
	auth RouteRegistrar,
	ingest RouteRegistrar,
	trace RouteRegistrar,
	log RouteRegistrar,
	metric RouteRegistrar,
	alert RouteRegistrar,
	investigation RouteRegistrar,
) *APIHandler {
	return &APIHandler{
		Auth:          auth,
		Ingest:        ingest,
		Trace:         trace,
		Log:           log,
		Metric:        metric,
		Alert:         alert,
		Investigation: investigation,
	}
}

// RegisterRoutes registers all routes under RoutePrefixAPIV1.
// Each domain module is explicitly invoked by name so all active routes are visible at a glance.
func (h *APIHandler) RegisterRoutes(router *gin.Engine) {
	api := router.Group(RoutePrefixAPIV1)

	// Explicit module registrations
	if h.Auth != nil {
		h.Auth.RegisterRoutes(api)
	}
	if h.Ingest != nil {
		h.Ingest.RegisterRoutes(api)
	}
	if h.Trace != nil {
		h.Trace.RegisterRoutes(api)
	}
	if h.Log != nil {
		h.Log.RegisterRoutes(api)
	}
	if h.Metric != nil {
		h.Metric.RegisterRoutes(api)
	}
	if h.Alert != nil {
		h.Alert.RegisterRoutes(api)
	}
	if h.Investigation != nil {
		h.Investigation.RegisterRoutes(api)
	}
}

// InternalHandler coordinates all internal service-to-service module handlers.
type InternalHandler struct {
	Investigation RouteRegistrar
}

// NewInternalHandler creates InternalHandler with explicitly declared internal registrars.
func NewInternalHandler(
	investigation RouteRegistrar,
) *InternalHandler {
	return &InternalHandler{
		Investigation: investigation,
	}
}

// RegisterRoutes registers all internal routes under RoutePrefixInternalV1.
func (h *InternalHandler) RegisterRoutes(router *gin.Engine) {
	internal := router.Group(RoutePrefixInternalV1)

	if h.Investigation != nil {
		h.Investigation.RegisterRoutes(internal)
	}
}
