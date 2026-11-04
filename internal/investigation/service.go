package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/alert"
)

// IncidentReader loads the alert incident an investigation is about.
type IncidentReader interface {
	GetIncident(ctx context.Context, instanceID uuid.UUID) (*alert.Incident, error)
}

type Service struct {
	store     *Store
	incidents IncidentReader
	logger    *slog.Logger
}

func NewService(store *Store, incidents IncidentReader, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, incidents: incidents, logger: logger}
}

// Create creates a new investigation and its outbox event atomically.
func (s *Service) Create(ctx context.Context, orgID, userID, incidentID, correlationID uuid.UUID, req CreateInvestigationRequest) (*Investigation, error) {
	incident, err := s.incidents.GetIncident(ctx, incidentID)
	if err != nil {
		return nil, fmt.Errorf("verify incident: %w", err)
	}
	if incident == nil || incident.Alert.OrgID != orgID {
		return nil, ErrIncidentNotFound
	}

	triggeredAt := incident.Instance.TriggeredAt
	start, end := triggeredAt.Add(-DefaultWindowPadding), triggeredAt.Add(DefaultWindowPadding)
	if req.TimeRangeStart != nil && !req.TimeRangeStart.IsZero() {
		start = *req.TimeRangeStart
	}
	if req.TimeRangeEnd != nil && !req.TimeRangeEnd.IsZero() {
		end = *req.TimeRangeEnd
	}
	if start.After(end) {
		return nil, ErrTimeWindowInvalid
	}
	if end.Sub(start) > MaxTimeWindow {
		return nil, ErrTimeWindowTooLarge
	}

	// The AI worker enforces the per-tenant concurrency cap; here we only log.
	if running, err := s.store.CountRunningByTenant(ctx, orgID); err != nil {
		s.logger.WarnContext(ctx, "count running investigations failed", "error", err)
	} else if running >= MaxConcurrentPerTenantDefault {
		s.logger.WarnContext(ctx, "tenant reached concurrent investigations limit", "org_id", orgID, "count", running)
	}

	focus := req.Focus
	if focus == nil {
		focus = []string{}
	}
	return s.enqueue(ctx, &Investigation{
		ID:             uuid.New(),
		OrgID:          orgID,
		IncidentID:     incidentID,
		RequestedBy:    userID,
		Status:         StatusQueued,
		TimeRangeStart: start.UTC(),
		TimeRangeEnd:   end.UTC(),
		Focus:          focus,
		CorrelationID:  orNew(correlationID),
		IdempotencyKey: req.IdempotencyKey,
	})
}

// Retry queues a new investigation for the same incident and window as a
// finished one.
func (s *Service) Retry(ctx context.Context, orgID, userID, id, correlationID uuid.UUID) (*Investigation, error) {
	existing, err := s.store.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrInvestigationNotFound
	}
	if !TerminalStatuses[existing.Status] {
		return nil, ErrCannotRetryActive
	}
	return s.enqueue(ctx, &Investigation{
		ID:                    uuid.New(),
		OrgID:                 orgID,
		IncidentID:            existing.IncidentID,
		RequestedBy:           userID,
		Status:                StatusQueued,
		TimeRangeStart:        existing.TimeRangeStart,
		TimeRangeEnd:          existing.TimeRangeEnd,
		Focus:                 existing.Focus,
		CorrelationID:         orNew(correlationID),
		ParentInvestigationID: &existing.ID,
	})
}

// enqueue stores inv with an outbox job event in one transaction.
func (s *Service) enqueue(ctx context.Context, inv *Investigation) (*Investigation, error) {
	payload, err := json.Marshal(InvestigationJobEvent{
		EventID:         uuid.New(),
		SchemaVersion:   JobSchemaVersion,
		EventType:       JobEventRequested,
		CorrelationID:   inv.CorrelationID,
		InvestigationID: inv.ID,
		IncidentID:      inv.IncidentID,
		TenantID:        inv.OrgID,
		CreatedAt:       time.Now().UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal job event: %w", err)
	}
	created, err := s.store.CreateWithOutbox(ctx, inv, payload)
	if err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "investigation queued",
		"investigation_id", created.ID, "incident_id", created.IncidentID,
		"tenant_id", created.OrgID, "correlation_id", created.CorrelationID,
		"parent_investigation_id", created.ParentInvestigationID)
	return created, nil
}

func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (*Investigation, error) {
	inv, err := s.store.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, ErrInvestigationNotFound
	}
	return inv, nil
}

func (s *Service) List(ctx context.Context, orgID, incidentID uuid.UUID, limit, offset int) ([]Investigation, int, error) {
	return s.store.List(ctx, orgID, incidentID, limit, offset)
}

// Cancel requests cancellation. Queued investigations cancel immediately; a
// running one is stopped by the worker. Terminal investigations are returned
// unchanged.
func (s *Service) Cancel(ctx context.Context, orgID, userID, id uuid.UUID) (*Investigation, error) {
	existing, err := s.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if TerminalStatuses[existing.Status] {
		return existing, nil
	}
	updated, err := s.store.Cancel(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	s.logger.InfoContext(ctx, "investigation cancellation requested", "investigation_id", id, "tenant_id", orgID, "user_id", userID)
	return updated, nil
}

func (s *Service) GetReport(ctx context.Context, orgID, id uuid.UUID) (*InvestigationReportRecord, error) {
	report, err := s.store.GetReport(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, ErrReportNotReady
	}
	return report, nil
}

func (s *Service) GetEvidence(ctx context.Context, orgID, id, evidenceID uuid.UUID) (*InvestigationEvidence, error) {
	ev, err := s.store.GetEvidence(ctx, orgID, id, evidenceID)
	if err != nil {
		return nil, err
	}
	if ev == nil {
		return nil, ErrEvidenceNotFound
	}
	return ev, nil
}

func (s *Service) ListEvents(ctx context.Context, orgID, id uuid.UUID) ([]InvestigationEvent, error) {
	return s.store.ListEvents(ctx, orgID, id)
}

// --- Internal API used by apm-ai-service ---------------------------------

// IncidentContext is the incident summary handed to the AI investigator.
func (s *Service) IncidentContext(ctx context.Context, incidentID uuid.UUID) (*IncidentContext, error) {
	inc, err := s.incidents.GetIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	if inc == nil {
		return nil, ErrIncidentNotFound
	}
	ic := &IncidentContext{
		IncidentID:  inc.Instance.ID,
		AlertID:     inc.Alert.ID,
		OrgID:       inc.Alert.OrgID,
		AlertName:   inc.Alert.Name,
		Severity:    string(inc.Alert.Severity),
		Condition:   inc.Alert.Condition,
		ServiceName: inc.ServiceName,
		TriggeredAt: inc.Instance.TriggeredAt,
		ResolvedAt:  inc.Instance.ResolvedAt,
		Value:       inc.Instance.Value,
		Threshold:   inc.Instance.Threshold,
		Message:     inc.Instance.Message,
	}
	if expr, err := inc.Alert.ParsedCondition(); err == nil {
		ic.ParsedCondition = &expr
	} else {
		s.logger.WarnContext(ctx, "incident condition does not parse", "alert_id", inc.Alert.ID, "error", err)
	}
	return ic, nil
}

// ApplyStatus moves an investigation to status through the state machine.
// Re-applying the current status is a no-op; an illegal move is
// ErrInvalidTransition.
func (s *Service) ApplyStatus(ctx context.Context, id uuid.UUID, status InvestigationStatus, failureCode, failureMessage *string) error {
	inv, err := s.store.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if inv == nil {
		return ErrInvestigationNotFound
	}
	if inv.Status == status {
		return nil
	}
	if !CanTransition(inv.Status, status) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, inv.Status, status)
	}
	return s.store.UpdateStatus(ctx, id, status, inv.Status, failureCode, failureMessage)
}

// SaveReport stores the final report. The AI worker sets the final status
// separately (ApplyStatus): only it knows whether the report's root cause is
// supported, which decides completed versus inconclusive.
func (s *Service) SaveReport(ctx context.Context, id uuid.UUID, req SaveReportRequest) error {
	return s.store.SaveReport(ctx, &InvestigationReportRecord{
		InvestigationID:  id,
		SchemaVersion:    req.SchemaVersion,
		ReportJSON:       req.ReportJSON,
		ValidationStatus: req.ValidationStatus,
	})
}

func (s *Service) AppendEvent(ctx context.Context, id uuid.UUID, req AppendEventRequest) error {
	payload := req.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	return s.store.AppendEvent(ctx, &InvestigationEvent{
		InvestigationID: id,
		SequenceNumber:  req.SequenceNumber,
		EventType:       req.EventType,
		Summary:         req.Summary,
		Payload:         payload,
	})
}

func orNew(id uuid.UUID) uuid.UUID {
	if id == uuid.Nil {
		return uuid.New()
	}
	return id
}
