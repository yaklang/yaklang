package util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListScanObservationFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scan_obs_sql_injection.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scan_obs_cmd_injection.json"), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scan_findings.json"), []byte("[]"), 0o644))

	paths := ListScanObservationFiles(dir)
	require.Len(t, paths, 2)
	require.Contains(t, paths[0], "scan_obs_cmd_injection.json")
	require.Contains(t, paths[1], "scan_obs_sql_injection.json")
}
