package alert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/notification"
)

// LockFunc runs fn while holding a cluster-wide lock and reports whether it
// ran. It lets only one instance evaluate alerts at a time.
type LockFunc func(ctx context.Context, fn func(ctx context.Context) error) (bool, error)

// EvaluationTaskDeps are the EvaluationTask's collaborators. Metrics, Logs
// and Traces read telemetry; Lock may be nil to skip cluster locking.
type EvaluationTaskDeps struct {
	Store    Repository
	Metrics  metricReader
	Logs     logReader
	Traces   traceReader
	Notifier Notifier
	Lock     LockFunc
	Interval time.Duration
	Logger   *slog.Logger
}

// EvaluationTask evaluates every enabled alert on a fixed cadence. It
// implements scheduler.Task.
type EvaluationTask struct {
	store     Repository
	evaluator *ExpressionEvaluator
	notifier  *alertNotifier
	lock      LockFunc
	interval  time.Duration
	log       *slog.Logger
}

func NewEvaluationTask(deps EvaluationTaskDeps) *EvaluationTask {
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	interval := deps.Interval
	if interval <= 0 {
		interval = DefaultEvaluationInterval
	}
	return &EvaluationTask{
		store: deps.Store,
		evaluator: NewExpressionEvaluator(
			NewMetricEvaluator(deps.Metrics, log),
			NewLogEvaluator(deps.Logs, log),
			NewTraceEvaluator(deps.Traces, log),
		),
		notifier: newAlertNotifier(deps.Notifier, deps.Store, log),
		lock:     deps.Lock,
		interval: interval,
		log:      log,
	}
}

func (t *EvaluationTask) Name() string            { return "alert_evaluation" }
func (t *EvaluationTask) Interval() time.Duration { return t.interval }

// Execute evaluates all enabled alerts once.
func (t *EvaluationTask) Execute(ctx context.Context) error {
	if t.lock == nil {
		return t.evaluateAll(ctx)
	}
	ran, err := t.lock(ctx, t.evaluateAll)
	if !ran && err == nil {
		t.log.DebugContext(ctx, "alert evaluation skipped, another instance holds the lock")
	}
	return err
}

func (t *EvaluationTask) evaluateAll(ctx context.Context) error {
	for offset := 0; ; offset += enabledAlertsPageSize {
		alerts, err := t.store.ListEnabledPage(ctx, enabledAlertsPageSize, offset)
		if err != nil {
			return fmt.Errorf("list enabled alerts page: %w", err)
		}
		for _, alert := range alerts {
			if err := t.evaluateAlert(ctx, alert); err != nil {
				t.log.ErrorContext(ctx, "alert evaluation failed", "alert_id", alert.ID, "name", alert.Name, "error", err)
			}
		}
		if len(alerts) < enabledAlertsPageSize {
			return nil
		}
	}
}

func (t *EvaluationTask) evaluateAlert(ctx context.Context, alert Alert) error {
	ctx, cancel := context.WithTimeout(ctx, evaluationTimeout)
	defer cancel()

	expr, err := alert.ParsedCondition()
	if err != nil {
		return err
	}
	result, err := t.evaluator.Evaluate(ctx, alert)
	if err != nil {
		return err
	}
	if !result.Fired {
		return t.resolveIfNeeded(ctx, alert)
	}
	return t.fireIfNeeded(ctx, alert, expr.Window(), result)
}

// fireIfNeeded opens a PENDING instance on the first breach and promotes it
// to FIRING once the breach has lasted the expression's window.
func (t *EvaluationTask) fireIfNeeded(ctx context.Context, alert Alert, window time.Duration, result *EvaluationResult) error {
	inst, err := t.store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	if inst == nil {
		inst = &AlertInstance{
			ID:              uuid.New(),
			AlertID:         alert.ID,
			State:           AlertStatePending,
			PendingFiredAt:  &now,
			TriggeredAt:     now,
			LastEvaluatedAt: now,
			Value:           result.Value,
			Threshold:       result.Threshold,
			Message:         result.Message,
		}
		if window == 0 {
			inst.State, inst.PendingFiredAt = AlertStateFiring, nil
		}
		if err := t.store.SaveInstance(ctx, *inst); err != nil {
			return err
		}
		if inst.State == AlertStateFiring {
			t.notifier.transition(ctx, alert, *inst, notification.EventAlertFiring)
		}
		return nil
	}

	inst.Value, inst.Threshold, inst.Message = result.Value, result.Threshold, result.Message
	inst.LastEvaluatedAt = now

	if inst.State == AlertStatePending {
		if inst.PendingFiredAt == nil || time.Since(*inst.PendingFiredAt) < window {
			return t.store.SaveInstance(ctx, *inst)
		}
		inst.State = AlertStateFiring
		if err := t.store.SaveInstance(ctx, *inst); err != nil {
			return err
		}
		t.notifier.transition(ctx, alert, *inst, notification.EventAlertFiring)
		return nil
	}

	inst.NotificationSent = true
	return t.store.SaveInstance(ctx, *inst)
}

// resolveIfNeeded resolves a FIRING or ACKNOWLEDGED instance once the
// condition no longer holds. A PENDING instance that recovers is left to be
// promoted or superseded on later evaluations.
func (t *EvaluationTask) resolveIfNeeded(ctx context.Context, alert Alert) error {
	inst, err := t.store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		return err
	}
	if inst == nil || (inst.State != AlertStateFiring && inst.State != AlertStateAcknowledged) {
		return nil
	}
	now := time.Now().UTC()
	inst.State = AlertStateResolved
	inst.ResolvedAt = &now
	inst.LastEvaluatedAt = now
	inst.Message = "Alert resolved after evaluation returned to normal"
	if err := t.store.SaveInstance(ctx, *inst); err != nil {
		return err
	}
	t.notifier.transition(ctx, alert, *inst, notification.EventAlertResolved)
	return nil
}
