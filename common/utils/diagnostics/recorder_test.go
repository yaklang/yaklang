package diagnostics

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRecorderTracksMeasurements(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	if _, err := rec.TrackHigh(TrackKindGeneral, "capture", func() error { return nil }); err != nil {
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
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	stepErr := errors.New("step failed")
	if _, err := rec.TrackHigh(TrackKindGeneral, "fails", func() error { return stepErr }); err == nil || !errors.Is(err, stepErr) {
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
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	_, _ = rec.TrackHigh(TrackKindGeneral, "foo", func() error { return nil })
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
	if _, err := rec.Track(TrackKindGeneral, "noop", func() error {
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
	SetLevel(LevelHigh)

	rec := NewRecorder()
	const workers = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if _, err := rec.TrackHigh(TrackKindGeneral, "parallel",
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

// TestRecorderSnapshotPreservesKind 断言 Snapshot 保留每条 measurement 的 Kind 标签
func TestRecorderSnapshotPreservesKind(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	_, _ = rec.TrackHigh(TrackKind("AST"), "AST[main.yak]", func() error { return nil })
	_, _ = rec.TrackHigh(TrackKind("Build"), "LazyBuild", func() error { return nil })
	_, _ = rec.TrackHigh(TrackKind("Database"), "ssadb.yieldIrCodes", func() error { return nil })
	_, _ = rec.TrackHigh(TrackKind("Scan"), "Rule foo", func() error { return nil })
	_, _ = rec.TrackHigh(TrackKind("StaticAnalyze"), "Rule Check", func() error { return nil })
	_, _ = rec.TrackHigh(TrackKindGeneral, "generic", func() error { return nil })

	snap := rec.Snapshot()
	assertKind := func(name string, want TrackKind) {
		for _, m := range snap {
			if m.Name == name {
				if m.Kind != want {
					t.Fatalf("measurement %q: want Kind %q, got %q", name, want, m.Kind)
				}
				return
			}
		}
		t.Fatalf("measurement %q not found in snapshot", name)
	}
	assertKind("AST[main.yak]", TrackKind("AST"))
	assertKind("LazyBuild", TrackKind("Build"))
	assertKind("ssadb.yieldIrCodes", TrackKind("Database"))
	assertKind("Rule foo", TrackKind("Scan"))
	assertKind("Rule Check", TrackKind("StaticAnalyze"))
	assertKind("generic", TrackKindGeneral)
}

// TestTrackLowRecordsAtLevelLow 断言 LevelLow 时 TrackLow 记录
func TestTrackLowRecordsAtLevelLow(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelLow)

	rec := NewRecorder()
	if _, err := rec.TrackLow(TrackKind("Database"), "ssadb.low", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one measurement at LevelLow, got %d", len(snap))
	}
	if snap[0].Kind != TrackKind("Database") {
		t.Fatalf("expected Kind Database, got %q", snap[0].Kind)
	}
	if snap[0].Name != "ssadb.low" {
		t.Fatalf("expected name ssadb.low, got %q", snap[0].Name)
	}
}

// TestTrackLowSkipsAtLevelNormal 断言 LevelNormal 时 TrackLow 不记录
func TestTrackLowSkipsAtLevelNormal(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelNormal)

	rec := NewRecorder()
	if _, err := rec.TrackLow(TrackKindGeneral, "low-skip", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.Snapshot()) != 0 {
		t.Fatalf("expected no recordings at LevelNormal for TrackLow, got %d", len(rec.Snapshot()))
	}
}

// TestTrackLowSkipsAtLevelHigh 断言 LevelHigh 时 TrackLow 不记录
func TestTrackLowSkipsAtLevelHigh(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	if _, err := rec.TrackLow(TrackKindGeneral, "low-skip-high", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.Snapshot()) != 0 {
		t.Fatalf("expected no recordings at LevelHigh for TrackLow, got %d", len(rec.Snapshot()))
	}
}

// TestTrackHighRecordsAtLevelHigh 断言 LevelHigh 时 TrackHigh 记录
func TestTrackHighRecordsAtLevelHigh(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

	rec := NewRecorder()
	if _, err := rec.TrackHigh(TrackKind("Scan"), "Rule critical", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	snap := rec.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one measurement at LevelHigh, got %d", len(snap))
	}
	if snap[0].Kind != TrackKind("Scan") {
		t.Fatalf("expected Kind Scan, got %q", snap[0].Kind)
	}
	if snap[0].Name != "Rule critical" {
		t.Fatalf("expected name Rule critical, got %q", snap[0].Name)
	}
}

// TestTrackHighRecordsAtLevelNormal 断言 LevelNormal 时 TrackHigh 也记录
func TestTrackHighRecordsAtLevelNormal(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelNormal)

	rec := NewRecorder()
	if _, err := rec.TrackHigh(TrackKind("Scan"), "Rule normal", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.Snapshot()) != 1 {
		t.Fatalf("expected one measurement at LevelNormal for TrackHigh, got %d", len(rec.Snapshot()))
	}
}

// TestTrackHighRecordsAtLevelLow 断言 LevelLow 时 TrackHigh 也记录
func TestTrackHighRecordsAtLevelLow(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelLow)

	rec := NewRecorder()
	if _, err := rec.TrackHigh(TrackKind("StaticAnalyze"), "Rule Check", func() error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.Snapshot()) != 1 {
		t.Fatalf("expected one measurement at LevelLow for TrackHigh, got %d", len(rec.Snapshot()))
	}
}

// TestTrackHighSkipsAtLevelOff 断言 LevelOff 时 TrackHigh 不记录
func TestTrackHighSkipsAtLevelOff(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelOff)

	rec := NewRecorder()
	run := false
	if _, err := rec.TrackHigh(TrackKind("Scan"), "high-off", func() error {
		run = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !run {
		t.Fatalf("expected step to run even when LevelOff")
	}
	if len(rec.Snapshot()) != 0 {
		t.Fatalf("expected no recordings at LevelOff for TrackHigh, got %d", len(rec.Snapshot()))
	}
}

// TestLevelAPIBehaviorMatrix 断言 Level × API 行为矩阵
func TestLevelAPIBehaviorMatrix(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)

	tests := []struct {
		level  Level
		track  bool // Track records?
		trackLow bool
		trackHigh bool
	}{
		{LevelLow, true, true, true},
		{LevelNormal, true, false, true},
		{LevelHigh, false, false, true},
		{LevelOff, false, false, false},
	}

	for _, tt := range tests {
		SetLevel(tt.level)
		rec := NewRecorder()
		_, _ = rec.Track(TrackKindGeneral, "t", func() error { return nil })
		_, _ = rec.TrackLow(TrackKindGeneral, "tl", func() error { return nil })
		_, _ = rec.TrackHigh(TrackKindGeneral, "th", func() error { return nil })

		snap := rec.Snapshot()
		gotTrack, gotLow, gotHigh := false, false, false
		for _, m := range snap {
			switch m.Name {
			case "t":
				gotTrack = true
			case "tl":
				gotLow = true
			case "th":
				gotHigh = true
			}
		}
		if gotTrack != tt.track || gotLow != tt.trackLow || gotHigh != tt.trackHigh {
			t.Errorf("level=%v: want Track=%v TrackLow=%v TrackHigh=%v, got Track=%v TrackLow=%v TrackHigh=%v",
				tt.level, tt.track, tt.trackLow, tt.trackHigh, gotTrack, gotLow, gotHigh)
		}
	}
}

// TestFormatPerformanceTableEmpty 断言空数据时的占位输出
func TestFormatPerformanceTableEmpty(t *testing.T) {
	out := FormatTable("My Title", nil, nil)
	if !strings.Contains(out, "No data for: My Title") {
		t.Fatalf("empty table should contain placeholder; got: %q", out)
	}
	_, rows := MeasurementsToRows([]Measurement{})
	out2 := FormatTable("Other", []string{"Name"}, rows)
	if !strings.Contains(out2, "No data for: Other") {
		t.Fatalf("empty rows should contain placeholder; got: %q", out2)
	}
}

// TestFormatPerformanceTableWithoutSize 断言无 Size 时表格格式：仅 Name、Duration 两列，不含 Size/ms/KB
func TestFormatPerformanceTableWithoutSize(t *testing.T) {
	ms := []Measurement{
		{Name: "foo", Total: 5 * time.Millisecond, Count: 1},
		{Name: "bar", Total: 10 * time.Millisecond, Count: 1},
	}
	headers, rows := MeasurementsToRows(ms)
	out := FormatTable("Simple", headers, rows)
	if !strings.Contains(out, "Name") || !strings.Contains(out, "Duration") {
		t.Fatalf("table must have Name and Duration headers; got: %s", out)
	}
	if strings.Contains(out, "Size") || strings.Contains(out, "ms/KB") {
		t.Fatalf("when no Size data, table should omit Size and ms/KB columns; got: %s", out)
	}
	if !strings.Contains(out, "foo") || !strings.Contains(out, "bar") {
		t.Fatalf("table must contain measurement names")
	}
	if !strings.Contains(out, "Simple") {
		t.Fatalf("table must contain title")
	}
}

// TestFormatPerformanceTableWithSize 断言有 Size 时表格格式：Name、Duration、Size、ms/KB
func TestFormatPerformanceTableWithSize(t *testing.T) {
	ms := []Measurement{
		{Name: "AST[a.yak]", Total: 5 * time.Millisecond, Count: 1, Size: 1024},
		{Name: "Build[b.yak]", Total: 20 * time.Millisecond, Count: 1, Size: 512},
	}
	headers, rows := MeasurementsToRows(ms) // 有 Size 时自动包含 Size、ms/KB 列
	out := FormatTable("File Summary", headers, rows)
	if !strings.Contains(out, "Name") || !strings.Contains(out, "Duration") {
		t.Fatalf("table must have Name and Duration")
	}
	if !strings.Contains(out, "Size") || !strings.Contains(out, "ms/KB") {
		t.Fatalf("with Size, table must have Size and ms/KB columns")
	}
	if !strings.Contains(out, "a.yak") || !strings.Contains(out, "b.yak") {
		t.Fatalf("table must contain measurement names")
	}
	// ms/KB for 5ms/1KB=5, 20ms/0.5KB=40
	if !strings.Contains(out, "5.00") || !strings.Contains(out, "40.00") {
		t.Fatalf("table must show ms/KB ratio values")
	}
}

// TestMeasurementsToRowsIncludeSizeOpt 断言 TableIncludeSize(true) 可强制包含 Size 列
func TestMeasurementsToRowsIncludeSizeOpt(t *testing.T) {
	ms := []Measurement{
		{Name: "rule1", Total: 100 * time.Millisecond, Count: 1},
	}
	headers, rows := MeasurementsToRows(ms, TableIncludeSize(true))
	if !strings.Contains(strings.Join(headers, " "), "Size") || !strings.Contains(strings.Join(headers, " "), "ms/KB") {
		t.Fatalf("with IncludeSize(true), must have Size and ms/KB columns")
	}
	if len(rows) != 1 || len(rows[0]) != 4 {
		t.Fatalf("expected 4 cells per row, got %d", len(rows[0]))
	}
}

// TestFormatTableWithOptions 断言 FormatTable 支持 TableOption
func TestFormatTableWithOptions(t *testing.T) {
	headers := []string{"A", "B"}
	rows := [][]string{{"short", "x"}, {"very_long_value", "y"}}
	out := FormatTable("Opts", headers, rows, TableCellMaxWidth(10))
	if out == "" || !strings.Contains(out, "Opts") {
		t.Fatalf("FormatTable with opts should produce output")
	}
	if !strings.Contains(out, "...") {
		t.Logf("cellMaxWidth=10 may truncate; output: %s", out)
	}
}

func TestRecorderConcurrentStepExpansion(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)
	SetLevel(LevelHigh)

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
			if _, err := rec.trackWithDuration(true, true, TrackKindGeneral, "expand", -1, steps...); err != nil {
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
