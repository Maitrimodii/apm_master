package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/gin-gonic/gin"
)

type Server struct {
	config     *config.Config
	httpServer *http.Server
}

func NewServer(
	cfg *config.Config,
	middlewares []gin.HandlerFunc,
	handler *APIHandler,
	health *HealthChecker,
	internalHandler ...*InternalHandler,
) *Server {

	router := gin.New()

	for _, mw := range middlewares {
		router.Use(mw)
	}

	// Liveness probe (HTTP server is running)
	healthHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
	router.GET("/api/v1/healthz", healthHandler)

	// Readiness probe (Dependencies are available)
	readyHandler := func(c *gin.Context) {
		if health == nil {
			c.JSON(http.StatusOK, gin.H{"status": "ok", "note": "no readiness checker configured"})
			return
		}
		report := health.Check(c.Request.Context())
		status := http.StatusOK
		if !report.OK {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, report)
	}
	router.GET("/api/v1/readyz", readyHandler)

	// Ensure OPTIONS preflights never 404
	router.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	handler.RegisterRoutes(router)

	for _, ih := range internalHandler {
		if ih != nil {
			ih.RegisterRoutes(router)
		}
	}

	return &Server{
		config: cfg,
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", cfg.HTTPPort),
			Handler:           router,
			ReadTimeout:       cfg.HTTPReadTimeout,
			WriteTimeout:      cfg.HTTPWriteTimeout,
			IdleTimeout:       cfg.HTTPIdleTimeout,
			ReadHeaderTimeout: cfg.HTTPHeaderTimeout,
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

