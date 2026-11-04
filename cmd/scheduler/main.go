// Command scheduler runs alert evaluation and data retention on their own,
// for deployments that separate them from the API (APP_MODE=scheduler).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Maitrimodii/apm_master/internal/app"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("scheduler failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	log := logger.New("apm_scheduler", cfg.App.Env, cfg.App.LogLevel)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return app.RunScheduler(ctx, cfg, log)
}
