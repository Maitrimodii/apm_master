package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	store        AlertStoreInterface
	notification *notification.Service
	log          *slog.Logger
}

func NewService(store AlertStoreInterface, notificationService *notification.Service, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}

	return &Service{store: store, notification: notificationService, log: log}
}

func (service *Service) Create(ctx context.Context, orgID, userID uuid.UUID, req CreateAlertRequest) (*AlertResponse, error) {
	if err := validateSeverity(req.Severity); err != nil {
		return nil, err
	}

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}

	alert, err := service.store.Create(ctx, Alert{
		OrgID:             orgID,
		CreatedBy:         userID,
		Name:              req.Name,
		Description:       req.Description,
		Severity:          strings.ToLower(req.Severity),
		Condition:         req.Condition,
		IsEnabled:         isEnabled,
		NotificationRoles: req.NotificationRoles,
	})
	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("alert created", "org_id", orgID, "alert_id", alert.ID, "created_by", userID)
	if service.notification != nil {
		_, err := service.notification.RecordEvent(ctx, notification.Event{
			OrgID:     orgID,
			UserID:    &userID,
			EventType: "alert_created",
			Channel:   notification.ChannelInApp,
			Recipient: userID.String(),
			Subject:   "Alert created",
			Body:      fmt.Sprintf("Alert %q was created with %s severity.", alert.Name, alert.Severity),
			Status:    notification.StatusPending,
			Metadata:  []byte(fmt.Sprintf(`{"alert_id":"%s"}`, alert.ID.String())),
		})
		if err != nil {
			service.log.Error("record alert notification event failed", "org_id", orgID, "alert_id", alert.ID, "error", err)
		}
	}

	resp := NewAlertResponse(*alert)
	return &resp, nil
}

func (service *Service) Get(ctx context.Context, orgID, alertID uuid.UUID) (*AlertResponse, error) {
	alert, err := service.store.Get(ctx, orgID, alertID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if alert == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert not found")
	}

	resp := NewAlertResponse(*alert)
	return &resp, nil
}

func (service *Service) List(ctx context.Context, orgID uuid.UUID, page, limit int) ([]AlertResponse, int, error) {
	alerts, total, err := service.store.List(ctx, orgID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, server.Internal(err)
	}

	responses := make([]AlertResponse, 0, len(alerts))
	for _, alert := range alerts {
		responses = append(responses, NewAlertResponse(alert))
	}

	return responses, total, nil
}

func (service *Service) Update(ctx context.Context, orgID, alertID uuid.UUID, req UpdateAlertRequest) (*AlertResponse, error) {
	if err := validateSeverity(req.Severity); err != nil {
		return nil, err
	}

	alert, err := service.store.Update(ctx, Alert{
		ID:                alertID,
		OrgID:             orgID,
		Name:              req.Name,
		Description:       req.Description,
		Severity:          strings.ToLower(req.Severity),
		Condition:         req.Condition,
		IsEnabled:         req.IsEnabled,
		NotificationRoles: req.NotificationRoles,
	})
	if err != nil {
		return nil, server.Internal(err)
	}
	if alert == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert not found")
	}

	resp := NewAlertResponse(*alert)
	return &resp, nil
}

func (service *Service) Delete(ctx context.Context, orgID, alertID uuid.UUID) error {
	if err := service.store.Delete(ctx, orgID, alertID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return server.NotFound(server.CodeNotFound, "alert not found")
		}
		return server.Internal(err)
	}
	return nil
}

func (service *Service) Share(ctx context.Context, orgID, userID, alertID uuid.UUID, req ShareAlertRequest) (*ShareAlertResponse, error) {
	alert, err := service.store.Get(ctx, orgID, alertID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if alert == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert not found")
	}
	if len(req.Emails) == 0 {
		return nil, server.BadRequest(server.CodeValidation, "emails are required")
	}
	if service.notification == nil {
		return nil, server.Internal(fmt.Errorf("notification service is not configured"))
	}

	body := fmt.Sprintf("Alert: %s\nSeverity: %s\nCondition: %s\nEnabled: %t", alert.Name, alert.Severity, alert.Condition, alert.IsEnabled)
	if alert.Description != nil && *alert.Description != "" {
		body += "\nDescription: " + *alert.Description
	}
	if req.Message != nil && *req.Message != "" {
		body += "\n\nMessage:\n" + *req.Message
	}

	for _, email := range req.Emails {
		recipient := strings.TrimSpace(email)
		if recipient == "" {
			continue
		}
		if _, err := mail.ParseAddress(recipient); err != nil {
			return nil, server.BadRequest(server.CodeValidation, "invalid recipient email")
		}

		desc := ""
		if alert.Description != nil {
			desc = *alert.Description
		}
		msgVal := ""
		if req.Message != nil {
			msgVal = *req.Message
		}

		err := service.notification.EnqueueHighPriorityEmail(ctx, notification.NotificationTask{
			OrgID:       orgID,
			UserID:      &userID,
			EventType:   "alert_shared",
			Recipient:   recipient,
			Subject:     fmt.Sprintf("APM Master alert: %s", alert.Name),
			Body:        body,
			ReferenceID: alert.ID.String(),
			Metadata: map[string]string{
				"alert_id":       alert.ID.String(),
				"user_id":        userID.String(),
				"alert_name":     alert.Name,
				"severity":       alert.Severity,
				"condition":      alert.Condition,
				"is_enabled":     fmt.Sprintf("%t", alert.IsEnabled),
				"description":    desc,
				"custom_message": msgVal,
			},
		})
		if err != nil {
			service.log.Warn("alert notification enqueue failed", "org_id", orgID, "alert_id", alert.ID, "recipient", recipient, "error", err)
		}
	}

	service.log.Info("alert shared", "org_id", orgID, "alert_id", alert.ID, "shared_by", userID, "recipients", len(req.Emails))
	return &ShareAlertResponse{Message: "Alert shared successfully"}, nil
}

func (service *Service) Acknowledge(ctx context.Context, orgID, alertID uuid.UUID) (*AlertInstanceResponse, error) {
	alert, err := service.store.Get(ctx, orgID, alertID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if alert == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert not found")
	}

	instance, err := service.store.GetActiveInstance(ctx, alertID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if instance == nil {
		return nil, server.NotFound(server.CodeNotFound, "no active alert instance found to acknowledge")
	}

	if instance.State == string(AlertStateAcknowledged) {
		resp := NewAlertInstanceResponse(*instance)
		return &resp, nil
	}

	now := time.Now().UTC()
	instance.State = string(AlertStateAcknowledged)
	instance.AcknowledgedAt = &now
	instance.UpdatedAt = now

	if err := service.store.SaveInstance(ctx, *instance); err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("alert instance acknowledged", "org_id", orgID, "alert_id", alertID, "instance_id", instance.ID)

	if service.notification != nil {
		subject := fmt.Sprintf("Acknowledged: %s", alert.Name)
		body := fmt.Sprintf("Alert %q has been acknowledged. Current value %.2f, threshold %.2f.", alert.Name, instance.Value, instance.Threshold)

		_, err := service.notification.RecordEvent(ctx, notification.Event{
			OrgID:     orgID,
			UserID:    &alert.CreatedBy,
			EventType: "alert_acknowledged",
			Channel:   notification.ChannelInApp,
			Recipient: alert.CreatedBy.String(),
			Subject:   subject,
			Body:      body,
			Status:    notification.StatusPending,
			Metadata:  []byte(fmt.Sprintf(`{"alert_id":"%s","instance_id":"%s"}`, alert.ID.String(), instance.ID.String())),
		})
		if err != nil {
			service.log.Error("record alert acknowledged notification event failed", "org_id", orgID, "alert_id", alert.ID, "error", err)
		}

		// Resolve notification roles and enqueue email jobs
		resolvedUsers, err := service.store.ResolveUsersByRoles(ctx, orgID, alert.NotificationRoles)
		if err != nil {
			service.log.Error("failed to resolve notification roles for alert acknowledgment email", "org_id", orgID, "alert_id", alert.ID, "roles", alert.NotificationRoles, "error", err)
		} else {
			for _, resolvedUser := range resolvedUsers {
				enqueueErr := service.notification.EnqueueHighPriorityEmail(ctx, notification.NotificationTask{
					OrgID:       orgID,
					UserID:      &resolvedUser.ID,
					EventType:   "alert_acknowledged",
					Recipient:   resolvedUser.Email,
					Subject:     subject,
					Body:        body,
					ReferenceID: fmt.Sprintf("%s:alert_acknowledged", instance.ID.String()),
					Metadata: map[string]string{
						"alert_id":    alert.ID.String(),
						"instance_id": instance.ID.String(),
						"alert_name":  alert.Name,
						"value":       fmt.Sprintf("%.2f", instance.Value),
						"threshold":   fmt.Sprintf("%.2f", instance.Threshold),
						"state":       string(instance.State),
					},
				})
				if enqueueErr != nil {
					service.log.Error("failed to enqueue acknowledgment email notification", "alert_id", alert.ID, "recipient", resolvedUser.Email, "error", enqueueErr)
				}
			}
		}
	}

	resp := NewAlertInstanceResponse(*instance)
	return &resp, nil
}

func validateSeverity(severity string) error {
	switch strings.ToLower(severity) {
	case "low", "medium", "high", "critical":
		return nil
	default:
		return server.BadRequest(server.CodeValidation, "severity must be one of low, medium, high, critical")
	}
}

func (service *Service) ListInstances(ctx context.Context, orgID uuid.UUID, state, severity string, alertID uuid.UUID, from, timeout *time.Time, page, pageSize int) ([]AlertInstanceResponse, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	instances, total, err := service.store.ListInstances(ctx, orgID, state, severity, alertID, from, timeout, pageSize, offset)
	if err != nil {
		return nil, 0, server.Internal(err)
	}

	responses := make([]AlertInstanceResponse, 0, len(instances))
	for _, inst := range instances {
		responses = append(responses, NewAlertInstanceResponse(inst))
	}

	return responses, total, nil
}

func (service *Service) GetInstanceDetails(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceDetailsResponse, error) {
	details, err := service.store.GetInstanceDetails(ctx, orgID, instanceID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if details == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert instance not found")
	}
	return details, nil
}

func (service *Service) AcknowledgeInstance(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceResponse, error) {
	instance, err := service.store.GetInstance(ctx, orgID, instanceID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if instance == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert instance not found")
	}

	if instance.State == string(AlertStateResolved) {
		return nil, server.BadRequest(server.CodeValidation, "cannot acknowledge a resolved alert instance")
	}

	if instance.State == string(AlertStateAcknowledged) {
		resp := NewAlertInstanceResponse(*instance)
		return &resp, nil
	}

	alert, err := service.store.Get(ctx, orgID, instance.AlertID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if alert == nil {
		return nil, server.NotFound(server.CodeNotFound, "alert definition not found")
	}

	now := time.Now().UTC()
	instance.State = string(AlertStateAcknowledged)
	instance.AcknowledgedAt = &now
	instance.UpdatedAt = now

	if err := service.store.SaveInstance(ctx, *instance); err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("alert instance acknowledged", "org_id", orgID, "alert_id", alert.ID, "instance_id", instance.ID)

	if service.notification != nil {
		subject := fmt.Sprintf("Acknowledged: %s", alert.Name)
		body := fmt.Sprintf("Alert %q has been acknowledged. Current value %.2f, threshold %.2f.", alert.Name, instance.Value, instance.Threshold)

		_, err := service.notification.RecordEvent(ctx, notification.Event{
			OrgID:     orgID,
			UserID:    &alert.CreatedBy,
			EventType: "alert_acknowledged",
			Channel:   notification.ChannelInApp,
			Recipient: alert.CreatedBy.String(),
			Subject:   subject,
			Body:      body,
			Status:    notification.StatusPending,
			Metadata:  []byte(fmt.Sprintf(`{"alert_id":"%s","instance_id":"%s"}`, alert.ID.String(), instance.ID.String())),
		})
		if err != nil {
			service.log.Error("record alert acknowledged notification event failed", "org_id", orgID, "alert_id", alert.ID, "error", err)
		}

		// Resolve notification roles and enqueue email jobs
		resolvedUsers, err := service.store.ResolveUsersByRoles(ctx, orgID, alert.NotificationRoles)
		if err != nil {
			service.log.Error("failed to resolve notification roles for alert acknowledgment email", "org_id", orgID, "alert_id", alert.ID, "roles", alert.NotificationRoles, "error", err)
		} else {
			for _, resolvedUser := range resolvedUsers {
				enqueueErr := service.notification.EnqueueHighPriorityEmail(ctx, notification.NotificationTask{
					OrgID:       orgID,
					UserID:      &resolvedUser.ID,
					EventType:   "alert_acknowledged",
					Recipient:   resolvedUser.Email,
					Subject:     subject,
					Body:        body,
					ReferenceID: fmt.Sprintf("%s:alert_acknowledged", instance.ID.String()),
					Metadata: map[string]string{
						"alert_id":    alert.ID.String(),
						"instance_id": instance.ID.String(),
						"alert_name":  alert.Name,
						"value":       fmt.Sprintf("%.2f", instance.Value),
						"threshold":   fmt.Sprintf("%.2f", instance.Threshold),
						"state":       string(instance.State),
					},
				})
				if enqueueErr != nil {
					service.log.Error("failed to enqueue acknowledgment email notification", "alert_id", alert.ID, "recipient", resolvedUser.Email, "error", enqueueErr)
				}
			}
		}
	}

	resp := NewAlertInstanceResponse(*instance)
	return &resp, nil
}
