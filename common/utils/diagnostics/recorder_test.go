package diagnostics

import (
	"errors"
	"testing"
)

func TestRecorderTracksMeasurements(t *testing.T) {
	rec := NewRecorder()
	if err := rec.Track(true, "capture", func() error { return nil }); err != nil {
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
	if err := rec.Track(true, "fails", func() error { return stepErr }); err == nil || !errors.Is(err, stepErr) {
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
	rec.Track(true, "foo", func() error { return nil })
	if len(rec.Snapshot()) != 1 {
		t.Fatalf("expected measurements before reset")
	}
	rec.Reset()
	if len(rec.Snapshot()) != 0 {
		t.Fatalf("expected no measurements after reset")
	}
}

func TestRecorderTrackDisabledRunsSteps(t *testing.T) {
	rec := NewRecorder()
	run := false
	if err := rec.Track(false, "noop", func() error {
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
