package scheduler

import (
	"context"
	"time"
)

// Task is the unit of work that the scheduler executes.
type Task interface {
	// Name returns a unique identifier for this task.
	Name() string
	// Interval returns how often this task should run.
	Interval() time.Duration
	// Execute runs the task logic. It receives a context for cancellation.
	Execute(ctx context.Context) error
}
