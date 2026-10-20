package server

import "github.com/gin-gonic/gin"



// RouteRegistrar is implemented by every module handler.
type RouteRegistrar interface {
	RegisterRoutes(routerGroup *gin.RouterGroup)
}

// APIHandler coordinates all module handlers.
type APIHandler struct {
	handlers []RouteRegistrar
}

// NewAPIHandler creates APIHandler with route registrars.
func NewAPIHandler(
	handlers []RouteRegistrar,
) *APIHandler {
	return &APIHandler{
		handlers: handlers,
	}
}

// RegisterRoutes registers all routes under /api/v1.
func (handler *APIHandler) RegisterRoutes(
	router *gin.Engine,
) {
	api := router.Group("/api/v1")

	for _, handlerItem := range handler.handlers {
		handlerItem.RegisterRoutes(api)
	}
}

// InternalHandler coordinates all internal module handlers.
type InternalHandler struct {
	handlers []RouteRegistrar
}

// NewInternalHandler creates InternalHandler with route registrars.
func NewInternalHandler(
	handlers []RouteRegistrar,
) *InternalHandler {
	return &InternalHandler{
		handlers: handlers,
	}
}

// RegisterRoutes registers all routes under /internal/v1.
func (handler *InternalHandler) RegisterRoutes(
	router *gin.Engine,
) {
	internal := router.Group("/internal/v1")

	for _, handlerItem := range handler.handlers {
		handlerItem.RegisterRoutes(internal)
	}
}

