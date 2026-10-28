package investigation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	store  *Store
	logger *slog.Logger
}

func NewService(store *Store, logger *slog.Logger) *Service {
	return &Service{
		store:  store,
		logger: logger,
	}
}

// Create creates a new investigation and outbox event atomically.
func (s *Service) Create(
	ctx context.Context,
	orgID, userID, incidentID, correlationID uuid.UUID,
	req CreateInvestigationRequest,
) (*Investigation, error) {
	// 1. Verify incident exists and belongs to this organization
	incidentContext, err := s.store.GetIncidentContext(ctx, incidentID)
	if err != nil {
		return nil, fmt.Errorf("verify incident: %w", err)
	}
	if incidentContext == nil || incidentContext.OrgID != orgID {
		return nil, ErrIncidentNotFound
	}

	// 2. Validate / default time window
	var start, end time.Time
	if req.TimeRangeStart != nil && !req.TimeRangeStart.IsZero() {
		start = *req.TimeRangeStart
	} else {
		start = incidentContext.TriggeredAt.Add(-DefaultWindowPadding)
	}

	if req.TimeRangeEnd != nil && !req.TimeRangeEnd.IsZero() {
		end = *req.TimeRangeEnd
	} else {
		end = incidentContext.TriggeredAt.Add(DefaultWindowPadding)
	}

	if start.After(end) {
		return nil, ErrTimeWindowInvalid
	}
	if end.Sub(start) > MaxTimeWindow {
		return nil, ErrTimeWindowTooLarge
	}

	// 3. Concurrency limit check
	runningCount, err := s.store.CountRunningByTenant(ctx, orgID)
	if err != nil {
		s.logger.Warn("failed to count running investigations for concurrency check", "error", err)
	} else if runningCount >= MaxConcurrentPerTenantDefault {
		s.logger.Warn("tenant reached concurrent investigations limit", "org_id", orgID, "count", runningCount)
	}

	invID := uuid.New()
	if correlationID == uuid.Nil {
		correlationID = uuid.New()
	}

	focus := req.Focus
	if focus == nil {
		focus = []string{}
	}

	inv := &Investigation{
		ID:             invID,
		OrgID:          orgID,
		IncidentID:     incidentID,
		RequestedBy:    userID,
		Status:         StatusQueued,
		TimeRangeStart: start.UTC(),
		TimeRangeEnd:   end.UTC(),
		Focus:          focus,
		CorrelationID:  correlationID,
		IdempotencyKey: req.IdempotencyKey,
	}

	jobEvent := InvestigationJobEvent{
		EventID:         uuid.New(),
		SchemaVersion:   "1.0",
		EventType:       "investigation.requested",
		CorrelationID:   correlationID,
		InvestigationID: invID,
		IncidentID:      incidentID,
		TenantID:        orgID,
		CreatedAt:       time.Now().UTC(),
	}

	payload, err := json.Marshal(jobEvent)
	if err != nil {
		return nil, fmt.Errorf("marshal job event: %w", err)
	}

	created, err := s.store.CreateWithOutbox(ctx, inv, payload)
	if err != nil {
		return nil, err
	}

	s.logger.Info("investigation created",
		"investigation_id", created.ID,
		"incident_id", created.IncidentID,
		"tenant_id", created.OrgID,
		"correlation_id", created.CorrelationID,
	)

	return created, nil
}

func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (*Investigation, error) {
	return s.store.Get(ctx, orgID, id)
}

func (s *Service) List(ctx context.Context, orgID, incidentID uuid.UUID, page, limit int) ([]Investigation, int, error) {
	if limit <= 0 {
		limit = DefaultPaginationLimit
	}
	if limit > MaxPaginationLimit {
		limit = MaxPaginationLimit
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	return s.store.List(ctx, orgID, incidentID, limit, offset)
}

func (s *Service) Cancel(ctx context.Context, orgID, userID, id uuid.UUID) (*Investigation, error) {
	existing, err := s.store.Get(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrInvestigationNotFound
	}

	// Idempotent: If already in a terminal state, return as is without error
	if TerminalStatuses[existing.Status] {
		return existing, nil
	}

	updated, err := s.store.Cancel(ctx, orgID, id)
	if err != nil {
		return nil, err
	}

	s.logger.Info("investigation cancellation requested",
		"investigation_id", id,
		"tenant_id", orgID,
		"user_id", userID,
	)

	return updated, nil
}

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

	if correlationID == uuid.Nil {
		correlationID = uuid.New()
	}

	newInvID := uuid.New()
	inv := &Investigation{
		ID:                    newInvID,
		OrgID:                 orgID,
		IncidentID:            existing.IncidentID,
		RequestedBy:           userID,
		Status:                StatusQueued,
		TimeRangeStart:        existing.TimeRangeStart,
		TimeRangeEnd:          existing.TimeRangeEnd,
		Focus:                 existing.Focus,
		CorrelationID:         correlationID,
		ParentInvestigationID: &existing.ID,
	}

	jobEvent := InvestigationJobEvent{
		EventID:         uuid.New(),
		SchemaVersion:   "1.0",
		EventType:       "investigation.requested",
		CorrelationID:   correlationID,
		InvestigationID: newInvID,
		IncidentID:      existing.IncidentID,
		TenantID:        orgID,
		CreatedAt:       time.Now().UTC(),
	}

	payload, err := json.Marshal(jobEvent)
	if err != nil {
		return nil, fmt.Errorf("marshal retry job event: %w", err)
	}

	created, err := s.store.CreateWithOutbox(ctx, inv, payload)
	if err != nil {
		return nil, err
	}

	s.logger.Info("investigation retried",
		"new_investigation_id", created.ID,
		"parent_investigation_id", existing.ID,
		"tenant_id", orgID,
		"correlation_id", correlationID,
	)

	return created, nil
}

func (s *Service) GetReport(ctx context.Context, orgID, id uuid.UUID) (*InvestigationReportRecord, error) {
	return s.store.GetReport(ctx, orgID, id)
}

func (s *Service) GetEvidence(ctx context.Context, orgID, id, evidenceID uuid.UUID) (*InvestigationEvidence, error) {
	return s.store.GetEvidence(ctx, orgID, id, evidenceID)
}

func (s *Service) ListEvents(ctx context.Context, orgID, id uuid.UUID) ([]InvestigationEvent, error) {
	return s.store.ListEvents(ctx, orgID, id)
}
