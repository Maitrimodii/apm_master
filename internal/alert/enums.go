package alert

// AlertState defines the lifecycle state of an alert instance.
type AlertState string

const (
	AlertStatePending      AlertState = "PENDING"
	AlertStateFiring       AlertState = "FIRING"
	AlertStateAcknowledged AlertState = "ACKNOWLEDGED"
	AlertStateResolved     AlertState = "RESOLVED"
)

// AlertSeverity defines the urgency classification for an alert.
type AlertSeverity string

const (
	SeverityCritical AlertSeverity = "critical"
	SeverityHigh     AlertSeverity = "high"
	SeverityMedium   AlertSeverity = "medium"
	SeverityLow      AlertSeverity = "low"
)
