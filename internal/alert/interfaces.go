package alert

import (
	"context"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/notification"
)

// DefinitionStore persists alert definitions.
type DefinitionStore interface {
	Create(ctx context.Context, alert Alert) (*Alert, error)
	Get(ctx context.Context, orgID, alertID uuid.UUID) (*Alert, error)
	List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Alert, int, error)
	Update(ctx context.Context, alert Alert) (*Alert, error)
	Delete(ctx context.Context, orgID, alertID uuid.UUID) (bool, error)
	ListEnabledPage(ctx context.Context, limit, offset int) ([]Alert, error)
}

// InstanceStore persists alert instances (incidents).
type InstanceStore interface {
	GetActiveInstance(ctx context.Context, alertID uuid.UUID) (*AlertInstance, error)
	SaveInstance(ctx context.Context, instance AlertInstance) error
	ListInstances(ctx context.Context, filter InstanceFilter) ([]AlertInstance, int, error)
	GetInstanceDetails(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceDetailsResponse, error)
	GetInstance(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstance, error)
}

// RecipientResolver finds the users holding any of the given roles.
type RecipientResolver interface {
	ResolveUsersByRoles(ctx context.Context, orgID uuid.UUID, roles []uuid.UUID) ([]Recipient, error)
}

// Repository is everything the alert service and evaluation task read and write.
type Repository interface {
	DefinitionStore
	InstanceStore
	RecipientResolver
}

// Notifier is the part of the notification service alerts use.
type Notifier interface {
	RecordEvent(ctx context.Context, event notification.Event) (*notification.Event, error)
	EnqueueHighPriorityEmail(ctx context.Context, task notification.Task) error
}

var (
	_ Repository = (*Store)(nil)
	_ Notifier   = (*notification.Service)(nil)
)
