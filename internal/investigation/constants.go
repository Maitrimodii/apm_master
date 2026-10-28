package investigation

import "time"

const (
	// MaxTimeWindow is the maximum permissible analysis window for an investigation.
	MaxTimeWindow = 24 * time.Hour

	// MaxConcurrentPerTenantDefault is the maximum number of concurrent running investigations per tenant.
	MaxConcurrentPerTenantDefault = 5

	// DefaultPaginationLimit is the default page size when listing investigations.
	DefaultPaginationLimit = 20

	// MaxPaginationLimit is the maximum page size when listing investigations.
	MaxPaginationLimit = 100

	// DefaultLeaseDuration is the timeout given to a worker to renew or complete an investigation lease.
	DefaultLeaseDuration = 150 * time.Second

	// DefaultWindowPadding is the default buffer added on either side of the trigger timestamp.
	DefaultWindowPadding = 15 * time.Minute
)
