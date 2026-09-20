package sca

import (
	"testing"
	"testing/fstest"
)

// The old Docker/Git fixtures and original tests are preserved in the baseline
// archive. These entry points have no implementation or runtime fallback.
func TestRemovedAcquisitionExports(t *testing.T) {
	for _, key := range []string{"ScanImageFromContext", "ScanContainerFromContext", "ScanImageFromFile", "ScanGitRepo", "endpoint"} {
		if _, ok := Exports[key]; ok {
			t.Errorf("removed export remains: %s", key)
		}
	}
	for _, key := range []string{"ScanLocalFilesystem", "ScanFilesystem", "customAnalyzer"} {
		if _, ok := Exports[key]; !ok {
			t.Errorf("retained export missing: %s", key)
		}
	}
}
func TestInvalidWorkerCount(t *testing.T) {
	for _, n := range []int{-1, 0, 65} {
		if _, err := ScanFilesystem(fstest.MapFS{}, _withConcurrent(n)); err == nil {
			t.Errorf("accepted workers=%d", n)
		}
	}
}
