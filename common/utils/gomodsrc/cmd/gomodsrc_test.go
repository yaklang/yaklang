package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCollectPackageTreeSourcesIncludesCgoSubdirHeaders guards the CI failure
// where a pruned runtime tree could not compile: `go list` reports only the
// files inside a cgo package's own directory, but common/minirehs includes
// native/mvscan/mvscan.h, so the header was never copied.
func TestCollectPackageTreeSourcesIncludesCgoSubdirHeaders(t *testing.T) {
	pkgDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "native", "mvscan"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "mvs_cgo.go"), []byte("package x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "native", "mvscan", "mvscan.h"), []byte("/* h */\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "native", "mvscan", "mvscan.c"), []byte("/* c */\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "native", "mvscan", "notes.txt"), []byte("skip\n"), 0o644))

	files, err := collectPackageTreeSources(pkgDir, []string{"mvs_cgo.go"}, true)
	require.NoError(t, err)

	joined := make(map[string]struct{}, len(files))
	for _, f := range files {
		joined[filepath.Clean(f)] = struct{}{}
	}
	require.Contains(t, joined, filepath.Join("native", "mvscan", "mvscan.h"))
	require.Contains(t, joined, filepath.Join("native", "mvscan", "mvscan.c"))
	require.NotContains(t, joined, filepath.Join("native", "mvscan", "notes.txt"),
		"only C sources and headers are needed from subdirectories")
}

// TestCollectPackageTreeSourcesLeavesPureGoPackagesAlone keeps the extra walk
// from touching ordinary Go packages.
func TestCollectPackageTreeSourcesLeavesPureGoPackagesAlone(t *testing.T) {
	pkgDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(pkgDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "a.go"), []byte("package x\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "nested", "b.h"), []byte("/* h */\n"), 0o644))

	files, err := collectPackageTreeSources(pkgDir, []string{"a.go"}, false)
	require.NoError(t, err)
	require.Equal(t, []string{"a.go"}, files)
}
