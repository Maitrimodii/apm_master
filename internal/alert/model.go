package alert

import (
	"time"

	"github.com/google/uuid"
)

// Alert is an alert definition (a monitoring rule).
type Alert struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	CreatedBy   uuid.UUID
	Name        string
	Description *string
	Severity    Severity
	// Condition is the rule as the user wrote it; Expression is its parsed
	// form, stored when the alert is saved. Expression is nil only for rows
	// written before parsed conditions were stored.
	Condition         string
	Expression        *Expression
	IsEnabled         bool
	NotificationRoles []uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ParsedCondition returns the stored expression, parsing Condition for
// rows saved before parsed conditions were stored.
func (a Alert) ParsedCondition() (Expression, error) {
	if a.Expression != nil && len(a.Expression.AnyOf) > 0 {
		return *a.Expression, nil
	}
	return ParseExpression(a.Condition)
}

// AlertInstance is one incident raised by an alert definition.
type AlertInstance struct {
	ID               uuid.UUID
	AlertID          uuid.UUID
	State            AlertState
	PendingFiredAt   *time.Time
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
	// Filled by ListInstances from the parent alert, for display.
	AlertName string
	Severity  Severity
}

// Recipient is a user who receives an alert's email notifications.
type Recipient struct {
	ID    uuid.UUID
	Email string
}

// InstanceFilter selects alert instances for one org.
type InstanceFilter struct {
	OrgID    uuid.UUID
	State    AlertState
	Severity Severity
	AlertID  uuid.UUID
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

// Incident is an alert instance together with the definition that raised it.
// AI investigations read incidents through this type.
type Incident struct {
	Alert       Alert
	Instance    AlertInstance
	ServiceName *string
}
