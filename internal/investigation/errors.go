package investigation

import (
	"github.com/Maitrimodii/apm_master/internal/investigation/evidence"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// Investigation error codes.
const (
	CodeInvestigationNotFound apperr.Code = "INV001"
	CodeInvalidTransition     apperr.Code = "INV002"
	CodeIncidentNotFound      apperr.Code = "INV003"
	CodeReportNotReady        apperr.Code = "INV004"
	CodeEvidenceNotFound      apperr.Code = "INV005"
	CodeInvalidTimeWindow     apperr.Code = "INV006"
	CodeInvalidEvidenceQuery  apperr.Code = "INV007"
	CodeTenantMismatch        apperr.Code = "INV008"
)

// apiErrors maps this module's sentinel errors to API errors.
var apiErrors = apperr.Mapping{
	ErrInvestigationNotFound: apperr.NotFound(CodeInvestigationNotFound, "investigation not found"),
	ErrIncidentNotFound:      apperr.NotFound(CodeIncidentNotFound, "incident not found"),
	ErrReportNotReady:        apperr.NotFound(CodeReportNotReady, "report not yet available"),
	ErrEvidenceNotFound:      apperr.NotFound(CodeEvidenceNotFound, "evidence not found"),
	ErrInvalidTransition:     apperr.Conflict(CodeInvalidTransition, "invalid status transition"),
	ErrCannotRetryActive:     apperr.Conflict(CodeInvalidTransition, "can only retry investigations in a terminal state"),
	ErrTimeWindowInvalid:     apperr.BadRequest(CodeInvalidTimeWindow, "time window start must be before end"),
	ErrTimeWindowTooLarge:    apperr.BadRequest(CodeInvalidTimeWindow, "time window cannot exceed 24 hours"),
	ErrTenantMismatch:        apperr.Forbidden(CodeTenantMismatch, "cross-tenant access denied"),
	evidence.ErrInvalidQuery: apperr.BadRequest(CodeInvalidEvidenceQuery, "invalid evidence query"),
}
