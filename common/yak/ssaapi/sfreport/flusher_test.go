package sfreport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReportFlusher_LatestSnapshotWins proves intermediates are written in the
// background and that the document on disk is the newest one after the final
// save drained the worker.
func TestReportFlusher_LatestSnapshotWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	defer file.Close()

	flusher := newReportFlusher(file)
	flusher.submit([]byte(`{"snapshot":1}`))
	flusher.submit([]byte(`{"snapshot":2}`))
	flusher.stopAndWait()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, `{"snapshot":2}`, string(data))
}
