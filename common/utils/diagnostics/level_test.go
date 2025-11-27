package diagnostics

import "testing"

func TestTraceLevelSkipsWhenDisabled(t *testing.T) {
	origLevel := GetLevel()
	origRec := ReplaceDefault(NewRecorder())
	defer SetLevel(origLevel)
	defer ReplaceDefault(origRec)

	SetLevel(LevelCritical)
	run := false
	if err := TrackTrace("trace-skip", func() error {
		run = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !run {
		t.Fatalf("expected step to run even if tracing is disabled")
	}
	if snaps := DefaultRecorder().Snapshot(); len(snaps) != 0 {
		t.Fatalf("expected no measurements when level blocks recording, got %d", len(snaps))
	}
}

func TestTraceLevelRecordsWhenAllowed(t *testing.T) {
	origLevel := GetLevel()
	origRec := ReplaceDefault(NewRecorder())
	defer SetLevel(origLevel)
	defer ReplaceDefault(origRec)

	SetLevel(LevelTrace)
	if err := TrackMeasure("measure-allowed", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snaps := DefaultRecorder().Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("expected exactly one measurement, got %d", len(snaps))
	}
	if snaps[0].Name != "measure-allowed" {
		t.Fatalf("expected measurement name capture, got %s", snaps[0].Name)
	}
	if snaps[0].Count != 1 {
		t.Fatalf("expected count 1, got %d", snaps[0].Count)
	}
}

func TestEnabledTierOrdering(t *testing.T) {
	orig := GetLevel()
	defer SetLevel(orig)

	SetLevel(LevelMeasure)
	if Enabled(LevelTrace) {
		t.Fatalf("trace should be disabled when minimum tier is measure")
	}
	if !Enabled(LevelFocus) {
		t.Fatalf("focus should be enabled when minimum tier is measure")
	}
}
