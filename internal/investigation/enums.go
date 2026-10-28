package investigation

// InvestigationStatus represents the lifecycle state of an investigation.
type InvestigationStatus string

const (
	StatusQueued       InvestigationStatus = "queued"
	StatusRunning      InvestigationStatus = "running"
	StatusCompleted    InvestigationStatus = "completed"
	StatusInconclusive InvestigationStatus = "inconclusive"
	StatusFailed       InvestigationStatus = "failed"
	StatusCancelled    InvestigationStatus = "cancelled"
	StatusTimedOut     InvestigationStatus = "timed_out"
)

// TerminalStatuses contains all states representing completed execution.
var TerminalStatuses = map[InvestigationStatus]bool{
	StatusCompleted:    true,
	StatusInconclusive: true,
	StatusFailed:       true,
	StatusCancelled:    true,
	StatusTimedOut:     true,
}

// ValidTransitions maps allowed state transitions.
var ValidTransitions = map[InvestigationStatus]map[InvestigationStatus]bool{
	StatusQueued: {
		StatusRunning:   true,
		StatusCancelled: true,
	},
	StatusRunning: {
		StatusCompleted:    true,
		StatusInconclusive: true,
		StatusFailed:       true,
		StatusTimedOut:     true,
		StatusCancelled:    true,
	},
}

// CanTransition checks if moving from fromStatus to toStatus is permitted.
func CanTransition(from, to InvestigationStatus) bool {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}
