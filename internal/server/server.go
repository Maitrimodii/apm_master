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
) *Server {

	router := gin.New()

	for _, mapItem := range middlewares {
		router.Use(mapItem)
	}

	// Health check
	router.GET(
		"/health",
		func(ginContext *gin.Context) {
			ginContext.JSON(http.StatusOK, gin.H{
				"status": "ok",
			})
		},
	)

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

func (server *Server) Shutdown(
	ctx context.Context,
) error {
	return server.httpServer.Shutdown(ctx)
}
