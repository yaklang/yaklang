package ssaapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupDebugDirEmptyIsNoop(t *testing.T) {
	cleanup := SetupDebugDir("", false)
	require.NotNil(t, cleanup)
	cleanup() // must not panic
}

func TestSetupDebugDirCreatesLogAndDirs(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "run-1")

	cleanup := SetupDebugDir(target, false)
	require.NotNil(t, cleanup)
	defer cleanup()

	info, err := os.Stat(target)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	_, err = os.Stat(filepath.Join(target, "log"))
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(target, "cpu-pprof"))
	require.NoError(t, err)
}

func TestSetupDebugDirPanicDoesNotEscape(t *testing.T) {
	orig := startDebugOutputImpl
	startDebugOutputImpl = func(string, bool) (DebugOutputCleanup, error) {
		panic("boom: debug setup")
	}
	t.Cleanup(func() { startDebugOutputImpl = orig })

	cleanup := SetupDebugDir(t.TempDir(), false)
	require.NotNil(t, cleanup)
	require.NotPanics(t, func() { cleanup() })
}
