package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type mockTask struct {
	name        string
	interval    time.Duration
	executeFunc func(ctx context.Context) error
	
	mu       sync.Mutex
	runCount int
}

func (m *mockTask) Name() string { return m.name }
func (m *mockTask) Interval() time.Duration { return m.interval }
func (m *mockTask) Execute(ctx context.Context) error {
	m.mu.Lock()
	m.runCount++
	m.mu.Unlock()
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
	}
	return nil
}

func (m *mockTask) RunCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runCount
}

func TestScheduler_Register(t *testing.T) {
	s := New(nil)

	err := s.Register(&mockTask{name: "task1", interval: time.Second})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = s.Register(&mockTask{name: "task1", interval: time.Second})
	if err == nil {
		t.Fatal("expected error registering duplicate task, got nil")
	}
	
	err = s.Register(&mockTask{name: "task2", interval: 0})
	if err == nil {
		t.Fatal("expected error registering task with 0 interval, got nil")
	}
}

func TestScheduler_ExecutionAndState(t *testing.T) {
	s := New(nil)
	
	task := &mockTask{
		name: "test-task",
		interval: 50 * time.Millisecond,
	}
	
	s.Register(task)
	
	ctx := context.Background()
	s.Start(ctx)
	
	time.Sleep(120 * time.Millisecond) // Should run immediately, then ~2 more times
	
	state, ok := s.GetTaskState("test-task")
	if !ok {
		t.Fatal("expected state to exist")
	}
	
	if state.RunCount < 2 {
		t.Fatalf("expected run count >= 2, got %d", state.RunCount)
	}
	
	if state.LastError != nil {
		t.Fatalf("expected no last error, got %v", state.LastError)
	}
	
	s.Stop()
}

func TestScheduler_ErrorHandling(t *testing.T) {
	s := New(nil)
	
	expectedErr := errors.New("something went wrong")
	task := &mockTask{
		name: "error-task",
		interval: 10 * time.Millisecond,
		executeFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}
	
	s.Register(task)
	s.Start(context.Background())
	
	time.Sleep(30 * time.Millisecond)
	s.Stop()
	
	state, _ := s.GetTaskState("error-task")
	if state.LastError == nil || state.LastError.Error() != expectedErr.Error() {
		t.Fatalf("expected last error %q, got %v", expectedErr, state.LastError)
	}
	if state.RunCount < 2 {
		t.Fatal("expected task to continue running after error")
	}
}

func TestScheduler_PanicRecovery(t *testing.T) {
	s := New(nil)
	
	task := &mockTask{
		name: "panic-task",
		interval: 10 * time.Millisecond,
		executeFunc: func(ctx context.Context) error {
			panic("intentional panic")
		},
	}
	
	s.Register(task)
	s.Start(context.Background())
	
	time.Sleep(30 * time.Millisecond)
	s.Stop()
	
	state, _ := s.GetTaskState("panic-task")
	if state.LastError == nil || state.LastError.Error() != "panic: intentional panic" {
		t.Fatalf("expected panic error, got %v", state.LastError)
	}
	if state.RunCount < 2 {
		t.Fatal("expected task to continue running after panic")
	}
}
