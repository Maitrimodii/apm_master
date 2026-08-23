package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Maitrimodii/apm_master/internal/alert"
	"github.com/Maitrimodii/apm_master/internal/auth"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/database"
	"github.com/Maitrimodii/apm_master/internal/ingest"
	"github.com/Maitrimodii/apm_master/internal/log"
	applogger "github.com/Maitrimodii/apm_master/internal/logger"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/retention"
	"github.com/Maitrimodii/apm_master/internal/scheduler"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/Maitrimodii/apm_master/internal/trace"

	"github.com/gin-gonic/gin"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("application failed",
			slog.Any("error", err),
		)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := applogger.New("apm_master", cfg.AppEnv, cfg.LogLevel)
	slog.SetDefault(logger)

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	if err := database.Migrate(cfg.GetPostgresDSN(), "migrations"); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	logger.Info("database migrations applied")

	dbCtx, dbCancel := context.WithTimeout(context.Background(), cfg.PostgresTimeout)
	defer dbCancel()

	databaseInstance, err := database.Connect(dbCtx, cfg.GetPoolDSN())
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	logger.Info("database connected")
	defer databaseInstance.Close()

	redisCtx, redisCancel := context.WithTimeout(context.Background(), cfg.RedisTimeout)
	defer redisCancel()

	redisClient, err := database.ConnectRedis(redisCtx, cfg.RedisURL, cfg.RedisTimeout)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	logger.Info("redis connected")
	defer redisClient.Close()

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	notificationService := notification.NewService(databaseInstance.Pool(), cfg, logger)
	notificationService.Start(appCtx)

	identityModule := auth.NewModule(
		databaseInstance.Pool(),
		redisClient.Client,
		cfg,
		notificationService,
		logger,
	)

	ingestModule, err := ingest.NewModule(
		appCtx,
		databaseInstance.Pool(),
		cfg,
		identityModule.APIKeyValidator,
		logger,
	)
	if err != nil {
		return fmt.Errorf("create ingest module: %w", err)
	}

	moduleDeps := middleware.CommonModuleDeps{
		DB:                databaseInstance.Pool(),
		PermissionChecker: identityModule.PermissionChecker,
		TokenParser:       identityModule.TokenParser,
		Logger:            logger,
	}

	apiHandler := server.NewAPIHandler(
		[]server.RouteRegistrar{
			identityModule,
			ingestModule,
			trace.NewModule(moduleDeps),
			log.NewModule(moduleDeps),
			metric.NewModule(moduleDeps),
			alert.NewModule(
				moduleDeps,
				notificationService,
			),
		},
	)

	middlewares := []gin.HandlerFunc{
		middleware.RequestID(),
		middleware.Logger(logger),
		middleware.Recovery(logger),
		middleware.BodyLimit(cfg.MaxBodyBytes),
		middleware.NewRateLimiter(redisClient.Client, cfg.RateLimitLimit, cfg.RateLimitBurst, cfg.RateLimitWindow).Limit(),
	}

	healthChecker := server.NewHealthChecker(databaseInstance.Pool(), redisClient.Client)

	srv := server.NewServer(
		cfg,
		middlewares,
		apiHandler,
		healthChecker,
	)

	var jobScheduler *scheduler.Scheduler
	if cfg.AppMode == "all" || cfg.AppMode == "" {
		jobScheduler = scheduler.New(logger)
		
		alertTask := alert.NewEvaluationTask(
			alert.NewStore(databaseInstance.Pool()),
			metric.NewStore(databaseInstance.Pool()),
			trace.NewStore(databaseInstance.Pool()),
			log.NewStore(databaseInstance.Pool()),
			notificationService,
			cfg,
			logger,
		)
		
		retentionStore := retention.NewStore(databaseInstance.Pool())
		retentionTask := retention.NewTask(retentionStore, cfg, logger)
		
		if err := jobScheduler.Register(alertTask); err != nil {
			logger.Error("failed to register alert evaluation task", slog.Any("error", err))
		}
		if err := jobScheduler.Register(retentionTask); err != nil {
			logger.Error("failed to register retention task", slog.Any("error", err))
		}
		
		jobScheduler.Start(appCtx)
		logger.Info("scheduler started (AppMode: all)")
	}

	go func() {
		logger.Info("server started", "port", cfg.HTTPPort)

		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", slog.Any("error", err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	signal.Stop(quit)

	logger.Info("shutting down server...")

	var shutdownErrs []error

	// Each component gets its own full shutdownTimeout budget,
	// so a slow HTTP drain doesn't eat into ingest's shutdown time.
	srvCtx, srvCancel := context.WithTimeout(appCtx, shutdownTimeout)
	if err := srv.Shutdown(srvCtx); err != nil {
		logger.Error("shutdown failed", slog.Any("error", err))
		shutdownErrs = append(shutdownErrs, fmt.Errorf("server shutdown: %w", err))
	}
	srvCancel()

	ingestCtx, ingestCancel := context.WithTimeout(appCtx, shutdownTimeout)
	if err := ingestModule.Shutdown(ingestCtx); err != nil {
		logger.Error("ingest shutdown failed", slog.Any("error", err))
		shutdownErrs = append(shutdownErrs, fmt.Errorf("ingest shutdown: %w", err))
	}
	ingestCancel()

	appCancel()
	
	if jobScheduler != nil {
		jobScheduler.Stop()
	}

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	select {
	case <-notificationService.Done():
	case <-notifyCtx.Done():
		logger.Error("notification worker shutdown timed out", slog.Any("error", notifyCtx.Err()))
		shutdownErrs = append(shutdownErrs, fmt.Errorf("notification worker: %w", notifyCtx.Err()))
	}
	notifyCancel()

	logger.Info("server exited")

	if len(shutdownErrs) > 0 {
		return errors.Join(shutdownErrs...)
	}
	return nil

}
