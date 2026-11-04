package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/config"
)

// RunScheduler runs alert evaluation and retention without the API, for
// deployments that split the API and background work into separate processes.
func RunScheduler(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if err := cfg.ValidateScheduler(); err != nil {
		return fmt.Errorf("validate scheduler configuration: %w", err)
	}
	pool, err := connectPostgres(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	appCtx, appCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer appCancel()

	notifications := newNotificationService(pool, cfg.SMTP, log)
	notifications.Start(appCtx)

	jobs, err := newScheduler(pool, cfg, notifications, log)
	if err != nil {
		return err
	}
	jobs.Start(appCtx)
	log.Info("scheduler started")

	<-ctx.Done()
	log.Info("shutting down scheduler")
	jobs.Stop()
	appCancel()

	waitCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	select {
	case <-notifications.Done():
	case <-waitCtx.Done():
		return fmt.Errorf("notification worker: %w", waitCtx.Err())
	}
	log.Info("scheduler exited")
	return nil
}
