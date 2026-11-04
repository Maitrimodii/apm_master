package alert

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

var errNotifierMissing = errors.New("notification service is not configured")

type Service struct {
	store    Repository
	notifier *alertNotifier
	log      *slog.Logger
}

func NewService(store Repository, notifier Notifier, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, notifier: newAlertNotifier(notifier, store, log), log: log}
}

// definition validates the user-editable fields of an alert and parses its
// condition, so invalid rules are rejected when saved, not when evaluated.
func definition(name string, description *string, severity, condition string) (Alert, error) {
	sev, ok := ParseSeverity(severity)
	if !ok {
		return Alert{}, apperr.ValidationError("severity", "must be one of low, medium, high, critical")
	}
	expr, err := ParseExpression(condition)
	if err != nil {
		return Alert{}, apperr.ValidationError("condition", err.Error())
	}
	return Alert{Name: name, Description: description, Severity: sev, Condition: condition, Expression: &expr}, nil
}

func (s *Service) Create(ctx context.Context, orgID, userID uuid.UUID, req CreateAlertRequest) (*AlertResponse, error) {
	alert, err := definition(req.Name, req.Description, req.Severity, req.Condition)
	if err != nil {
		return nil, err
	}
	alert.OrgID, alert.CreatedBy = orgID, userID
	alert.NotificationRoles = req.NotificationRoles
	alert.IsEnabled = req.IsEnabled == nil || *req.IsEnabled

	created, err := s.store.Create(ctx, alert)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	s.log.InfoContext(ctx, "alert created", "org_id", orgID, "alert_id", created.ID, "created_by", userID)
	s.notifier.created(ctx, *created)

	resp := NewAlertResponse(*created)
	return &resp, nil
}

func (s *Service) Get(ctx context.Context, orgID, alertID uuid.UUID) (*AlertResponse, error) {
	alert, err := s.getAlert(ctx, orgID, alertID)
	if err != nil {
		return nil, err
	}
	resp := NewAlertResponse(*alert)
	return &resp, nil
}

func (s *Service) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]AlertResponse, int, error) {
	alerts, total, err := s.store.List(ctx, orgID, limit, offset)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	responses := make([]AlertResponse, 0, len(alerts))
	for _, a := range alerts {
		responses = append(responses, NewAlertResponse(a))
	}
	return responses, total, nil
}

func (s *Service) Update(ctx context.Context, orgID, alertID uuid.UUID, req UpdateAlertRequest) (*AlertResponse, error) {
	alert, err := definition(req.Name, req.Description, req.Severity, req.Condition)
	if err != nil {
		return nil, err
	}
	alert.ID, alert.OrgID = alertID, orgID
	alert.IsEnabled = req.IsEnabled
	alert.NotificationRoles = req.NotificationRoles

	updated, err := s.store.Update(ctx, alert)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if updated == nil {
		return nil, apperr.NotFound(apperr.CodeNotFound, "alert not found")
	}
	resp := NewAlertResponse(*updated)
	return &resp, nil
}

func (s *Service) Delete(ctx context.Context, orgID, alertID uuid.UUID) error {
	deleted, err := s.store.Delete(ctx, orgID, alertID)
	if err != nil {
		return apperr.Internal(err)
	}
	if !deleted {
		return apperr.NotFound(apperr.CodeNotFound, "alert not found")
	}
	return nil
}

func (s *Service) Share(ctx context.Context, orgID, userID, alertID uuid.UUID, req ShareAlertRequest) (*ShareAlertResponse, error) {
	if !s.notifier.enabled() {
		return nil, apperr.Internal(errNotifierMissing)
	}
	alert, err := s.getAlert(ctx, orgID, alertID)
	if err != nil {
		return nil, err
	}
	var recipients []string
	for _, email := range req.Emails {
		addr := strings.TrimSpace(email)
		if addr == "" {
			continue
		}
		if _, err := mail.ParseAddress(addr); err != nil {
			return nil, apperr.ValidationError("emails", "invalid recipient email: "+addr)
		}
		recipients = append(recipients, addr)
	}
	if len(recipients) == 0 {
		return nil, apperr.ValidationError("emails", "at least one recipient is required")
	}

	message := ""
	if req.Message != nil {
		message = *req.Message
	}
	s.notifier.shared(ctx, *alert, userID, recipients, message)
	s.log.InfoContext(ctx, "alert shared", "org_id", orgID, "alert_id", alert.ID, "shared_by", userID, "recipients", len(recipients))
	return &ShareAlertResponse{Message: "Alert shared successfully"}, nil
}

// Acknowledge acknowledges the currently active instance of an alert.
func (s *Service) Acknowledge(ctx context.Context, orgID, alertID uuid.UUID) (*AlertInstanceResponse, error) {
	alert, err := s.getAlert(ctx, orgID, alertID)
	if err != nil {
		return nil, err
	}
	inst, err := s.store.GetActiveInstance(ctx, alertID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if inst == nil {
		return nil, apperr.NotFound(apperr.CodeNotFound, "no active alert instance found to acknowledge")
	}
	return s.acknowledge(ctx, *alert, *inst)
}

// AcknowledgeInstance acknowledges one alert instance by ID.
func (s *Service) AcknowledgeInstance(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceResponse, error) {
	inst, err := s.store.GetInstance(ctx, orgID, instanceID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if inst == nil {
		return nil, apperr.NotFound(apperr.CodeNotFound, "alert instance not found")
	}
	alert, err := s.getAlert(ctx, orgID, inst.AlertID)
	if err != nil {
		return nil, err
	}
	return s.acknowledge(ctx, *alert, *inst)
}

// acknowledge moves an instance to ACKNOWLEDGED. It is idempotent, and a
// resolved instance cannot be acknowledged.
func (s *Service) acknowledge(ctx context.Context, alert Alert, inst AlertInstance) (*AlertInstanceResponse, error) {
	switch inst.State {
	case AlertStateResolved:
		return nil, apperr.Conflict(apperr.CodeConflict, "cannot acknowledge a resolved alert instance")
	case AlertStateAcknowledged:
		resp := NewAlertInstanceResponse(inst)
		return &resp, nil
	}

	now := time.Now().UTC()
	inst.State = AlertStateAcknowledged
	inst.AcknowledgedAt = &now
	inst.UpdatedAt = now
	if err := s.store.SaveInstance(ctx, inst); err != nil {
		return nil, apperr.Internal(err)
	}
	s.log.InfoContext(ctx, "alert instance acknowledged", "org_id", alert.OrgID, "alert_id", alert.ID, "instance_id", inst.ID)
	s.notifier.transition(ctx, alert, inst, notification.EventAlertAcknowledged)

	resp := NewAlertInstanceResponse(inst)
	return &resp, nil
}

func (s *Service) ListInstances(ctx context.Context, filter InstanceFilter) ([]AlertInstanceResponse, int, error) {
	instances, total, err := s.store.ListInstances(ctx, filter)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	responses := make([]AlertInstanceResponse, 0, len(instances))
	for _, inst := range instances {
		responses = append(responses, NewAlertInstanceResponse(inst))
	}
	return responses, total, nil
}

func (s *Service) GetInstanceDetails(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceDetailsResponse, error) {
	details, err := s.store.GetInstanceDetails(ctx, orgID, instanceID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if details == nil {
		return nil, apperr.NotFound(apperr.CodeNotFound, "alert instance not found")
	}
	return details, nil
}

func (s *Service) getAlert(ctx context.Context, orgID, alertID uuid.UUID) (*Alert, error) {
	alert, err := s.store.Get(ctx, orgID, alertID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if alert == nil {
		return nil, apperr.NotFound(apperr.CodeNotFound, "alert not found")
	}
	return alert, nil
}
