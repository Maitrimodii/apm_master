package alert

import "time"

const (
	// DefaultEvaluationInterval applies when ALERT_EVALUATION_INTERVAL_MS is unset.
	DefaultEvaluationInterval = time.Minute
	// evaluationTimeout bounds the telemetry queries for one alert.
	evaluationTimeout = 10 * time.Second
	// enabledAlertsPageSize is how many alerts one evaluation query loads.
	enabledAlertsPageSize = 500
	// defaultPageSize is the list page size for alerts and instances.
	defaultPageSize = 20
	// adminRoleName is notified when an alert has no notification roles.
	adminRoleName = "admin"
)
