package retention

import (
	"context"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
)

type Policy struct {
	Table       string
	TimeColumn  string
	RetentionDays int
}

type Task struct {
	store    *Store
	log      *slog.Logger
	interval time.Duration
	policies []Policy
	lockKey  int64
}

func NewTask(store *Store, cfg *config.Config, logger *slog.Logger) *Task {
	if logger == nil {
		logger = slog.Default()
	}

	return &Task{
		store:    store,
		log:      logger,
		interval: 24 * time.Hour, // Run once a day
		lockKey:  0xB1B2B3B4,
		policies: []Policy{
			{
				Table:       "notification_jobs",
				TimeColumn:  "created_at",
				RetentionDays: cfg.NotificationRetentionDays,
			},
			{
				Table:       "notification_events",
				TimeColumn:  "created_at",
				RetentionDays: cfg.NotificationRetentionDays,
			},
			{
				Table:       "refresh_tokens",
				TimeColumn:  "expires_at",
				RetentionDays: cfg.RefreshTokenRetentionDays,
			},
		},
	}
}

func (t *Task) Name() string {
	return "data_retention"
}

func (t *Task) Interval() time.Duration {
	return t.interval
}

func (t *Task) Execute(ctx context.Context) error {
	acquired, err := t.store.AcquireLock(ctx, t.lockKey)
	if err != nil || !acquired {
		t.log.Debug("retention task skipped, lock held by another instance or error", "error", err)
		return nil
	}
	defer t.store.ReleaseLock(ctx, t.lockKey)

	t.log.Info("starting retention cleanup")

	for _, policy := range t.policies {
		if policy.RetentionDays <= 0 {
			continue // disabled
		}

		totalDeleted := int64(0)
		batchSize := 5000

		for {
			deleted, err := t.store.DeleteOldRecords(ctx, policy.Table, policy.TimeColumn, policy.RetentionDays, batchSize)
			if err != nil {
				t.log.Error("failed to delete old records", "table", policy.Table, "error", err)
				break
			}
			
			totalDeleted += deleted
			if deleted < int64(batchSize) {
				break // No more rows to delete
			}
			
			// Small sleep to yield to other queries
			time.Sleep(100 * time.Millisecond)
		}
		
		t.log.Info("retention cleanup completed for table", "table", policy.Table, "deleted_rows", totalDeleted)
	}

	return nil
}
