package alert

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AlertStoreInterface abstracts all database operations for alert definitions and instances.
// Accepting this interface instead of the concrete *Store allows service-level unit tests
// to use in-memory mocks without a live database.
type AlertStoreInterface interface {
	Create(ctx context.Context, alert Alert) (*Alert, error)
	Get(ctx context.Context, orgID, alertID uuid.UUID) (*Alert, error)
	List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Alert, int, error)
	Update(ctx context.Context, alert Alert) (*Alert, error)
	Delete(ctx context.Context, orgID, alertID uuid.UUID) error
	ListEnabledPage(ctx context.Context, limit, offset int) ([]Alert, error)
	ResolveUsersByRoles(ctx context.Context, orgID uuid.UUID, roles []uuid.UUID) ([]ResolvedUser, error)
	GetActiveInstance(ctx context.Context, alertID uuid.UUID) (*AlertInstance, error)
	SaveInstance(ctx context.Context, instance AlertInstance) error
	ListInstances(ctx context.Context, orgID uuid.UUID, state, severity string, alertID uuid.UUID, from, to *time.Time, limit, offset int) ([]AlertInstance, int, error)
	GetInstanceDetails(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceDetailsResponse, error)
	GetInstance(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstance, error)
	AcquireLock(ctx context.Context, lockKey int64) (bool, error)
	ReleaseLock(ctx context.Context, lockKey int64) error
}

// compile-time assertion: *Store must satisfy AlertStoreInterface.
var _ AlertStoreInterface = (*Store)(nil)
