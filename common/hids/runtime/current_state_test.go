//go:build hids && linux

package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

type currentStateProvider struct {
	processes func(context.Context) ([]model.Event, error)
}

func (p currentStateProvider) ListProcessEvents(ctx context.Context) ([]model.Event, error) {
	return p.processes(ctx)
}
func (currentStateProvider) ListNetworkEvents(context.Context) ([]model.Event, error) {
	return nil, nil
}
func (currentStateProvider) ListHostUserEvents(context.Context) ([]model.Event, error) {
	return nil, nil
}

func currentStateManager(provider inventoryProvider) *Manager {
	manager := NewManager()
	manager.instance = &Instance{baselineProvider: provider, events: make(chan model.Event, 256)}
	return manager
}

func TestCurrentStateMissingRuntimeFails(t *testing.T) {
	t.Parallel()
	for _, manager := range []*Manager{nil, NewManager(), currentStateManager(nil)} {
		if err := manager.CollectCurrentState(context.Background(), "processes"); !errors.Is(err, ErrCurrentStateUnavailable) {
			t.Fatalf("expected unavailable runtime, got %v", err)
		}
		if _, err := manager.CollectCurrentStateWithSink(context.Background(), "processes", nil); !errors.Is(err, ErrCurrentStateUnavailable) {
			t.Fatalf("expected unavailable synchronous runtime, got %v", err)
		}
	}
}

func TestCurrentStateCanceledBeforeCollectionDoesNotPublish(t *testing.T) {
	t.Parallel()
	providerCalls, sinkCalls := 0, 0
	manager := currentStateManager(currentStateProvider{processes: func(context.Context) ([]model.Event, error) {
		providerCalls++
		return []model.Event{{Type: model.EventTypeProcessState}}, nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for index := 0; index < 100; index++ {
		count, err := manager.CollectCurrentStateWithSink(ctx, "processes", func(context.Context, model.Event) error {
			sinkCalls++
			return nil
		})
		if count != 0 || !errors.Is(err, context.Canceled) {
			t.Fatalf("expected canceled collection with no delivered records, count=%d err=%v", count, err)
		}
	}
	if providerCalls != 0 || sinkCalls != 0 {
		t.Fatalf("canceled collection invoked provider %d / sink %d times", providerCalls, sinkCalls)
	}
}

func TestCurrentStateSlowProviderCannotReportSuccessAfterDeadline(t *testing.T) {
	t.Parallel()
	manager := currentStateManager(currentStateProvider{processes: func(ctx context.Context) ([]model.Event, error) {
		<-ctx.Done()
		// Model a provider which finishes reading after its deadline but does
		// not report the cancellation itself, as the old inventory did.
		return []model.Event{{Type: model.EventTypeProcessState}}, nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	sinkCalls := 0
	count, err := manager.CollectCurrentStateWithSink(ctx, "processes", func(context.Context, model.Event) error {
		sinkCalls++
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || count != 0 || sinkCalls != 0 {
		t.Fatalf("expected deadline failure before publication, count=%d sink_calls=%d err=%v", count, sinkCalls, err)
	}
}

func TestCurrentStateProviderFailureIsReturned(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("cannot enumerate processes")
	manager := currentStateManager(currentStateProvider{processes: func(context.Context) ([]model.Event, error) {
		return nil, wantErr
	}})
	if err := manager.CollectCurrentState(context.Background(), "processes"); !errors.Is(err, wantErr) {
		t.Fatalf("expected provider failure, got %v", err)
	}
}

func TestCurrentStateSynchronousSinkDeliversLargeInventory(t *testing.T) {
	t.Parallel()
	const size = 10000
	events := make([]model.Event, size)
	for index := range events {
		events[index] = model.Event{Type: model.EventTypeProcessState, Process: &model.Process{PID: index + 1}}
	}
	manager := currentStateManager(currentStateProvider{processes: func(context.Context) ([]model.Event, error) { return events, nil }})
	received := 0
	count, err := manager.CollectCurrentStateWithSink(context.Background(), "processes", func(ctx context.Context, event model.Event) error {
		received++
		if event.Process.PID != received {
			t.Fatalf("out-of-order or missing record: pid=%d expected=%d", event.Process.PID, received)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("publication did not inherit the collection deadline")
		}
		return nil
	})
	if err != nil || count != size || received != size {
		t.Fatalf("incomplete collection: count=%d received=%d err=%v", count, received, err)
	}
	if len(manager.instance.events) != 0 {
		t.Fatal("synchronous inventory leaked into the asynchronous pipeline")
	}
}

func TestCurrentStateKeepsExplicitCollectionBudgetSeparateFromArtifactBudget(t *testing.T) {
	t.Parallel()
	var collectionDeadline time.Time
	manager := currentStateManager(currentStateProvider{processes: func(ctx context.Context) ([]model.Event, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 15*time.Second || time.Until(deadline) > 20*time.Second {
			t.Fatalf("collection inherited the automatic artifact budget: deadline=%v ok=%v", deadline, ok)
		}
		collectionDeadline = deadline
		return []model.Event{{Type: model.EventTypeProcessState, Process: &model.Process{PID: 1, Image: "/unread/image"}}}, nil
	}})
	count, err := manager.CollectCurrentStateWithSink(context.Background(), "processes", func(ctx context.Context, event model.Event) error {
		deadline, _ := ctx.Deadline()
		if !deadline.Equal(collectionDeadline) {
			t.Errorf("sink lost the collection budget: deadline=%v collection=%v", deadline, collectionDeadline)
		}
		if event.Process.Artifact != nil || event.Data["artifact_errors"] != nil {
			t.Error("explicit inventory entered the automatic artifact/pipeline path")
		}
		return nil
	})
	if err != nil || count != 1 {
		t.Fatalf("explicit collection failed: count=%d err=%v", count, err)
	}
}

func TestCurrentStateSinkFailureReturnsAcknowledgedCount(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("durable publish failed")
	manager := currentStateManager(currentStateProvider{processes: func(context.Context) ([]model.Event, error) { return make([]model.Event, 5), nil }})
	calls := 0
	count, err := manager.CollectCurrentStateWithSink(context.Background(), "processes", func(context.Context, model.Event) error {
		calls++
		if calls == 3 {
			return wantErr
		}
		return nil
	})
	if !errors.Is(err, wantErr) || count != 2 || calls != 3 {
		t.Fatalf("incorrect partial publication: count=%d calls=%d err=%v", count, calls, err)
	}
}

func TestCurrentStateCancellationAfterFinalSinkCallIsNotSuccess(t *testing.T) {
	t.Parallel()
	manager := currentStateManager(currentStateProvider{processes: func(context.Context) ([]model.Event, error) {
		return []model.Event{{Type: model.EventTypeProcessState}}, nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count, err := manager.CollectCurrentStateWithSink(ctx, "processes", func(context.Context, model.Event) error {
		cancel()
		return nil
	})
	if count != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected acknowledged count with cancellation error, count=%d err=%v", count, err)
	}
}

func TestCurrentStateBlockedPublicationTimesOut(t *testing.T) {
	t.Parallel()
	manager := currentStateManager(currentStateProvider{processes: func(context.Context) ([]model.Event, error) { return make([]model.Event, 714), nil }})
	manager.instance.events = make(chan model.Event, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := manager.CollectCurrentState(ctx, "processes"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected partial enqueue failure on deadline, got %v", err)
	}
}

func TestPublishInventoryEventsAlreadyCanceledDoesNotSend(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sink := make(chan model.Event, 256)
	for index := 0; index < 100; index++ {
		if err := publishInventoryEvents(ctx, sink, make([]model.Event, 714)); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	}
	if len(sink) != 0 {
		t.Fatalf("published %d events after cancellation", len(sink))
	}
}
