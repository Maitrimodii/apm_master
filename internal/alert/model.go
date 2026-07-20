package alert

import (
	"time"

	"github.com/google/uuid"
)

type Alert struct {
	ID                uuid.UUID
	OrgID             uuid.UUID
	CreatedBy         uuid.UUID
	Name              string
	Description       *string
	Severity          string
	Condition         string
	IsEnabled         bool
	NotificationRoles []uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

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

type AlertResponse struct {
	ID                uuid.UUID   `json:"id"`
	OrgID             uuid.UUID   `json:"org_id"`
	CreatedBy         uuid.UUID   `json:"created_by"`
	Name              string      `json:"name"`
	Description       *string     `json:"description,omitempty"`
	Severity          string      `json:"severity"`
	Condition         string      `json:"condition"`
	IsEnabled         bool        `json:"is_enabled"`
	NotificationRoles []uuid.UUID `json:"notification_roles"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type ShareAlertResponse struct {
	Message string `json:"message"`
}

const (
	AlertStateOK           = "OK"
	AlertStateFiring       = "FIRING"
	AlertStateAcknowledged = "ACKNOWLEDGED"
	AlertStateResolved     = "RESOLVED"
)

// AlertInstance stores a firing incident for an alert definition.
type AlertInstance struct {
	ID               uuid.UUID
	AlertID          uuid.UUID
	State            string
	TriggeredAt      time.Time
	AcknowledgedAt   *time.Time
	ResolvedAt       *time.Time
	LastEvaluatedAt  time.Time
	Value            float64
	Threshold        float64
	Message          string
	NotificationSent bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type AlertInstanceResponse struct {
	ID               uuid.UUID  `json:"id"`
	AlertID          uuid.UUID  `json:"alert_id"`
	State            string     `json:"state"`
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

func NewAlertInstanceResponse(instance AlertInstance) AlertInstanceResponse {
	return AlertInstanceResponse{
		ID:               instance.ID,
		AlertID:          instance.AlertID,
		State:            instance.State,
		TriggeredAt:      instance.TriggeredAt,
		AcknowledgedAt:   instance.AcknowledgedAt,
		ResolvedAt:       instance.ResolvedAt,
		LastEvaluatedAt:  instance.LastEvaluatedAt,
		Value:            instance.Value,
		Threshold:        instance.Threshold,
		Message:          instance.Message,
		NotificationSent: instance.NotificationSent,
		CreatedAt:        instance.CreatedAt,
		UpdatedAt:        instance.UpdatedAt,
	}
}

func NewAlertResponse(alert Alert) AlertResponse {
	return AlertResponse{
		ID:                alert.ID,
		OrgID:             alert.OrgID,
		CreatedBy:         alert.CreatedBy,
		Name:              alert.Name,
		Description:       alert.Description,
		Severity:          alert.Severity,
		Condition:         alert.Condition,
		IsEnabled:         alert.IsEnabled,
		NotificationRoles: alert.NotificationRoles,
		CreatedAt:         alert.CreatedAt,
		UpdatedAt:         alert.UpdatedAt,
	}
}

type AlertInstanceDetailsResponse struct {
	ID               uuid.UUID  `json:"id"`
	AlertID          uuid.UUID  `json:"alert_id"`
	AlertName        string     `json:"alert_name"`
	AlertDescription *string    `json:"alert_description,omitempty"`
	State            string     `json:"state"`
	Severity         string     `json:"severity"`
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
