package retention

import (
	"context"
	"log/slog"
	"time"
)

const (
	// DefaultInterval is how often retention runs.
	DefaultInterval = 24 * time.Hour
	// deleteBatchSize rows are deleted per statement.
	deleteBatchSize = 5000
	// batchPause yields to other queries between delete batches.
	batchPause = 100 * time.Millisecond
)

// Policy deletes rows of Table whose TimeColumn is older than RetentionDays.
// A zero or negative RetentionDays disables the policy.
type Policy struct {
	Table         string
	TimeColumn    string
	RetentionDays int
}

// DefaultPolicies covers the tables Timescale retention does not: telemetry
// hypertables expire through their own retention policies (migration 000009).
func DefaultPolicies(notificationDays, refreshTokenDays int) []Policy {
	return []Policy{
		{Table: "notification_jobs", TimeColumn: "created_at", RetentionDays: notificationDays},
		{Table: "notification_events", TimeColumn: "created_at", RetentionDays: notificationDays},
		{Table: "refresh_tokens", TimeColumn: "expires_at", RetentionDays: refreshTokenDays},
	}
}

// LockFunc runs fn while holding a cluster-wide lock and reports whether it ran.
type LockFunc func(ctx context.Context, fn func(ctx context.Context) error) (bool, error)

type deleter interface {
	DeleteOldRecords(ctx context.Context, table, timeCol string, days, batchSize int) (int64, error)
}

// Task deletes expired rows. It implements scheduler.Task.
type Task struct {
	store    deleter
	policies []Policy
	lock     LockFunc
	interval time.Duration
	log      *slog.Logger
}

// NewTask creates the retention task. lock may be nil to skip cluster locking.
func NewTask(store deleter, policies []Policy, lock LockFunc, log *slog.Logger) *Task {
	if log == nil {
		log = slog.Default()
	}
	return &Task{store: store, policies: policies, lock: lock, interval: DefaultInterval, log: log}
}

func (t *Task) Name() string            { return "data_retention" }
func (t *Task) Interval() time.Duration { return t.interval }

func (t *Task) Execute(ctx context.Context) error {
	if t.lock == nil {
		return t.run(ctx)
	}
	ran, err := t.lock(ctx, t.run)
	if !ran && err == nil {
		t.log.DebugContext(ctx, "retention skipped, another instance holds the lock")
	}
	return err
}

func (t *Task) run(ctx context.Context) error {
	for _, p := range t.policies {
		if p.RetentionDays <= 0 {
			continue
		}
		var total int64
		for {
			deleted, err := t.store.DeleteOldRecords(ctx, p.Table, p.TimeColumn, p.RetentionDays, deleteBatchSize)
			if err != nil {
				t.log.ErrorContext(ctx, "retention delete failed", "table", p.Table, "error", err)
				break
			}
			total += deleted
			if deleted < deleteBatchSize {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(batchPause):
			}
		}
		t.log.InfoContext(ctx, "retention cleanup completed", "table", p.Table, "deleted_rows", total)
	}
	return nil
}
