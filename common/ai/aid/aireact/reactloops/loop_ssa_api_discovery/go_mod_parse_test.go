package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGoModSummary(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "go.mod")
	require.NoError(t, os.WriteFile(modPath, []byte("module example.com/foo\n\ngo 1.22\n\nrequire github.com/a/b v1.0.0\n"), 0o644))

	summary, err := parseGoModSummary(modPath)
	require.NoError(t, err)
	require.Equal(t, "example.com/foo", summary.ModulePath)
	require.Equal(t, "1.22", summary.GoVersion)
	require.Contains(t, summary.Requires, "github.com/a/b")
}

func TestBuildDependenciesPayloadFromRoot_GoMod(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module vuln-detect-benchmark\n\ngo 1.23\n"), 0o644))

	payload := buildDependenciesPayloadFromRoot(dir)
	manifests, ok := payload["manifests"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, manifests, 1)
	require.Equal(t, "vuln-detect-benchmark", manifests[0]["module"])
}
