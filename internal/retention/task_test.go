package retention

import (
	"context"
	"testing"
)

type fakeDeleter struct {
	batches map[string][]int64
	calls   map[string]int
}

func (f *fakeDeleter) DeleteOldRecords(_ context.Context, table, _ string, _, _ int) (int64, error) {
	i := f.calls[table]
	f.calls[table]++
	if i < len(f.batches[table]) {
		return f.batches[table][i], nil
	}
	return 0, nil
}

func TestTaskDeletesInBatchesAndSkipsDisabledPolicies(t *testing.T) {
	d := &fakeDeleter{
		batches: map[string][]int64{"a": {deleteBatchSize, 10}},
		calls:   map[string]int{},
	}
	task := NewTask(d, []Policy{
		{Table: "a", TimeColumn: "created_at", RetentionDays: 30},
		{Table: "disabled", TimeColumn: "created_at", RetentionDays: 0},
	}, nil, nil)

	if err := task.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if d.calls["a"] != 2 {
		t.Errorf("table a: %d delete calls, want 2 (full batch, then partial)", d.calls["a"])
	}
	if d.calls["disabled"] != 0 {
		t.Errorf("disabled policy ran %d times", d.calls["disabled"])
	}
}

func TestTaskSkipsWhenLockHeldElsewhere(t *testing.T) {
	d := &fakeDeleter{calls: map[string]int{}}
	notAcquired := func(context.Context, func(context.Context) error) (bool, error) { return false, nil }
	task := NewTask(d, []Policy{{Table: "a", TimeColumn: "created_at", RetentionDays: 1}}, notAcquired, nil)
	if err := task.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if d.calls["a"] != 0 {
		t.Fatal("task ran without the lock")
	}
}
