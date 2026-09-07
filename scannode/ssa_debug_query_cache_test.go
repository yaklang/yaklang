package scannode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadCachedDebugAnalysisUsesFreshCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte("line\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	want := []byte(`{"status":"running","samples":[]}`)
	if err := writeCachedDebugAnalysis(dir, want); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	got, ok := readCachedDebugAnalysis(dir)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(got) != string(want) {
		t.Fatalf("cached analysis = %s, want %s", got, want)
	}
}

func TestReadCachedDebugAnalysisRejectsStaleCacheOnNewProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	profiles := filepath.Join(dir, "cpu-pprof")
	if err := os.MkdirAll(profiles, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cachePath := filepath.Join(dir, debugAnalysisCacheName)
	if err := os.WriteFile(cachePath, []byte(`{"status":"old"}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profiles, "20260101-120000-x.cpu.prof"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	if _, ok := readCachedDebugAnalysis(dir); ok {
		t.Fatal("expected stale cache miss when a newer profile exists")
	}
}

// The engine keeps appending to debug/log for the whole run; the log must
// NOT invalidate the analysis cache, otherwise every console poll forces a
// full re-parse of every profile (which OOM'd nodes on long debug runs).
func TestReadCachedDebugAnalysisIgnoresLogGrowth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	profiles := filepath.Join(dir, "cpu-pprof")
	if err := os.MkdirAll(profiles, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	profile := filepath.Join(profiles, "20260101-120000-x.cpu.prof")
	if err := os.WriteFile(profile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(profile, old, old); err != nil {
		t.Fatalf("chtimes profile: %v", err)
	}
	want := []byte(`{"status":"running","samples":[]}`)
	if err := writeCachedDebugAnalysis(dir, want); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte("newer log line\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	got, ok := readCachedDebugAnalysis(dir)
	if !ok {
		t.Fatal("expected cache hit even though the log is newer")
	}
	if string(got) != string(want) {
		t.Fatalf("cached analysis = %s, want %s", got, want)
	}
}

// capTestSample builds one analysis sample whose inline stack payload weighs
// about stackKB kilobytes.
func capTestSample(sequence int, stackKB int) map[string]any {
	frame := strings.Repeat("F", stackKB*1024)
	return map[string]any{
		"sequence":   sequence,
		"label":      fmt.Sprintf("20260101-12%02d00-x", sequence%60),
		"phase":      "scan",
		"runtime":    map[string]any{"goroutines": 42},
		"cpu_stacks": []map[string]any{{"frames": []string{frame}, "value": 1}},
		"cpu_top":    []map[string]any{{"name": "pkg.fn", "cum_value": 1}},
	}
}

func TestCapLiveDebugAnalysisStripsOldSamples(t *testing.T) {
	samples := make([]map[string]any, 0, 40)
	for i := 0; i < 40; i++ {
		samples = append(samples, capTestSample(i, 25)) // 40 × ~25KB > 900KB
	}
	raw, err := json.Marshal(map[string]any{
		"status":  "running",
		"phases":  []map[string]any{{"phase": "scan", "status": "running"}},
		"samples": samples,
	})
	require.NoError(t, err)
	require.Greater(t, len(raw), 900*1024)

	capped := capLiveDebugAnalysis(raw, 900*1024)
	assert.LessOrEqual(t, len(capped), 900*1024)

	var payload struct {
		Status  string           `json:"status"`
		Samples []map[string]any `json:"samples"`
	}
	require.NoError(t, json.Unmarshal(capped, &payload))
	assert.Equal(t, "running", payload.Status)
	// The sample LIST survives; only the heavy inline details are trimmed.
	assert.Len(t, payload.Samples, 40)
	// Newest samples keep their stacks, the oldest lose them.
	assert.NotEmpty(t, payload.Samples[len(payload.Samples)-1]["cpu_stacks"])
	assert.Empty(t, payload.Samples[0]["cpu_stacks"])
	assert.NotEmpty(t, payload.Samples[0]["label"])
}

func TestCapLiveDebugAnalysisDropsRowsWhenStillTooBig(t *testing.T) {
	// One sample with a giant non-strippable field: the only way under the
	// budget is dropping rows down to the last one (best effort).
	samples := make([]map[string]any, 0, 30)
	for i := 0; i < 30; i++ {
		samples = append(samples, map[string]any{
			"sequence": i,
			"label":    fmt.Sprintf("20260101-1200%02d-x", i),
		})
	}
	raw, err := json.Marshal(map[string]any{
		"status": "running",
		"phases": []map[string]any{{"phase": strings.Repeat("P", 2*1024*1024)}},
		"samples": samples,
	})
	require.NoError(t, err)

	capped := capLiveDebugAnalysis(raw, 900*1024)
	var payload struct {
		Samples []map[string]any `json:"samples"`
	}
	require.NoError(t, json.Unmarshal(capped, &payload))
	assert.Less(t, len(payload.Samples), 30, "oldest sample rows must be dropped")
	assert.NotEmpty(t, payload.Samples, "at least one sample row must survive")
}

func TestCapLiveDebugAnalysisKeepsSmallPayloadUntouched(t *testing.T) {
	raw := json.RawMessage(`{"status":"running","samples":[{"sequence":1,"label":"a"}]}`)
	capped := capLiveDebugAnalysis(raw, 900*1024)
	assert.Equal(t, string(raw), string(capped))
}
