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
) *Server {

	router := gin.New()

	for _, mw := range middlewares {
		router.Use(mw)
	}

	// Liveness probe (HTTP server is running)
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Readiness probe (Dependencies are available)
	router.GET("/readyz", func(c *gin.Context) {
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
	})

	handler.RegisterRoutes(router)

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

