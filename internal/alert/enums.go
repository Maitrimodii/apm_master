package alert

import "strings"

// AlertState is the lifecycle state of an alert instance.
type AlertState string

const (
	AlertStatePending      AlertState = "PENDING"
	AlertStateFiring       AlertState = "FIRING"
	AlertStateAcknowledged AlertState = "ACKNOWLEDGED"
	AlertStateResolved     AlertState = "RESOLVED"
)

// activeStates are the states an instance can be in before it resolves,
// as plain strings for use as a SQL array parameter.
var activeStates = []string{string(AlertStatePending), string(AlertStateFiring), string(AlertStateAcknowledged)}

// Severity is the urgency of an alert definition. Stored lowercase.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// ParseSeverity accepts any letter case ("CRITICAL", "critical").
func ParseSeverity(s string) (Severity, bool) {
	sev := Severity(strings.ToLower(strings.TrimSpace(s)))
	switch sev {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return sev, true
	}
	return "", false
}
