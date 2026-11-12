// Package app wires apm_master's components. cmd/apm and cmd/scheduler call
// RunAPI and RunScheduler; nothing else constructs the dependency graph.
package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maitrimodii/apm_master/internal/alert"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/logs"
	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/platform/pg"
	"github.com/Maitrimodii/apm_master/internal/platform/secretbox"
	"github.com/Maitrimodii/apm_master/internal/retention"
	"github.com/Maitrimodii/apm_master/internal/scheduler"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

// MigrationsDir is where SQL migrations live relative to the working directory.
const MigrationsDir = "migrations"

// connectPostgres applies migrations when enabled and opens the pool.
func connectPostgres(ctx context.Context, cfg *config.Config, log *slog.Logger) (*pgxpool.Pool, error) {
	if cfg.App.ShouldAutoMigrate() {
		if err := pg.Migrate(cfg.Postgres.MigrateDSN(), MigrationsDir); err != nil {
			return nil, fmt.Errorf("apply database migrations: %w", err)
		}
		log.Info("database migrations applied")
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Postgres.Timeout)
	defer cancel()
	pool, err := pg.Connect(ctx, cfg.Postgres.PoolDSN(), pg.PoolConfig{
		MaxConns:        cfg.Postgres.MaxConns,
		MinConns:        cfg.Postgres.MinConns,
		MaxConnIdleTime: cfg.Postgres.MaxConnIdleTime,
		MaxConnLifetime: cfg.Postgres.MaxConnLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	log.Info("database connected")
	return pool, nil
}

// newNotificationService builds the notification queue with SMTP delivery.
// Its keyring encrypts and decrypts the payload of sensitive emails; the
// configuration was validated at startup, so a parse error here is a bug.
func newNotificationService(pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger) (*notification.Service, error) {
	keys, err := secretbox.ParseKeyring(cfg.Notify.EncryptionKeys, cfg.Notify.EncryptionKeyID)
	if err != nil {
		return nil, fmt.Errorf("notification encryption keys: %w", err)
	}
	store := notification.NewStore(pool, keys)
	smtp := notification.NewSMTPProvider(notification.SMTPConfig{
		Host: cfg.SMTP.Host, Port: cfg.SMTP.Port, Username: cfg.SMTP.Username, Password: cfg.SMTP.Password, From: cfg.SMTP.From,
		AllowInsecure: cfg.SMTP.AllowInsecure,
	})
	return notification.NewService(store, notification.NewWorker(store, []notification.Provider{smtp}, log, 0), log), nil
}

// lockFunc binds a cluster-wide advisory lock key for a singleton task.
func lockFunc(pool *pgxpool.Pool, key int64) func(context.Context, func(context.Context) error) (bool, error) {
	return func(ctx context.Context, fn func(context.Context) error) (bool, error) {
		return pg.WithAdvisoryLock(ctx, pool, key, fn)
	}
}

// newScheduler registers alert evaluation and retention. Registration
// failures stop startup: a scheduler silently missing a task is worse.
func newScheduler(pool *pgxpool.Pool, cfg *config.Config, notifier alert.Notifier, log *slog.Logger) (*scheduler.Scheduler, error) {
	s := scheduler.New(log)
	evaluation := alert.NewEvaluationTask(alert.EvaluationTaskDeps{
		Store:    alert.NewStore(pool),
		Metrics:  metrics.NewStore(pool),
		Logs:     logs.NewStore(pool),
		Traces:   traces.NewStore(pool),
		Notifier: notifier,
		Lock:     lockFunc(pool, pg.LockAlertEvaluation),
		Interval: cfg.Alerting.EvaluationInterval,
		Logger:   log,
	})
	cleanup := retention.NewTask(
		retention.NewStore(pool),
		retention.DefaultPolicies(cfg.Retention.NotificationDays, cfg.Retention.RefreshTokenDays),
		lockFunc(pool, pg.LockRetention),
		log,
	)
	// Erasing expired dead-letter ciphertext needs no keys.
	jobs := notification.NewStore(pool, nil)
	purge := notification.NewPayloadPurgeTask(jobs, cfg.Notify.DeadLetterRecovery(), log)
	recovery := notification.NewStaleJobRecoveryTask(jobs, cfg.Notify.StaleProcessing(), log)
	for _, task := range []scheduler.Task{evaluation, cleanup, purge, recovery} {
		if err := s.Register(task); err != nil {
			return nil, fmt.Errorf("register %s task: %w", task.Name(), err)
		}
	}
	return s, nil
}
