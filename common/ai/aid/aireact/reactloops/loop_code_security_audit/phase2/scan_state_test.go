package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// markFileDoneForTest marks a file audited without recording disposition.
// Test-only helper（生产路径走 MarkFileDoneWithDisposition）。
func markFileDoneForTest(scan *ScanState, filePath string) {
	scan.MarkFileDoneWithDisposition(filePath, "")
}

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

	markFileDoneForTest(scan, "/tmp/a.go")
	markFileDoneForTest(scan, "/tmp/not-in-list.go")

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
	markFileDoneForTest(scan, "/tmp/a.go")
	require.False(t, scan.AllDone())
	markFileDoneForTest(scan, "/tmp/b.go")
	require.True(t, scan.AllDone())
}

// auditedCount 必须在重复 mark（幂等）时不多计。
func TestScanState_MarkFileDoneIdempotent(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})

	markFileDoneForTest(scan, "/tmp/a.go")
	markFileDoneForTest(scan, "/tmp/a.go")

	done, total := scan.Progress()
	require.Equal(t, 1, done)
	require.Equal(t, 1, total)
	require.True(t, scan.AllDone())
}

// 先 mark 后纳入目标列表的顺序同样要计入 auditedCount（AddTargetFiles 补计）。
func TestScanState_MarkBeforeAddTarget(t *testing.T) {
	scan := newScanState()
	markFileDoneForTest(scan, "/tmp/late.go")
	done, total := scan.Progress()
	require.Equal(t, 0, done)
	require.Equal(t, 0, total)

	scan.AddTargetFiles([]string{"/tmp/late.go"})
	done, total = scan.Progress()
	require.Equal(t, 1, done)
	require.Equal(t, 1, total)
	require.True(t, scan.AllDone())
}

func TestScanState_PhaseBReadAndGrepCount(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.CommitToAudit()

	read, grep := scan.PhaseBReadAndGrepCount("/tmp/a.go")
	require.Equal(t, 0, read)
	require.Equal(t, 0, grep)

	scan.BumpPhaseBRead("/tmp/a.go")
	scan.BumpPhaseBRead("/tmp/a.go")
	scan.BumpPhaseBGrep("/tmp/a.go")

	read, grep = scan.PhaseBReadAndGrepCount("/tmp/a.go")
	require.Equal(t, 2, read)
	require.Equal(t, 1, grep)
}

func TestScanState_PhaseBReadGuardSnapshot(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})

	phase, isTarget, audited, count := scan.PhaseBReadGuardSnapshot("/tmp/a.go")
	require.Equal(t, ScanPhaseSearch, phase)
	require.True(t, isTarget)
	require.False(t, audited)
	require.Equal(t, 0, count)

	scan.CommitToAudit()
	scan.BumpPhaseBRead("/tmp/a.go")
	phase, isTarget, audited, count = scan.PhaseBReadGuardSnapshot("/tmp/a.go")
	require.Equal(t, ScanPhaseAudit, phase)
	require.Equal(t, 1, count)

	markFileDoneForTest(scan, "/tmp/a.go")
	_, _, audited, _ = scan.PhaseBReadGuardSnapshot("/tmp/a.go")
	require.True(t, audited)
}
