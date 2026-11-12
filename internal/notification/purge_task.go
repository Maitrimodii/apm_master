package notification

import (
	"context"
	"log/slog"
	"time"
)

// payloadPurgeInterval is how often expired dead-letter payloads are erased.
const payloadPurgeInterval = time.Hour

// PayloadPurgeTask erases the encrypted payload of dead-lettered jobs once
// their recovery window has passed. It implements scheduler.Task. The update
// is idempotent, so concurrent runs on several schedulers are harmless.
type PayloadPurgeTask struct {
	store    *Store
	recovery time.Duration
	log      *slog.Logger
}

func NewPayloadPurgeTask(store *Store, recovery time.Duration, log *slog.Logger) *PayloadPurgeTask {
	if log == nil {
		log = slog.Default()
	}
	return &PayloadPurgeTask{store: store, recovery: recovery, log: log}
}

func (t *PayloadPurgeTask) Name() string            { return "notification_payload_purge" }
func (t *PayloadPurgeTask) Interval() time.Duration { return payloadPurgeInterval }

func (t *PayloadPurgeTask) Execute(ctx context.Context) error {
	purged, err := t.store.PurgeDeadLetterPayloads(ctx, t.recovery)
	if err != nil {
		return err
	}
	if purged > 0 {
		t.log.InfoContext(ctx, "erased expired dead-letter notification payloads", "count", purged, "recovery_window", t.recovery.String())
	}
	return nil
}

// staleRecoveryInterval is how often stuck 'processing' jobs are looked for.
const staleRecoveryInterval = 5 * time.Minute

// StaleJobRecoveryTask requeues jobs whose worker died mid-send (see
// Store.RecoverStaleProcessing). It implements scheduler.Task. The update
// skips locked rows and is idempotent, so concurrent runs are harmless.
type StaleJobRecoveryTask struct {
	store      *Store
	staleAfter time.Duration
	log        *slog.Logger
}

func NewStaleJobRecoveryTask(store *Store, staleAfter time.Duration, log *slog.Logger) *StaleJobRecoveryTask {
	if log == nil {
		log = slog.Default()
	}
	return &StaleJobRecoveryTask{store: store, staleAfter: staleAfter, log: log}
}

func (t *StaleJobRecoveryTask) Name() string            { return "notification_stale_job_recovery" }
func (t *StaleJobRecoveryTask) Interval() time.Duration { return staleRecoveryInterval }

func (t *StaleJobRecoveryTask) Execute(ctx context.Context) error {
	requeued, deadLettered, err := t.store.RecoverStaleProcessing(ctx, t.staleAfter)
	if err != nil {
		return err
	}
	if requeued+deadLettered > 0 {
		t.log.WarnContext(ctx, "recovered notification jobs stuck in processing",
			"requeued", requeued, "dead_lettered", deadLettered, "stale_after", t.staleAfter.String())
	}
	return nil
}
