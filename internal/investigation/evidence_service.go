package investigation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/investigation/evidence"
)

// provenanceSource names this service in evidence provenance.
const provenanceSource = "apm_master"

// EvidenceService runs evidence queries for the AI investigator and records
// every result, so each evidence ID the AI cites exists in the database.
type EvidenceService struct {
	store    *Store
	registry evidence.Registry
	log      *slog.Logger
}

func NewEvidenceService(store *Store, registry evidence.Registry, log *slog.Logger) *EvidenceService {
	if log == nil {
		log = slog.Default()
	}
	return &EvidenceService{store: store, registry: registry, log: log}
}

// Query checks the request against its investigation's tenant, runs it, and
// persists the evidence before returning it. If persisting fails the whole
// query fails, so the AI never receives an evidence ID with no stored record.
func (s *EvidenceService) Query(ctx context.Context, req EvidenceQueryRequest) (*EvidenceQueryResponse, error) {
	if req.TimeRangeStart.After(req.TimeRangeEnd) {
		return nil, ErrTimeWindowInvalid
	}
	if req.TimeRangeEnd.Sub(req.TimeRangeStart) > MaxTimeWindow {
		return nil, ErrTimeWindowTooLarge
	}
	inv, err := s.store.GetByID(ctx, req.InvestigationID)
	if err != nil {
		return nil, err
	}
	if inv == nil {
		return nil, ErrInvestigationNotFound
	}
	if inv.OrgID != req.TenantID {
		return nil, ErrTenantMismatch
	}

	queryCtx, cancel := context.WithTimeout(ctx, evidenceQueryTimeout)
	defer cancel()
	qt := evidence.QueryType(req.QueryType)
	result, err := s.registry.Execute(queryCtx, qt, evidence.Query{
		TenantID:   req.TenantID,
		From:       req.TimeRangeStart,
		To:         req.TimeRangeEnd,
		Parameters: req.Parameters,
	})
	if err != nil {
		return nil, err
	}

	provenance := map[string]any{
		"source":           provenanceSource,
		"query_type":       req.QueryType,
		"queried_at":       time.Now().UTC().Format(time.RFC3339),
		"tenant_id":        req.TenantID.String(),
		"investigation_id": req.InvestigationID.String(),
	}
	record := &InvestigationEvidence{
		ID:                  uuid.New(),
		InvestigationID:     req.InvestigationID,
		EvidenceType:        req.QueryType,
		SourceService:       result.ServiceName,
		QueryParameters:     req.Parameters,
		Summary:             result.Summary,
		Result:              result.Data,
		Provenance:          provenance,
		DataQualityWarnings: result.Warnings,
	}
	if err := s.store.SaveEvidence(ctx, record); err != nil {
		return nil, fmt.Errorf("persist evidence: %w", err)
	}
	return &EvidenceQueryResponse{
		EvidenceID:          record.ID,
		QueryType:           req.QueryType,
		Result:              result.Data,
		Provenance:          provenance,
		DataQualityWarnings: result.Warnings,
	}, nil
}
