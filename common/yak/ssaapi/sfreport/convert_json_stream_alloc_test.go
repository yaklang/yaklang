//go:build !race

package sfreport

import (
	"bytes"
	"encoding/json"
	"io"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// A report is written at every stage boundary and at the end of the scan. Its
// size must not decide how much memory a snapshot needs: the writer streams,
// so peak allocation stays near one element instead of one document.
func TestReport_SnapshotDoesNotAllocateTheDocument(t *testing.T) {
	report := populatedReport(t, 400)

	var buf bytes.Buffer
	require.NoError(t, report.writeJSONSnapshot(&buf))
	documentSize := buf.Len()
	require.Greater(t, documentSize, 256<<10, "the fixture must be large enough to matter")

	// The largest single element bounds a snapshot's working set; a whole
	// document would be several times that.
	largestElement := 0
	for _, risk := range report.Risks {
		encoded, err := json.Marshal(risk)
		require.NoError(t, err)
		if len(encoded) > largestElement {
			largestElement = len(encoded)
		}
	}
	for _, file := range report.File {
		encoded, err := json.Marshal(file)
		require.NoError(t, err)
		if len(encoded) > largestElement {
			largestElement = len(encoded)
		}
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	require.NoError(t, report.writeJSONSnapshot(io.Discard))
	runtime.ReadMemStats(&after)

	allocated := after.TotalAlloc - before.TotalAlloc
	require.Less(t, allocated, uint64(documentSize),
		"one snapshot must not allocate a whole document (doc=%d, alloc=%d)", documentSize, allocated)
	t.Logf("document=%dB largest element=%dB allocated by one snapshot=%dB",
		documentSize, largestElement, allocated)
}
