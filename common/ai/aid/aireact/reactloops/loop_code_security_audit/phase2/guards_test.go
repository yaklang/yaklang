package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPhase2PhaseASpotReadGuard_BlocksAfterMaxReads(t *testing.T) {
	scan := newScanState()
	for i := 0; i < phase2MaxSpotReadsBeforeLock; i++ {
		scan.BumpPhaseASpotReads()
	}

	guard := buildPhase2PhaseASpotReadGuard(scan)
	allow, msg := guard("read_file", nil)
	require.False(t, allow)
	require.Contains(t, msg, "lock_target_files")
}

func TestBuildPhase2PhaseASpotReadGuard_AllowsUnderLimit(t *testing.T) {
	scan := newScanState()
	scan.BumpPhaseASpotReads()

	guard := buildPhase2PhaseASpotReadGuard(scan)
	allow, msg := guard("read_file", nil)
	require.True(t, allow)
	require.Empty(t, msg)
}

func TestScanState_ResetPhaseASpotReadsOnLock(t *testing.T) {
	scan := newScanState()
	scan.BumpPhaseASpotReads()
	scan.BumpPhaseASpotReads()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.ResetPhaseASpotReads()
	require.Equal(t, 0, scan.PhaseASpotReadCount())
}

func TestBuildPhase2PhaseBDiscoveryToolGuard_BlocksFindFileInAudit(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.CommitToAudit()

	guard := buildPhase2PhaseBDiscoveryToolGuard(scan)
	allow, msg := guard("find_file", nil)
	require.False(t, allow)
	require.Contains(t, msg, "discovery")
}

func TestBuildPhase2PhaseBDiscoveryToolGuard_AllowsGrepInAudit(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.CommitToAudit()

	guard := buildPhase2PhaseBDiscoveryToolGuard(scan)
	allow, msg := guard("grep", nil)
	require.True(t, allow)
	require.Empty(t, msg)
}

func TestFormatCompleteScanBlockedFeedback(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go", "/tmp/b.go"})
	scan.CommitToAudit()
	scan.MarkFileDoneWithDisposition("/tmp/a.go", FileDispositionNotVul)

	msg := formatCompleteScanBlockedFeedback(scan, nil, "test", "")
	require.Contains(t, msg, "禁止调用 complete_scan")
	require.Contains(t, msg, "/tmp/b.go")
	require.Contains(t, msg, "not_vul")
}

func TestBuildPhase2PhaseBReadSpinGuard_BlocksThirdRead(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.CommitToAudit()
	scan.BumpPhaseBRead("/tmp/a.go")
	scan.BumpPhaseBRead("/tmp/a.go")

	guard := buildPhase2PhaseBReadSpinGuard(scan)
	params := map[string]any{"file": "/tmp/a.go"}
	allow, msg := guard("read_file", params)
	require.False(t, allow)
	require.Contains(t, msg, "mark_file_done")
	require.Contains(t, msg, "/tmp/a.go")
}

func TestBuildPhase2PhaseBReadSpinGuard_AllowsFirstRead(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.CommitToAudit()

	guard := buildPhase2PhaseBReadSpinGuard(scan)
	params := map[string]any{"file": "/tmp/a.go"}
	allow, msg := guard("read_file", params)
	require.True(t, allow)
	require.Empty(t, msg)
}
