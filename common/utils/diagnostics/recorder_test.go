package diagnostics

import (
	"errors"
	"sync"
	"testing"
)

func TestRecorderTracksMeasurements(t *testing.T) {
	rec := NewRecorder()
	if err := rec.Track("capture", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snaps := rec.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected one measurement, got %d", len(snaps))
	}
	if snaps[0].Name != "capture" {
		t.Fatalf("unexpected measurement name %q", snaps[0].Name)
	}
	if snaps[0].Count != 1 {
		t.Fatalf("expected count 1, got %d", snaps[0].Count)
	}
}

func TestRecorderTracksStepErrors(t *testing.T) {
	rec := NewRecorder()
	stepErr := errors.New("step failed")
	if err := rec.Track("fails", func() error { return stepErr }); err == nil || !errors.Is(err, stepErr) {
		t.Fatalf("expected wrapped step error, got %v", err)
	}
	snaps := rec.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected measurement entry on error, got %d", len(snaps))
	}
	if snaps[0].Count != 0 {
		t.Fatalf("expected zero count when step fails, got %d", snaps[0].Count)
	}
	if snaps[0].ErrorCount != 1 {
		t.Fatalf("expected recorded error, got %d", snaps[0].ErrorCount)
	}
}

func TestRecorderSnapshotAndReset(t *testing.T) {
	rec := NewRecorder()
	rec.Track("foo", func() error { return nil })
	if len(rec.Snapshot()) != 1 {
		t.Fatalf("expected measurements before reset")
	}
	rec.Reset()
	if len(rec.Snapshot()) != 0 {
		t.Fatalf("expected no measurements after reset")
	}
}

func TestRecorderTrackDisabledRunsSteps(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	run := false
	if err := rec.Track("noop", func() error {
		run = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error when disabled: %v", err)
	}
	if !run {
		t.Fatalf("expected step to run even when recording disabled")
	}
	if len(rec.Snapshot()) != 0 {
		t.Fatalf("expected no recorded measurements when disabled")
	}
}

func TestRecorderConcurrentTrackCount(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelLow)

	rec := NewRecorder()
	const workers = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if err := rec.Track("parallel",
				func() error { return nil },
				func() error { return nil },
			); err != nil {
				t.Errorf("track failed: %v", err)
			}
		}()
	}
	wg.Wait()

	snaps := rec.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected one measurement, got %d", len(snaps))
	}
	if got := snaps[0].Count; got != workers {
		t.Fatalf("expected count %d, got %d", workers, got)
	}
	if got := len(snaps[0].Steps); got < 2 {
		t.Fatalf("expected at least 2 steps, got %d", got)
	}
}

func TestRecorderConcurrentStepExpansion(t *testing.T) {
	rec := NewRecorder()
	const workers = 120

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			stepCount := 1 + (i % 8)
			steps := make([]func() error, stepCount)
			for j := range steps {
				steps[j] = func() error { return nil }
			}
			if err := rec.track(true, "expand", steps...); err != nil {
				t.Errorf("track failed: %v", err)
			}
		}(i)
	}
	wg.Wait()

	snaps := rec.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected one measurement, got %d", len(snaps))
	}
	if got := snaps[0].Count; got != workers {
		t.Fatalf("expected count %d, got %d", workers, got)
	}
	if got := len(snaps[0].Steps); got != 8 {
		t.Fatalf("expected expanded steps len 8, got %d", got)
	}
}
