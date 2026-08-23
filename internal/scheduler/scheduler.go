package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// TaskState tracks the execution state of a task.
type TaskState struct {
	LastRunAt time.Time
	NextRunAt time.Time
	LastError error
	RunCount  int64
	IsRunning bool
}

// Scheduler executes registered tasks at their defined intervals.
type Scheduler struct {
	mu      sync.RWMutex
	tasks   map[string]Task
	states  map[string]*TaskState
	log     *slog.Logger
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running bool
}

// New creates a new job scheduler.
func New(log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		tasks:  make(map[string]Task),
		states: make(map[string]*TaskState),
		log:    log,
	}
}

// Register adds a task to the scheduler. Returns an error if a task with the same name already exists.
func (s *Scheduler) Register(task Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := task.Name()
	if _, exists := s.tasks[name]; exists {
		return fmt.Errorf("task %q is already registered", name)
	}

	if task.Interval() <= 0 {
		return fmt.Errorf("task %q must have an interval greater than 0", name)
	}

	s.tasks[name] = task
	s.states[name] = &TaskState{}
	return nil
}

// Start launches the scheduler and begins running registered tasks.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	ctx, s.cancel = context.WithCancel(ctx)

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, task := range s.tasks {
		s.wg.Add(1)
		go s.runTask(ctx, task)
	}
	s.log.Info("scheduler started", "task_count", len(s.tasks))
}

// Stop gracefully shuts down the scheduler, waiting for in-flight tasks to complete.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()

	s.log.Info("scheduler stopping, waiting for tasks...")
	s.wg.Wait()
	s.log.Info("scheduler stopped")
}

// GetTaskState returns a copy of the current state for a given task name.
func (s *Scheduler) GetTaskState(name string) (TaskState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.states[name]
	if !exists {
		return TaskState{}, false
	}
	// Return a copy
	return *state, true
}

// runTask is the main event loop for a single task.
func (s *Scheduler) runTask(ctx context.Context, task Task) {
	defer s.wg.Done()

	name := task.Name()
	interval := task.Interval()

	s.log.Debug("task started", "task", name, "interval", interval)

	// Recover from panics in the main task loop
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("task loop panicked", "task", name, "panic", r)
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial execution handling (e.g. downtime recovery or immediate start)
	s.mu.RLock()
	state := s.states[name]
	s.mu.RUnlock()

	// If it's time to run (or past due), run immediately
	if time.Since(state.LastRunAt) >= interval {
		s.executeSingleRun(ctx, task)
	} else {
		// Update next run time for observability if we skipped initial run
		s.mu.Lock()
		state.NextRunAt = state.LastRunAt.Add(interval)
		s.mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			s.log.Debug("task stopped via context", "task", name)
			return
		case <-ticker.C:
			s.executeSingleRun(ctx, task)
		}
	}
}

// executeSingleRun performs a single execution of the task, managing state and panics.
func (s *Scheduler) executeSingleRun(ctx context.Context, task Task) {
	name := task.Name()

	// Prepare state
	s.mu.Lock()
	state := s.states[name]
	if state.IsRunning {
		s.log.Warn("task execution skipped, already running", "task", name)
		s.mu.Unlock()
		return
	}
	state.IsRunning = true
	s.mu.Unlock()

	// Ensure state cleanup and panic recovery for this run
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			s.log.Error("task execution panicked", "task", name, "panic", r)
		}

		s.mu.Lock()
		state.IsRunning = false
		state.LastRunAt = time.Now().UTC()
		state.NextRunAt = state.LastRunAt.Add(task.Interval())
		state.LastError = err
		state.RunCount++
		s.mu.Unlock()
	}()

	s.log.Debug("executing task", "task", name)
	
	// Create a sub-context for the execution, optional timeout could be added here
	err = task.Execute(ctx)
	if err != nil {
		s.log.Error("task execution failed", "task", name, "error", err)
	}
}
