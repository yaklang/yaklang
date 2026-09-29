package sfreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteReportSnapshot_FileIsReplacedAtomically proves a file destination is
// replaced through a temporary file and a rename: the target always holds one
// complete document and no temporary file is left behind.
func TestWriteReportSnapshot_FileIsReplacedAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	require.NoError(t, os.WriteFile(path, []byte("previous"), 0o644))

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	defer file.Close()

	require.NoError(t, writeReportSnapshot(file, []byte(`{"risks":1}`)))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `{"risks":1}`, string(data))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	require.Equal(t, []string{"report.json"}, names, "no temporary snapshot file may remain")
}

// TestWriteReportSnapshot_BufferIsRewound proves a non-file destination keeps
// the replace-instead-of-append contract.
func TestWriteReportSnapshot_BufferIsRewound(t *testing.T) {
	buf := &SnapshotBuffer{}
	require.NoError(t, writeReportSnapshot(buf, []byte("first")))
	require.NoError(t, writeReportSnapshot(buf, []byte("second")))
	require.Equal(t, "second", buf.buf.String())
	require.True(t, strings.HasPrefix(buf.buf.String(), "second"))
}
