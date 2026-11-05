package state

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFlowBitsIntsAndOverflow(t *testing.T) {
	store := testStore(t, nil)
	key := testFlow(1, 1, 1)

	if err := store.SetFlowBit(key, "ready"); err != nil {
		t.Fatal(err)
	}
	if set, err := store.ToggleFlowBit(key, "ready"); err != nil || set {
		t.Fatalf("toggle set=%v err=%v", set, err)
	}
	if set, err := store.ToggleFlowBit(key, "ready"); err != nil || !set {
		t.Fatalf("toggle set=%v err=%v", set, err)
	}
	if err := store.UnsetFlowBit(key, "ready"); err != nil {
		t.Fatal(err)
	}

	if err := store.SetFlowInt(key, "score", 40); err != nil {
		t.Fatal(err)
	}
	if got, err := store.AddFlowInt(key, "score", 2); err != nil || got != 42 {
		t.Fatalf("add got=%d err=%v", got, err)
	}
	if got, err := store.CompareFlowInt(key, "score", IntGreaterOrEqual, 42); err != nil || !got {
		t.Fatalf("compare got=%v err=%v", got, err)
	}

	if err := store.SetFlowInt(key, "max", math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if got, err := store.AddFlowInt(key, "max", 1); !errors.Is(err, ErrOverflow) || got != math.MaxInt64 {
		t.Fatalf("positive overflow got=%d err=%v", got, err)
	}
	if err := store.SetFlowInt(key, "min", math.MinInt64); err != nil {
		t.Fatal(err)
	}
	if got, err := store.AddFlowInt(key, "min", -1); !errors.Is(err, ErrOverflow) || got != math.MinInt64 {
		t.Fatalf("negative overflow got=%d err=%v", got, err)
	}
}

func TestEvaluateAtomicPredicateEffectsAndRollback(t *testing.T) {
	store := testStore(t, nil)
	key := testFlow(1, 1, 1)
	if err := store.SetFlowInt(key, "value", 1); err != nil {
		t.Fatal(err)
	}

	matched, err := store.Evaluate(key,
		func(view FlowView) (bool, error) {
			return view.CompareInt("value", IntEqual, 1)
		},
		func(flow *FlowState) error { return flow.SetBit("effect") },
		func(flow *FlowState) error {
			_, err := flow.AddInt("value", 1)
			return err
		},
	)
	if err != nil || !matched {
		t.Fatalf("matched=%v err=%v", matched, err)
	}

	boom := errors.New("boom")
	matched, err = store.Evaluate(key,
		func(FlowView) (bool, error) { return true, nil },
		func(flow *FlowState) error { return flow.SetBit("rolled-back") },
		func(*FlowState) error { return boom },
	)
	if matched || !errors.Is(err, boom) {
		t.Fatalf("matched=%v err=%v", matched, err)
	}
	if got, err := store.TestFlowBit(key, "rolled-back"); err != nil || got {
		t.Fatalf("failed effects committed: got=%v err=%v", got, err)
	}

	matched, err = store.Evaluate(key, func(FlowView) (bool, error) { return false, nil }, func(flow *FlowState) error {
		return flow.SetBit("must-not-run")
	})
	if err != nil || matched {
		t.Fatalf("false predicate matched=%v err=%v", matched, err)
	}
}

func TestConcurrentToggleAndAdd(t *testing.T) {
	store := testStore(t, nil)
	key := testFlow(1, 1, 1)
	const goroutines = 32
	const iterations = 1_000
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				if _, err := store.ToggleFlowBit(key, "toggle"); err != nil {
					t.Errorf("toggle: %v", err)
					return
				}
				if _, err := store.AddFlowInt(key, "counter", 1); err != nil {
					t.Errorf("add: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got, exists, err := store.GetFlowInt(key, "counter"); err != nil || !exists || got != goroutines*iterations {
		t.Fatalf("counter got=%d exists=%v err=%v", got, exists, err)
	}
	if got, err := store.TestFlowBit(key, "toggle"); err != nil || got {
		t.Fatalf("even toggles got=%v err=%v", got, err)
	}
}

func TestDifferentFlowsRunConcurrently(t *testing.T) {
	store := testStore(t, nil)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	fn := func(*FlowState) error {
		current := active.Add(1)
		for {
			old := maxActive.Load()
			if current <= old || maxActive.CompareAndSwap(old, current) {
				break
			}
		}
		entered <- struct{}{}
		<-release
		active.Add(-1)
		return nil
	}
	var wg sync.WaitGroup
	for _, key := range []FlowKey{testFlow(1, 1, 1), testFlow(1, 1, 2)} {
		wg.Add(1)
		go func(key FlowKey) {
			defer wg.Done()
			if err := store.WithFlow(key, fn); err != nil {
				t.Errorf("WithFlow: %v", err)
			}
		}(key)
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("different flows did not enter concurrently")
		}
	}
	close(release)
	wg.Wait()
	if maxActive.Load() != 2 {
		t.Fatalf("max active = %d", maxActive.Load())
	}
}

func TestFlowLifecycleAndExplicitCaptureTime(t *testing.T) {
	start := time.Unix(100, 0)
	clock := newFakeClock(start.Add(24 * time.Hour))
	store := testStore(t, clock)
	key := testFlow(1, 1, 1)
	if err := store.WithFlowAt(key, start, func(flow *FlowState) error {
		return flow.SetBit("seen")
	}); err != nil {
		t.Fatal(err)
	}
	matched, err := store.EvaluateAt(key, start.Add(time.Second), func(view FlowView) (bool, error) {
		return view.TestBit("seen")
	}, func(flow *FlowState) error {
		return flow.SetInt("count", 1)
	})
	if err != nil || !matched {
		t.Fatalf("EvaluateAt matched=%v err=%v", matched, err)
	}
	if err := store.WithFlowAt(key, start.Add(-time.Second), func(flow *FlowState) error {
		return flow.SetBit("later")
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, ok, err := store.SnapshotFlow(key)
	if err != nil || !ok {
		t.Fatalf("snapshot ok=%v err=%v", ok, err)
	}
	if !snapshot.CreatedAt.Equal(start) || !snapshot.UpdatedAt.Equal(start.Add(time.Second)) {
		t.Fatalf("unexpected lifecycle times: created=%v updated=%v", snapshot.CreatedAt, snapshot.UpdatedAt)
	}
	if deleted, err := store.DeleteFlow(key); err != nil || !deleted {
		t.Fatalf("delete deleted=%v err=%v", deleted, err)
	}
	if _, ok, err := store.SnapshotFlow(key); err != nil || ok {
		t.Fatalf("deleted flow ok=%v err=%v", ok, err)
	}
	if deleted, err := store.DeleteFlow(key); err != nil || deleted {
		t.Fatalf("second delete deleted=%v err=%v", deleted, err)
	}
}

func TestFlowResourceLimitsAndNameLimit(t *testing.T) {
	cfg := testConfig()
	cfg.MaxFlows = 2
	cfg.MaxFlowBitsPerFlow = 2
	cfg.MaxFlowIntsPerFlow = 1
	cfg.MaxNameLength = 4
	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key := testFlow(1, 1, 1)
	if err := store.SetFlowBit(key, "aaaa"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFlowBit(key, "bbbb"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFlowBit(key, "cccc"); !errors.Is(err, ErrPerFlowLimit) {
		t.Fatalf("bit limit err=%v", err)
	}
	if err := store.SetFlowBit(key, "abcde"); !errors.Is(err, ErrNameTooLong) {
		t.Fatalf("name limit err=%v", err)
	}
	if err := store.SetFlowInt(key, "i", 1); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFlowInt(key, "j", 1); !errors.Is(err, ErrPerFlowLimit) {
		t.Fatalf("int limit err=%v", err)
	}
	if err := store.SetFlowBit(testFlow(1, 1, 2), "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetFlowBit(testFlow(1, 1, 3), "a"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("flow capacity err=%v", err)
	}
}
