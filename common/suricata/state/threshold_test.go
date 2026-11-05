package state

import (
	"errors"
	"testing"
	"time"
)

func TestThresholdModes(t *testing.T) {
	tests := []struct {
		name string
		mode ThresholdMode
		want []bool
	}{
		{name: "limit", mode: ThresholdLimit, want: []bool{true, true, false, false}},
		{name: "threshold", mode: ThresholdEvery, want: []bool{false, true, false, true}},
		{name: "both", mode: ThresholdBoth, want: []bool{false, true, false, false}},
		{name: "detection-filter", mode: DetectionFilter, want: []bool{false, false, true, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := newFakeClock(time.Unix(100, 0))
			store := testStore(t, clock)
			key := GlobalThresholdKey(1, testDomain(1), 99)
			spec := ThresholdSpec{Mode: tt.mode, Count: 2, Window: time.Minute}
			for i, want := range tt.want {
				got, err := store.CheckThreshold(key, spec)
				if err != nil {
					t.Fatal(err)
				}
				if got.Alert != want || got.Count != uint64(i+1) {
					t.Fatalf("event %d got=%+v want alert=%v", i+1, got, want)
				}
			}
		})
	}
}

func TestThresholdWindowBoundaryAndTimeRollback(t *testing.T) {
	start := time.Unix(1_000, 0)
	clock := newFakeClock(start)
	store := testStore(t, clock)
	key := GlobalThresholdKey(1, testDomain(1), 1)
	spec := ThresholdSpec{Mode: ThresholdLimit, Count: 1, Window: time.Second}

	result, err := store.CheckThresholdAt(key, start, spec)
	if err != nil || result.Count != 1 || !result.Alert {
		t.Fatalf("first result=%+v err=%v", result, err)
	}
	clock.Set(start.Add(24 * time.Hour))
	result, err = store.CheckThresholdAt(key, start.Add(time.Second-time.Nanosecond), spec)
	if err != nil || result.Count != 2 || result.Alert {
		t.Fatalf("end-1ns result=%+v err=%v", result, err)
	}
	result, err = store.CheckThresholdAt(key, start.Add(time.Second), spec)
	if err != nil || result.Count != 1 || !result.Alert || !result.WindowStart.Equal(start.Add(time.Second)) {
		t.Fatalf("exact end result=%+v err=%v", result, err)
	}
	result, err = store.CheckThresholdAt(key, start.Add(time.Second-time.Nanosecond), spec)
	if err != nil || result.Count != 2 || result.Alert {
		t.Fatalf("rollback result=%+v err=%v", result, err)
	}
	result, err = store.CheckThresholdAt(key, start.Add(2*time.Second+time.Nanosecond), spec)
	if err != nil || result.Count != 1 || !result.WindowStart.Equal(start.Add(2*time.Second+time.Nanosecond)) {
		t.Fatalf("end+1ns result=%+v err=%v", result, err)
	}
}

func TestThresholdEpochIsolationSpecAndCapacity(t *testing.T) {
	cfg := testConfig()
	cfg.MaxThresholds = 2
	store, err := NewStore(cfg, newFakeClock(time.Unix(1, 0)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	spec := ThresholdSpec{Mode: ThresholdLimit, Count: 1, Window: time.Second}
	for epoch := uint64(1); epoch <= 2; epoch++ {
		result, err := store.CheckThreshold(GlobalThresholdKey(epoch, testDomain(1), 1), spec)
		if err != nil || result.Count != 1 {
			t.Fatalf("epoch %d result=%+v err=%v", epoch, result, err)
		}
	}
	if _, err := store.CheckThreshold(GlobalThresholdKey(3, testDomain(1), 1), spec); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity err=%v", err)
	}
	if _, err := store.CheckThreshold(GlobalThresholdKey(1, testDomain(1), 1), ThresholdSpec{Mode: ThresholdLimit, Count: 2, Window: time.Second}); !errors.Is(err, ErrSpecMismatch) {
		t.Fatalf("spec mismatch err=%v", err)
	}
}
