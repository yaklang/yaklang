package asyncexecmanager

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestNewResourceRequiresCallbacks(t *testing.T) {
	execute := func(context.Context) error { return nil }
	for _, callbacks := range []struct {
		execute func(context.Context) error
		inspect func(context.Context) (Observation, error)
	}{
		{},
		{execute: execute},
		{inspect: quietInspector},
	} {
		resource, err := NewResource(callbacks.execute, callbacks.inspect)
		if resource != nil || !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid callbacks: %v, %v", resource, err)
		}
	}
}

func TestFunctionResourceIntegration(t *testing.T) {
	type contextKey struct{}
	owner := context.WithValue(context.Background(), contextKey{}, "owner")
	m := newTestManager(t, owner, Config{})
	started := make(chan struct{})
	var executions, observations atomic.Int32
	var cleanedUp atomic.Bool
	resource, err := NewResource(func(ctx context.Context) error {
		executions.Add(1)
		if ctx.Value(contextKey{}) != "owner" {
			t.Error("lost owner context")
		}
		close(started)
		<-ctx.Done()
		cleanedUp.Store(true)
		return ctx.Err()
	}, func(ctx context.Context) (Observation, error) {
		observations.Add(1)
		if ctx.Value(contextKey{}) != "inspection" {
			t.Error("lost inspection context")
		}
		return Observation{Stdout: "owned by resource"}, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if executions.Load() != 0 || observations.Load() != 0 {
		t.Fatal("constructor invoked callbacks")
	}
	_, err = m.Start(Request{ID: "functions", Resource: resource})
	if err != nil {
		t.Fatal(err)
	}
	awaitSignal(t, started)
	if observations.Load() != 0 {
		t.Fatal("start sampled observations")
	}
	if _, err := m.Stop("functions"); err != nil {
		t.Fatal(err)
	}
	final := waitExecution(t, m, "functions")
	if final.State != StateCancelled || !cleanedUp.Load() || executions.Load() != 1 {
		t.Fatalf("execution lifecycle: %+v, cleanup=%v, executions=%d", final, cleanedUp.Load(), executions.Load())
	}
	ctx := context.WithValue(context.Background(), contextKey{}, "inspection")
	snapshot, err := m.Inspect(ctx, "functions")
	if err != nil || snapshot.Observation.Stdout != "owned by resource" || observations.Load() != 1 {
		t.Fatalf("retained inspection: %+v, %v, reads=%d", snapshot, err, observations.Load())
	}
}
