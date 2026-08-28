package scannode

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestReadCachedDebugAnalysisRejectsStaleCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, debugAnalysisCacheName)
	if err := os.WriteFile(cachePath, []byte(`{"status":"old"}`), 0o644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatalf("chtimes cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte("newer\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	if _, ok := readCachedDebugAnalysis(dir); ok {
		t.Fatal("expected stale cache miss")
	}
}
