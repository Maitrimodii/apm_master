package alert

import "time"

const (
	// DefaultEvaluationInterval is the fallback period for evaluating alert conditions.
	DefaultEvaluationInterval = time.Minute

	// DefaultAdvisoryLockKey is the PostgreSQL advisory lock key for alert evaluation task clustering.
	DefaultAdvisoryLockKey = int64(0xA1A2A3A4)

	// DefaultPendingDuration is the time an alert must continuously meet firing condition before transitioning from PENDING to FIRING.
	DefaultPendingDuration = 2 * time.Minute

	// DefaultQueryLimit is the standard record limit when listing alert instances.
	DefaultQueryLimit = 50
)
