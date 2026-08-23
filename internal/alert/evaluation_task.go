package alert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
)

// EvaluationTask evaluates enabled alerts on a fixed cadence.
// It implements the scheduler.Task interface.
type EvaluationTask struct {
	store           AlertStoreInterface
	metricsStore    metric.MetricStoreInterface
	tracesStore     trace.TraceStoreInterface
	logsStore       log.LogStoreInterface
	notification    *notification.Service
	log             *slog.Logger
	interval        time.Duration
	advisoryLockKey int64
}

// NewEvaluationTask creates a new alert evaluation task.
func NewEvaluationTask(
	store AlertStoreInterface,
	metricsStore metric.MetricStoreInterface,
	tracesStore trace.TraceStoreInterface,
	logsStore log.LogStoreInterface,
	notificationService *notification.Service,
	cfg *config.Config,
	logger *slog.Logger,
) *EvaluationTask {
	if logger == nil {
		logger = slog.Default()
	}
	interval := time.Minute
	if cfg != nil && cfg.AlertEvaluationInterval > 0 {
		interval = cfg.AlertEvaluationInterval
	}

	return &EvaluationTask{
		store:           store,
		metricsStore:    metricsStore,
		tracesStore:     tracesStore,
		logsStore:       logsStore,
		notification:    notificationService,
		log:             logger,
		interval:        interval,
		advisoryLockKey: 0xA1A2A3A4,
	}
}

// Name returns the unique name for this task.
func (t *EvaluationTask) Name() string {
	return "alert_evaluation"
}

// Interval returns how often this task should run.
func (t *EvaluationTask) Interval() time.Duration {
	return t.interval
}

// Execute performs a single run of alert evaluation.
func (t *EvaluationTask) Execute(ctx context.Context) error {
	if !t.tryAcquireLock(ctx) {
		t.log.Debug("evaluation skipped, lock held by another instance")
		return nil
	}
	defer t.releaseLock(ctx)

	limit := 500
	offset := 0

	for {
		alerts, err := t.store.ListEnabledPage(ctx, limit, offset)
		if err != nil {
			return fmt.Errorf("list enabled alerts page: %w", err)
		}

		for _, alert := range alerts {
			if err := t.evaluateAlert(ctx, alert); err != nil {
				t.log.Error("alert evaluation failed", "alert_id", alert.ID, "name", alert.Name, "error", err)
			}
		}

		if len(alerts) < limit {
			break
		}
		offset += limit
	}

	return nil
}

func (t *EvaluationTask) tryAcquireLock(ctx context.Context) bool {
	if t.store == nil {
		return true
	}

	acquired, err := t.store.AcquireLock(ctx, t.advisoryLockKey)
	if err != nil {
		t.log.Warn("advisory lock probe failed", "error", err)
		return false
	}
	return acquired
}

func (t *EvaluationTask) releaseLock(ctx context.Context) {
	if t.store == nil {
		return
	}
	_ = t.store.ReleaseLock(ctx, t.advisoryLockKey)
}

// evaluateAlert evaluates a single alert, exporting it here for testing if needed.
func (t *EvaluationTask) evaluateAlert(ctx context.Context, alert Alert) error {
	evalCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	evaluator := t.evaluatorFor(alert)
	result, err := evaluator.Evaluate(evalCtx, alert)
	if err != nil {
		return err
	}

	if !result.Fired {
		return t.resolveIfNeeded(evalCtx, alert)
	}

	return t.fireIfNeeded(evalCtx, alert, result)
}

func (t *EvaluationTask) evaluatorFor(alert Alert) Evaluator {
	return NewCompoundEvaluator(t.metricsStore, t.logsStore, t.tracesStore, t.log)
}

func (t *EvaluationTask) fireIfNeeded(ctx context.Context, alert Alert, result *EvaluationResult) error {
	instance, err := t.store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		return err
	}

	cond, err := parseCondition(alert.Condition)
	if err != nil {
		return err
	}
	window := cond.window

	if instance == nil {
		now := time.Now().UTC()
		
		state := string(AlertStatePending)
		var pendingFiredAt *time.Time
		if window == 0 {
			state = string(AlertStateFiring)
		} else {
			pendingFiredAt = &now
		}

		instance = &AlertInstance{
			ID:               uuid.New(),
			AlertID:          alert.ID,
			State:            state,
			PendingFiredAt:   pendingFiredAt,
			TriggeredAt:      now,
			LastEvaluatedAt:  now,
			Value:            result.Value,
			Threshold:        result.Threshold,
			Message:          result.Message,
			NotificationSent: false,
		}
		if err := t.store.SaveInstance(ctx, *instance); err != nil {
			return err
		}
		
		if state == string(AlertStateFiring) {
			return t.notify(ctx, alert, instance, "alert_firing")
		}
		return nil
	}

	now := time.Now().UTC()
	instance.Value = result.Value
	instance.Threshold = result.Threshold
	instance.Message = result.Message
	instance.LastEvaluatedAt = now

	if instance.State == string(AlertStatePending) {
		if instance.PendingFiredAt != nil && time.Since(*instance.PendingFiredAt) >= window {
			instance.State = string(AlertStateFiring)
			if err := t.store.SaveInstance(ctx, *instance); err != nil {
				return err
			}
			return t.notify(ctx, alert, instance, "alert_firing")
		}
		return t.store.SaveInstance(ctx, *instance)
	}

	instance.NotificationSent = true
	if err := t.store.SaveInstance(ctx, *instance); err != nil {
		return err
	}
	return nil
}

func (t *EvaluationTask) resolveIfNeeded(ctx context.Context, alert Alert) error {
	instance, err := t.store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		return err
	}
	if instance == nil || (instance.State != string(AlertStateFiring) && instance.State != string(AlertStateAcknowledged)) {
		return nil
	}

	resolvedAt := time.Now().UTC()
	instance.State = string(AlertStateResolved)
	instance.ResolvedAt = &resolvedAt
	instance.LastEvaluatedAt = resolvedAt
	instance.Message = "Alert resolved after evaluation returned to normal"
	if err := t.store.SaveInstance(ctx, *instance); err != nil {
		return err
	}
	return t.notify(ctx, alert, instance, "alert_resolved")
}

func (t *EvaluationTask) notify(ctx context.Context, alert Alert, instance *AlertInstance, eventType string) error {
	if t.notification == nil {
		return nil
	}

	subject := fmt.Sprintf("Alert %s", alert.Name)
	body := fmt.Sprintf("Alert %q is %s. Current value %.2f, threshold %.2f. %s", alert.Name, instance.State, instance.Value, instance.Threshold, instance.Message)

	if eventType == "alert_resolved" {
		subject = fmt.Sprintf("Resolved: %s", alert.Name)
		body = fmt.Sprintf("Alert %q has resolved. Current value %.2f, threshold %.2f.", alert.Name, instance.Value, instance.Threshold)
	} else if eventType == "alert_acknowledged" {
		subject = fmt.Sprintf("Acknowledged: %s", alert.Name)
		body = fmt.Sprintf("Alert %q has been acknowledged. Current value %.2f, threshold %.2f.", alert.Name, instance.Value, instance.Threshold)
	}

	_, err := t.notification.RecordEvent(ctx, notification.Event{
		OrgID:     alert.OrgID,
		UserID:    &alert.CreatedBy,
		EventType: eventType,
		Channel:   notification.ChannelInApp,
		Recipient: alert.CreatedBy.String(),
		Subject:   subject,
		Body:      body,
		Status:    notification.StatusPending,
		Metadata:  []byte(fmt.Sprintf(`{"alert_id":"%s","instance_id":"%s"}`, alert.ID.String(), instance.ID.String())),
	})
	if err != nil {
		t.log.Error("record alert event failed", "alert_id", alert.ID, "error", err)
	}

	// Resolve notification roles and enqueue email jobs
	resolvedUsers, resolveErr := t.store.ResolveUsersByRoles(ctx, alert.OrgID, alert.NotificationRoles)
	if resolveErr != nil {
		t.log.Error("failed to resolve notification roles for email notification", "alert_id", alert.ID, "roles", alert.NotificationRoles, "error", resolveErr)
	} else {
		for _, resolvedUser := range resolvedUsers {
			enqueueErr := t.notification.EnqueueHighPriorityEmail(ctx, notification.NotificationTask{
				OrgID:       alert.OrgID,
				UserID:      &resolvedUser.ID,
				EventType:   eventType,
				Recipient:   resolvedUser.Email,
				Subject:     subject,
				Body:        body,
				ReferenceID: fmt.Sprintf("%s:%s", instance.ID.String(), eventType),
				Metadata: map[string]string{
					"alert_id":    alert.ID.String(),
					"instance_id": instance.ID.String(),
					"alert_name":  alert.Name,
					"value":       fmt.Sprintf("%.2f", instance.Value),
					"threshold":   fmt.Sprintf("%.2f", instance.Threshold),
					"state":       string(instance.State),
					"severity":    alert.Severity,
					"condition":   alert.Condition,
				},
			})
			if enqueueErr != nil {
				t.log.Error("failed to enqueue email notification", "alert_id", alert.ID, "recipient", resolvedUser.Email, "error", enqueueErr)
			}
		}
	}

	return err
}
