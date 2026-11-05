package state

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCleanupAndClose(t *testing.T) {
	start := time.Unix(10, 0)
	clock := newFakeClock(start)
	cfg := testConfig()
	cfg.FlowIdleTimeout = time.Second
	store, err := NewStore(cfg, clock)
	if err != nil {
		t.Fatal(err)
	}
	key := testFlow(1, 1, 1)
	if err := store.SetFlowBit(key, "bit"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetXBit(FlowXBitKey(key, "x"), time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CheckThreshold(GlobalThresholdKey(1, testDomain(1), 1), ThresholdSpec{Mode: ThresholdLimit, Count: 1, Window: time.Second}); err != nil {
		t.Fatal(err)
	}
	clock.Set(start.Add(time.Second - time.Nanosecond))
	if removed, err := store.Cleanup(time.Time{}); err != nil || removed != 0 {
		t.Fatalf("early cleanup removed=%d err=%v", removed, err)
	}
	clock.Set(start.Add(time.Second))
	if removed, err := store.Cleanup(time.Time{}); err != nil || removed != 3 {
		t.Fatalf("boundary cleanup removed=%d err=%v", removed, err)
	}
	stats := store.Stats()
	if stats.Flows != 0 || stats.XBits != 0 || stats.Thresholds != 0 || stats.Cleaned != 3 {
		t.Fatalf("stats after cleanup: %+v", stats)
	}

	if err := store.SetFlowBit(key, "bit"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	stats = store.Stats()
	if !stats.Closed || stats.Flows != 0 || stats.FlowBits != 0 {
		t.Fatalf("stats after close: %+v", stats)
	}
	if err := store.SetFlowBit(key, "bit"); !errors.Is(err, ErrClosed) {
		t.Fatalf("post-close err=%v", err)
	}
	if _, err := store.Cleanup(time.Time{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("post-close cleanup err=%v", err)
	}
}

func TestCloseWaitsForTransaction(t *testing.T) {
	store := testStore(t, nil)
	key := testFlow(1, 1, 1)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- store.WithFlow(key, func(flow *FlowState) error {
			close(entered)
			<-release
			return flow.SetBit("committed")
		})
	}()
	<-entered
	closeDone := make(chan struct{})
	go func() {
		_ = store.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned while transaction was active")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish")
	}
}

func TestConcurrentHighCardinalityLimits(t *testing.T) {
	cfg := testConfig()
	cfg.MaxFlows = 10
	cfg.MaxXBits = 10
	store, err := NewStore(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const attempts = 100
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := testFlow(1, 1, uint64(i+1))
			_ = store.SetFlowBit(key, "b")
			_ = store.SetXBit(FlowXBitKey(key, "x"), 0)
		}(i)
	}
	wg.Wait()
	stats := store.Stats()
	if stats.Flows != cfg.MaxFlows || stats.XBits != cfg.MaxXBits {
		t.Fatalf("limits exceeded or underfilled: %+v", stats)
	}
	if stats.Rejected == 0 {
		t.Fatalf("expected rejected resources: %+v", stats)
	}
}
