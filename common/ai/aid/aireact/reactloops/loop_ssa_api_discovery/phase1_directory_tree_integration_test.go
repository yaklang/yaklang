package loop_ssa_api_discovery

import (
	"os"
	"testing"
	"time"
)

// Optional integration test against a real repo on disk.
// Run with: go test -run TestBuildDirectoryTree_realPublicCMS -timeout 30s
func TestBuildDirectoryTree_realPublicCMS(t *testing.T) {
	root := "/home/murkfox/yak-ssa-api-discovery/benchmark-repos/real-cms/PublicCMS/publiccms-parent"
	if _, err := os.Stat(root); err != nil {
		t.Skip("PublicCMS benchmark repo not present:", root)
	}

	start := time.Now()
	javaRoot := FindJavaRoot(root)
	findDur := time.Since(start)
	if javaRoot == "" {
		t.Fatal("FindJavaRoot returned empty")
	}
	if !isDirAncestorOrEqual(root, javaRoot) {
		t.Fatalf("java root escaped: root=%q javaRoot=%q", root, javaRoot)
	}
	t.Logf("FindJavaRoot: %v -> %s", findDur.Round(time.Millisecond), javaRoot)

	treeStart := time.Now()
	tree := BuildDirectoryTree(javaRoot)
	treeDur := time.Since(treeStart)
	if tree == nil {
		t.Fatal("nil tree")
	}
	t.Logf("BuildDirectoryTree: %v dirs=%d files=%d kb=%d", treeDur.Round(time.Millisecond), tree.TotalDirs, tree.TotalFiles, tree.TotalSizeKB)

	if treeDur > 10*time.Second {
		t.Fatalf("BuildDirectoryTree too slow on PublicCMS: %v", treeDur)
	}
	if tree.TotalFiles == 0 {
		t.Fatal("expected java files in PublicCMS tree")
	}
}
