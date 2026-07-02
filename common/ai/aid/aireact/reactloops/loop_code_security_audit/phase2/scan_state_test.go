package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScanState_AddTargetFilesAndCommit(t *testing.T) {
	scan := newScanState()
	require.Equal(t, ScanPhaseSearch, scan.Phase)

	added, total := scan.AddTargetFiles([]string{"/tmp/a.go", "/tmp/a.go", "/tmp/b.go"})
	require.Equal(t, 2, added)
	require.Equal(t, 2, total)
	require.Equal(t, 2, scan.TargetFileCount())

	collected := scan.CollectedTargetFiles()
	require.Len(t, collected, 2)
	require.Contains(t, collected, "/tmp/a.go")

	locked := scan.CommitToAudit()
	require.Len(t, locked, 2)

	scan.mu.Lock()
	require.Equal(t, ScanPhaseAudit, scan.Phase)
	scan.mu.Unlock()
}

func TestScanState_ProgressOnlyCountsTargetFiles(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go", "/tmp/b.go"})
	scan.CommitToAudit()

	scan.MarkFileDone("/tmp/a.go")
	scan.MarkFileDone("/tmp/not-in-list.go")

	done, total := scan.Progress()
	require.Equal(t, 1, done)
	require.Equal(t, 2, total)
	require.False(t, scan.AllDone())
}

func TestScanState_AllDone(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go", "/tmp/b.go"})
	scan.CommitToAudit()

	require.False(t, scan.AllDone())
	scan.MarkFileDone("/tmp/a.go")
	require.False(t, scan.AllDone())
	scan.MarkFileDone("/tmp/b.go")
	require.True(t, scan.AllDone())
}
