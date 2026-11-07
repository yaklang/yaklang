package state

import (
	"testing"
	"time"
)

func TestThresholdCapacityReclaimsExpiredWindows(t *testing.T) {
	start := time.Unix(100, 0)
	clock := newFakeClock(start)
	cfg := testConfig()
	cfg.MaxThresholds = 1
	store, err := NewStore(cfg, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	spec := ThresholdSpec{Mode: ThresholdLimit, Count: 1, Window: time.Second}
	if _, err := store.CheckThreshold(GlobalThresholdKey(1, testDomain(1), 1), spec); err != nil {
		t.Fatal(err)
	}
	clock.Set(start.Add(time.Second))
	if _, err := store.CheckThreshold(GlobalThresholdKey(1, testDomain(1), 2), spec); err != nil {
		t.Fatalf("expired threshold blocked a new key: %v", err)
	}
	stats := store.Stats()
	if stats.Thresholds != 1 || stats.Cleaned != 1 {
		t.Fatalf("unexpected stats after reclaim: %+v", stats)
	}
}
