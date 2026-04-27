package loop_syntaxflow_scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildCodeScanJSONForLocalPath(t *testing.T) {
	d := t.TempDir()
	j, err := BuildCodeScanJSONForLocalPath(d)
	require.NoError(t, err)
	require.Contains(t, j, d)
	_, err = BuildCodeScanJSONForLocalPath("/nonexistent/path/that/should/not/exist/12345")
	require.Error(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(d, "x.txt"), []byte("x"), 0o644))
	f := filepath.Join(d, "x.txt")
	j2, err := BuildCodeScanJSONForLocalPath(f)
	require.NoError(t, err)
	require.Contains(t, j2, d)
}
