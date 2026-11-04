package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/config"
)

// ServerParams bundles all dependencies and configuration for Server initialization.
type ServerParams struct {
	HTTP            config.HTTPConfig
	Middlewares     []gin.HandlerFunc
	APIHandler      *APIHandler
	InternalHandler *InternalHandler
	HealthChecker   *HealthChecker
}

// Server wraps the standard library http.Server and handles graceful lifecycles.
type Server struct {
	httpServer *http.Server
}

// NewServer initializes the Gin router, registers middleware, mounts health probes and API modules.
func NewServer(params ServerParams) *Server {
	router := gin.New()

	for _, mw := range params.Middlewares {
		router.Use(mw)
	}

	// 1. Kubernetes & Infrastructure Health Probes (mounted at root and /api/v1 alias)
	if params.HealthChecker != nil {
		router.GET("/healthz", params.HealthChecker.LiveHandler)
		router.GET("/readyz", params.HealthChecker.ReadyHandler)
		router.GET(RoutePrefixAPIV1+"/healthz", params.HealthChecker.LiveHandler)
		router.GET(RoutePrefixAPIV1+"/readyz", params.HealthChecker.ReadyHandler)
	}

	// 2. Public API V1 Modules
	if params.APIHandler != nil {
		params.APIHandler.RegisterRoutes(router)
	}

	// 3. Internal Service-to-Service Modules
	if params.InternalHandler != nil {
		params.InternalHandler.RegisterRoutes(router)
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              ":" + params.HTTP.Port,
			Handler:           router,
			ReadTimeout:       params.HTTP.ReadTimeout,
			WriteTimeout:      params.HTTP.WriteTimeout,
			IdleTimeout:       params.HTTP.IdleTimeout,
			ReadHeaderTimeout: params.HTTP.HeaderTimeout,
		},
	}
}

func (s *Server) Start() error {
	if err := s.httpServer.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
