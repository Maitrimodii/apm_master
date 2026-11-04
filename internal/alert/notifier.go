package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/notification"
)

// alertNotifier records in-app events and queues emails for alert activity.
// Notification failures are logged, never returned: an alert transition must
// not fail because an email could not be queued.
type alertNotifier struct {
	notifier   Notifier
	recipients RecipientResolver
	log        *slog.Logger
}

func newAlertNotifier(n Notifier, recipients RecipientResolver, log *slog.Logger) *alertNotifier {
	return &alertNotifier{notifier: n, recipients: recipients, log: log}
}

func (an *alertNotifier) enabled() bool { return an.notifier != nil }

// created records that a user created an alert.
func (an *alertNotifier) created(ctx context.Context, alert Alert) {
	if an.notifier == nil {
		return
	}
	an.recordEvent(ctx, alert, notification.EventAlertCreated, "Alert created",
		fmt.Sprintf("Alert %q was created with %s severity.", alert.Name, alert.Severity),
		map[string]string{"alert_id": alert.ID.String()})
}

// shared emails an alert definition to the given addresses.
func (an *alertNotifier) shared(ctx context.Context, alert Alert, sharedBy uuid.UUID, emails []string, message string) {
	body := fmt.Sprintf("Alert: %s\nSeverity: %s\nCondition: %s\nEnabled: %t", alert.Name, alert.Severity, alert.Condition, alert.IsEnabled)
	description := ""
	if alert.Description != nil {
		description = *alert.Description
	}
	if description != "" {
		body += "\nDescription: " + description
	}
	if message != "" {
		body += "\n\nMessage:\n" + message
	}
	for _, email := range emails {
		err := an.notifier.EnqueueHighPriorityEmail(ctx, notification.Task{
			OrgID:       alert.OrgID,
			UserID:      &sharedBy,
			EventType:   notification.EventAlertShared,
			Recipient:   email,
			Subject:     fmt.Sprintf("APM Master alert: %s", alert.Name),
			Body:        body,
			ReferenceID: alert.ID.String(),
			Metadata: map[string]string{
				"alert_id":       alert.ID.String(),
				"user_id":        sharedBy.String(),
				"alert_name":     alert.Name,
				"severity":       string(alert.Severity),
				"condition":      alert.Condition,
				"is_enabled":     fmt.Sprintf("%t", alert.IsEnabled),
				"description":    description,
				"custom_message": message,
			},
		})
		if err != nil {
			an.log.WarnContext(ctx, "alert share email enqueue failed", "alert_id", alert.ID, "error", err)
		}
	}
}

// transition notifies the alert owner in-app and emails everyone in the
// alert's notification roles that an instance fired, resolved or was acknowledged.
func (an *alertNotifier) transition(ctx context.Context, alert Alert, inst AlertInstance, event notification.EventType) {
	if an.notifier == nil {
		return
	}
	subject, body := transitionText(alert, inst, event)
	an.recordEvent(ctx, alert, event, subject, body, map[string]string{
		"alert_id":    alert.ID.String(),
		"instance_id": inst.ID.String(),
	})

	recipients, err := an.recipients.ResolveUsersByRoles(ctx, alert.OrgID, alert.NotificationRoles)
	if err != nil {
		an.log.ErrorContext(ctx, "resolve alert notification recipients failed", "alert_id", alert.ID, "error", err)
		return
	}
	for _, r := range recipients {
		err := an.notifier.EnqueueHighPriorityEmail(ctx, notification.Task{
			OrgID:       alert.OrgID,
			UserID:      &r.ID,
			EventType:   event,
			Recipient:   r.Email,
			Subject:     subject,
			Body:        body,
			ReferenceID: fmt.Sprintf("%s:%s", inst.ID, event),
			Metadata: map[string]string{
				"alert_id":    alert.ID.String(),
				"instance_id": inst.ID.String(),
				"alert_name":  alert.Name,
				"value":       fmt.Sprintf("%.2f", inst.Value),
				"threshold":   fmt.Sprintf("%.2f", inst.Threshold),
				"state":       string(inst.State),
				"severity":    string(alert.Severity),
				"condition":   alert.Condition,
			},
		})
		if err != nil {
			an.log.ErrorContext(ctx, "alert email enqueue failed", "alert_id", alert.ID, "user_id", r.ID, "error", err)
		}
	}
}

func transitionText(alert Alert, inst AlertInstance, event notification.EventType) (subject, body string) {
	switch event {
	case notification.EventAlertResolved:
		return "Resolved: " + alert.Name,
			fmt.Sprintf("Alert %q has resolved. Current value %.2f, threshold %.2f.", alert.Name, inst.Value, inst.Threshold)
	case notification.EventAlertAcknowledged:
		return "Acknowledged: " + alert.Name,
			fmt.Sprintf("Alert %q has been acknowledged. Current value %.2f, threshold %.2f.", alert.Name, inst.Value, inst.Threshold)
	default:
		return "Alert " + alert.Name,
			fmt.Sprintf("Alert %q is %s. Current value %.2f, threshold %.2f. %s", alert.Name, inst.State, inst.Value, inst.Threshold, inst.Message)
	}
}

func (an *alertNotifier) recordEvent(ctx context.Context, alert Alert, event notification.EventType, subject, body string, meta map[string]string) {
	metadata, err := json.Marshal(meta)
	if err != nil {
		an.log.ErrorContext(ctx, "encode alert event metadata failed", "alert_id", alert.ID, "error", err)
		return
	}
	_, err = an.notifier.RecordEvent(ctx, notification.Event{
		OrgID:     alert.OrgID,
		UserID:    &alert.CreatedBy,
		EventType: event,
		Channel:   notification.ChannelInApp,
		Recipient: alert.CreatedBy.String(),
		Subject:   subject,
		Body:      body,
		Status:    notification.StatusPending,
		Metadata:  metadata,
	})
	if err != nil {
		an.log.ErrorContext(ctx, "record alert event failed", "alert_id", alert.ID, "event", event, "error", err)
	}
}
