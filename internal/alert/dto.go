package alert

import (
	"time"

	"github.com/google/uuid"
)

type CreateAlertRequest struct {
	Name              string      `json:"name" binding:"required"`
	Description       *string     `json:"description,omitempty"`
	Severity          string      `json:"severity" binding:"required"`
	Condition         string      `json:"condition" binding:"required"`
	IsEnabled         *bool       `json:"is_enabled,omitempty"`
	NotificationRoles []uuid.UUID `json:"notification_roles,omitempty"`
}

type UpdateAlertRequest struct {
	Name              string      `json:"name" binding:"required"`
	Description       *string     `json:"description,omitempty"`
	Severity          string      `json:"severity" binding:"required"`
	Condition         string      `json:"condition" binding:"required"`
	IsEnabled         bool        `json:"is_enabled"`
	NotificationRoles []uuid.UUID `json:"notification_roles" binding:"required"`
}

type ShareAlertRequest struct {
	Emails  []string `json:"emails" binding:"required"`
	Message *string  `json:"message,omitempty"`
}

type ShareAlertResponse struct {
	Message string `json:"message"`
}

type AlertResponse struct {
	ID                uuid.UUID   `json:"id"`
	OrgID             uuid.UUID   `json:"org_id"`
	CreatedBy         uuid.UUID   `json:"created_by"`
	Name              string      `json:"name"`
	Description       *string     `json:"description,omitempty"`
	Severity          Severity    `json:"severity"`
	Condition         string      `json:"condition"`
	IsEnabled         bool        `json:"is_enabled"`
	NotificationRoles []uuid.UUID `json:"notification_roles"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

func NewAlertResponse(a Alert) AlertResponse {
	return AlertResponse{
		ID:                a.ID,
		OrgID:             a.OrgID,
		CreatedBy:         a.CreatedBy,
		Name:              a.Name,
		Description:       a.Description,
		Severity:          a.Severity,
		Condition:         a.Condition,
		IsEnabled:         a.IsEnabled,
		NotificationRoles: a.NotificationRoles,
		CreatedAt:         a.CreatedAt,
		UpdatedAt:         a.UpdatedAt,
	}
}

type AlertInstanceResponse struct {
	ID               uuid.UUID  `json:"id"`
	AlertID          uuid.UUID  `json:"alert_id"`
	AlertName        string     `json:"alert_name,omitempty"`
	Severity         Severity   `json:"severity,omitempty"`
	State            AlertState `json:"state"`
	TriggeredAt      time.Time  `json:"triggered_at"`
	AcknowledgedAt   *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	LastEvaluatedAt  time.Time  `json:"last_evaluated_at"`
	Value            float64    `json:"value"`
	Threshold        float64    `json:"threshold"`
	Message          string     `json:"message"`
	NotificationSent bool       `json:"notification_sent"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func NewAlertInstanceResponse(i AlertInstance) AlertInstanceResponse {
	return AlertInstanceResponse{
		ID:               i.ID,
		AlertID:          i.AlertID,
		AlertName:        i.AlertName,
		Severity:         i.Severity,
		State:            i.State,
		TriggeredAt:      i.TriggeredAt,
		AcknowledgedAt:   i.AcknowledgedAt,
		ResolvedAt:       i.ResolvedAt,
		LastEvaluatedAt:  i.LastEvaluatedAt,
		Value:            i.Value,
		Threshold:        i.Threshold,
		Message:          i.Message,
		NotificationSent: i.NotificationSent,
		CreatedAt:        i.CreatedAt,
		UpdatedAt:        i.UpdatedAt,
	}
}

type AlertInstanceDetailsResponse struct {
	ID               uuid.UUID  `json:"id"`
	AlertID          uuid.UUID  `json:"alert_id"`
	AlertName        string     `json:"alert_name"`
	AlertDescription *string    `json:"alert_description,omitempty"`
	State            AlertState `json:"state"`
	Severity         Severity   `json:"severity"`
	TriggeredAt      time.Time  `json:"triggered_at"`
	AcknowledgedAt   *time.Time `json:"acknowledged_at,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	LastEvaluatedAt  time.Time  `json:"last_evaluated_at"`
	Value            float64    `json:"value"`
	Threshold        float64    `json:"threshold"`
	Message          string     `json:"message"`
	NotificationSent bool       `json:"notification_sent"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
