package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Maitrimodii/apm_master/internal/alert"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/database"
	"github.com/Maitrimodii/apm_master/internal/log"
	applogger "github.com/Maitrimodii/apm_master/internal/logger"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/retention"
	"github.com/Maitrimodii/apm_master/internal/scheduler"
	"github.com/Maitrimodii/apm_master/internal/trace"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("scheduler application failed",
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

	logger := applogger.New("apm_scheduler", cfg.AppEnv, cfg.LogLevel)
	slog.SetDefault(logger)

	if err := cfg.ValidateScheduler(); err != nil {
		return fmt.Errorf("validate scheduler configuration: %w", err)
	}

	if err := database.Migrate(cfg.GetPostgresDSN(), "migrations"); err != nil {
		return fmt.Errorf("apply database migrations: %w", err)
	}
	logger.Info("database migrations applied")

	dbCtx, dbCancel := context.WithTimeout(context.Background(), cfg.PostgresTimeout)
	defer dbCancel()

	databaseInstance, err := database.Connect(dbCtx, cfg.GetPoolDSN(), cfg.GetPoolConfig())
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	logger.Info("database connected")
	defer databaseInstance.Close()

	// Notification service depends on db
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	notificationService := notification.NewService(databaseInstance.Pool(), cfg, logger)
	notificationService.Start(appCtx)

	// Set up the generic scheduler
	jobScheduler := scheduler.New(logger)

	// Register the alert evaluation task
	alertTask := alert.NewEvaluationTask(
		alert.NewStore(databaseInstance.Pool()),
		metric.NewStore(databaseInstance.Pool()),
		trace.NewStore(databaseInstance.Pool()),
		log.NewStore(databaseInstance.Pool()),
		notificationService,
		cfg,
		logger,
	)

	if err := jobScheduler.Register(alertTask); err != nil {
		return fmt.Errorf("failed to register alert task: %w", err)
	}

	retentionStore := retention.NewStore(databaseInstance.Pool())
	retentionTask := retention.NewTask(retentionStore, cfg, logger)
	if err := jobScheduler.Register(retentionTask); err != nil {
		return fmt.Errorf("failed to register retention task: %w", err)
	}

	jobScheduler.Start(appCtx)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit
	signal.Stop(quit)

	logger.Info("shutting down scheduler...")

	var shutdownErrs []error

	jobScheduler.Stop()
	appCancel()

	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	select {
	case <-notificationService.Done():
	case <-notifyCtx.Done():
		logger.Error("notification worker shutdown timed out", slog.Any("error", notifyCtx.Err()))
		shutdownErrs = append(shutdownErrs, fmt.Errorf("notification worker: %w", notifyCtx.Err()))
	}
	notifyCancel()

	logger.Info("scheduler exited")

	if len(shutdownErrs) > 0 {
		return errors.Join(shutdownErrs...)
	}
	return nil
}
