package asyncexecmanager

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type inspectorFunc func(context.Context) (Observation, error)

func (f inspectorFunc) Inspect(ctx context.Context) (Observation, error) { return f(ctx) }

type testResource struct {
	run      func(context.Context) error
	observer Inspector
}

func (r *testResource) Execute(ctx context.Context) error { return r.run(ctx) }
func (r *testResource) Inspector() Inspector              { return r.observer }

func quietInspector(context.Context) (Observation, error) { return Observation{}, nil }

func newTestManager(t *testing.T, parent context.Context, config Config) *Manager {
	t.Helper()
	m, err := New(parent, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	return m
}

func waitExecution(t *testing.T, m *Manager, id string) Summary {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	summary, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return summary
}

func awaitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("resource did not reach expected phase")
	}
}

func TestInspectorIsLiveAndNotSampledByManager(t *testing.T) {
	m := newTestManager(t, context.Background(), Config{})
	started, release := make(chan struct{}), make(chan struct{})
	var mu sync.Mutex
	observation := Observation{Stdout: "first", PartialResult: json.RawMessage(`{"count":1}`)}
	var reads atomic.Int32
	observer := inspectorFunc(func(ctx context.Context) (Observation, error) {
		reads.Add(1)
		mu.Lock()
		defer mu.Unlock()
		value := observation
		value.Result = append(json.RawMessage(nil), value.Result...)
		value.PartialResult = append(json.RawMessage(nil), value.PartialResult...)
		return value, ctx.Err()
	})
	resource := &testResource{observer: observer, run: func(ctx context.Context) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	receipt, err := m.Start(Request{ID: "live", Resource: resource})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State != StatePending {
		t.Fatalf("receipt: %+v", receipt)
	}
	awaitSignal(t, started)
	if len(m.List()) != 1 || reads.Load() != 0 {
		t.Fatal("start/list sampled observations")
	}
	first, err := m.Inspect(context.Background(), "live")
	if err != nil || first.Observation.Stdout != "first" {
		t.Fatalf("first inspection: %+v, %v", first, err)
	}
	mu.Lock()
	observation.Stdout = "changed without a manager event"
	observation.Stderr = "warning"
	observation.Result = json.RawMessage(`{"answer":42}`)
	mu.Unlock()
	second, err := m.Inspect(context.Background(), "live")
	if err != nil || second.Observation.Stdout != observation.Stdout || second.Observation.Stderr != "warning" {
		t.Fatalf("live inspection: %+v, %v", second, err)
	}
	second.Observation.Result[0] = 'x'
	close(release)
	finished := waitExecution(t, m, "live")
	if finished.State != StateCompleted || finished.EndedAt == nil {
		t.Fatalf("final state: %+v", finished)
	}
	if reads.Load() != 2 {
		t.Fatal("execution/wait sampled observations")
	}
	final, err := m.Inspect(context.Background(), "live")
	if err != nil || string(final.Observation.Result) != `{"answer":42}` {
		t.Fatalf("retained result: %+v, %v", final, err)
	}
}

func TestWaitTimeoutDoesNotStopExecution(t *testing.T) {
	m := newTestManager(t, context.Background(), Config{})
	started, release, wasCancelled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	_, err := m.Start(Request{ID: "waiting", Resource: &testResource{
		observer: inspectorFunc(quietInspector), run: func(ctx context.Context) error {
			close(started)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				close(wasCancelled)
				return ctx.Err()
			}
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	awaitSignal(t, started)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	summary, err := m.Wait(ctx, "waiting")
	if !errors.Is(err, context.DeadlineExceeded) || summary.State != StateRunning {
		t.Fatalf("wait: %+v, %v", summary, err)
	}
	select {
	case <-wasCancelled:
		t.Fatal("waiter cancelled resource")
	default:
	}
	close(release)
	if final := waitExecution(t, m, "waiting"); final.State != StateCompleted {
		t.Fatalf("final: %+v", final)
	}
}

func TestStopRetainsCapacityUntilResourceCleanupReturns(t *testing.T) {
	m := newTestManager(t, context.Background(), Config{MaxConcurrent: 1})
	started, cancelled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
	defer close(cleanup)
	_, err := m.Start(Request{ID: "cleanup", Resource: &testResource{
		observer: inspectorFunc(quietInspector), run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(cancelled)
			<-cleanup
			return ctx.Err()
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	awaitSignal(t, started)
	summary, err := m.Stop("cleanup")
	if err != nil || summary.State != StateStopping {
		t.Fatalf("stop: %+v, %v", summary, err)
	}
	repeated, err := m.Stop("cleanup")
	if err != nil || repeated.Revision != summary.Revision {
		t.Fatalf("repeated stop: %+v, %v", repeated, err)
	}
	awaitSignal(t, cancelled)
	_, err = m.Start(Request{Resource: &testResource{observer: inspectorFunc(quietInspector), run: func(context.Context) error { return nil }}})
	if !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity released before cleanup: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := m.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("premature shutdown completion: %v", err)
	}
	if state, _ := m.Get("cleanup"); state.State != StateStopping {
		t.Fatalf("state during cleanup: %+v", state)
	}
}

func TestOwnerCancellationPropagates(t *testing.T) {
	owner, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	m := newTestManager(t, owner, Config{})
	started := make(chan struct{})
	_, err := m.Start(Request{ID: "owned", Resource: &testResource{
		observer: inspectorFunc(quietInspector), run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	awaitSignal(t, started)
	cause := errors.New("parent task stopped")
	cancel(cause)
	final := waitExecution(t, m, "owned")
	if final.State != StateCancelled || final.CancelReason != cause.Error() {
		t.Fatalf("owner cancellation: %+v", final)
	}
	_, err = m.Start(Request{Resource: &testResource{observer: inspectorFunc(quietInspector), run: func(context.Context) error { t.Error("started after owner cancellation"); return nil }}})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("owner closure: %v", err)
	}
}

func TestStopAllIsLocalAndTerminalRecordsAreStable(t *testing.T) {
	m1 := newTestManager(t, context.Background(), Config{})
	m2 := newTestManager(t, context.Background(), Config{})
	start := func(m *Manager, id string) <-chan struct{} {
		started := make(chan struct{})
		_, err := m.Start(Request{ID: id, Resource: &testResource{observer: inspectorFunc(quietInspector), run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}}})
		if err != nil {
			t.Fatal(err)
		}
		return started
	}
	awaitSignal(t, start(m1, "same-id"))
	awaitSignal(t, start(m2, "same-id"))
	m1.StopAll()
	final := waitExecution(t, m1, "same-id")
	if final.State != StateCancelled {
		t.Fatalf("stop all: %+v", final)
	}
	if other, _ := m2.Get("same-id"); other.State != StateRunning {
		t.Fatalf("cancelled other manager: %+v", other)
	}
	stopped, err := m1.Stop("same-id")
	if err != nil || stopped.Revision != final.Revision {
		t.Fatalf("terminal record changed: %+v, %v", stopped, err)
	}
	_, err = m1.Start(Request{ID: "next", Resource: &testResource{observer: inspectorFunc(quietInspector), run: func(context.Context) error { return nil }}})
	if err != nil {
		t.Fatalf("stop_all closed manager: %v", err)
	}
	waitExecution(t, m1, "next")
}

func TestInspectionDoesNotHoldManagerLock(t *testing.T) {
	m := newTestManager(t, context.Background(), Config{})
	inspecting := make(chan struct{})
	_, err := m.Start(Request{ID: "blocked-inspector", Resource: &testResource{
		observer: inspectorFunc(func(ctx context.Context) (Observation, error) {
			close(inspecting)
			<-ctx.Done()
			return Observation{}, ctx.Err()
		}),
		run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inspectDone := make(chan struct{})
	go func() { defer close(inspectDone); _, _ = m.Inspect(ctx, "blocked-inspector") }()
	awaitSignal(t, inspecting)
	stopDone := make(chan struct{})
	go func() { defer close(stopDone); _, _ = m.Stop("blocked-inspector") }()
	awaitSignal(t, stopDone)
	cancel()
	awaitSignal(t, inspectDone)
	waitExecution(t, m, "blocked-inspector")
}

func TestMetadataCopiesAndAdmissionErrors(t *testing.T) {
	m := newTestManager(t, context.Background(), Config{MaxExecutions: 1})
	attributes := map[string]string{"task_id": "original"}
	resource := &testResource{observer: inspectorFunc(quietInspector), run: func(context.Context) error { return nil }}
	receipt, err := m.Start(Request{ID: "copy", Metadata: Metadata{Attributes: attributes}, Resource: resource})
	if err != nil {
		t.Fatal(err)
	}
	attributes["task_id"] = "mutated"
	receipt.Metadata.Attributes["task_id"] = "receipt changed"
	final := waitExecution(t, m, "copy")
	final.Metadata.Attributes["task_id"] = "snapshot changed"
	*final.EndedAt = time.Time{}
	actual, _ := m.Get("copy")
	if actual.Metadata.Attributes["task_id"] != "original" || actual.EndedAt.IsZero() {
		t.Fatalf("mutable metadata escaped: %+v", actual)
	}
	if _, err := m.Start(Request{ID: "copy", Resource: resource}); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := m.Start(Request{Resource: resource}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("record limit: %v", err)
	}
	if _, err := m.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if _, err := m.Stop("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stop missing: %v", err)
	}
	if _, err := m.Inspect(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inspect missing: %v", err)
	}
}

func TestFailurePanicAndInspectionError(t *testing.T) {
	for _, test := range []struct {
		name  string
		run   func(context.Context) error
		state State
	}{
		{"failure", func(context.Context) error { return errors.New("run failed") }, StateFailed},
		{"panic", func(context.Context) error { panic("broken resource") }, StateFailed},
		{"deadline", func(context.Context) error { return context.DeadlineExceeded }, StateTimedOut},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := newTestManager(t, context.Background(), Config{})
			_, err := m.Start(Request{ID: test.name, Resource: &testResource{
				run: test.run, observer: inspectorFunc(func(context.Context) (Observation, error) { return Observation{}, errors.New("inspection unavailable") }),
			}})
			if err != nil {
				t.Fatal(err)
			}
			final := waitExecution(t, m, test.name)
			if final.State != test.state || final.Error == "" {
				t.Fatalf("failure: %+v", final)
			}
			snapshot, err := m.Inspect(context.Background(), test.name)
			if err == nil || snapshot.State != test.state {
				t.Fatalf("lost lifecycle on inspector failure: %+v, %v", snapshot, err)
			}
		})
	}
}

func TestInvalidResourcesAndCancelledOwner(t *testing.T) {
	if _, err := New(nil, Config{}); err == nil {
		t.Fatal("accepted nil owner")
	}
	if _, err := New(context.Background(), Config{MaxConcurrent: -1}); err == nil {
		t.Fatal("accepted invalid limits")
	}
	m := newTestManager(t, context.Background(), Config{})
	var typedNil *testResource
	for _, resource := range []Resource{nil, typedNil, &testResource{observer: nil}} {
		if _, err := m.Start(Request{Resource: resource}); !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid resource: %v", err)
		}
	}
	owner, cancel := context.WithCancel(context.Background())
	cancel()
	closed := newTestManager(t, owner, Config{})
	if _, err := closed.Start(Request{Resource: &testResource{observer: inspectorFunc(quietInspector), run: func(context.Context) error {
		t.Error("executed with cancelled owner")
		return nil
	}}}); !errors.Is(err, ErrClosed) {
		t.Fatalf("cancelled owner accepted execution: %v", err)
	}
}

func TestConcurrentControlAndObservation(t *testing.T) {
	m := newTestManager(t, context.Background(), Config{MaxConcurrent: 16})
	for i := 0; i < 8; i++ {
		_, err := m.Start(Request{Resource: &testResource{observer: inspectorFunc(quietInspector), run: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	ids := m.List()
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for _, item := range ids {
				_, _ = m.Get(item.ID)
				_, _ = m.Inspect(context.Background(), item.ID)
				_, _ = m.Stop(item.ID)
				m.List()
			}
		}()
	}
	workers.Wait()
	for _, item := range ids {
		if final := waitExecution(t, m, item.ID); !final.State.IsTerminal() {
			t.Fatalf("not terminal: %+v", final)
		}
	}
}
