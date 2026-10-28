package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/gin-gonic/gin"
)

// ServerParams bundles all dependencies and configuration for Server initialization.
type ServerParams struct {
	Config          *config.Config
	Middlewares     []gin.HandlerFunc
	APIHandler      *APIHandler
	InternalHandler *InternalHandler
	HealthChecker   *HealthChecker
}

// Server wraps the standard library http.Server and handles graceful lifecycles.
type Server struct {
	config     *config.Config
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
		config: params.Config,
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", params.Config.HTTPPort),
			Handler:           router,
			ReadTimeout:       params.Config.HTTPReadTimeout,
			WriteTimeout:      params.Config.HTTPWriteTimeout,
			IdleTimeout:       params.Config.HTTPIdleTimeout,
			ReadHeaderTimeout: params.Config.HTTPHeaderTimeout,
		},
	}
}

func (server *Server) Start() error {
	if err := server.httpServer.ListenAndServe(); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (server *Server) Shutdown(ctx context.Context) error {
	return server.httpServer.Shutdown(ctx)
}
