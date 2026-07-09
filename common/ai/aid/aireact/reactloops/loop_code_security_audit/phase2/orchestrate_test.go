package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldResumeCategoryScanFromPhaseA_WithDiscoveryOnly(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/abs/app/sink.php"})
	require.True(t, shouldResumeCategoryScanFromPhaseA(scan))
}

func TestShouldResumeCategoryScanFromPhaseA_WithLockedTargets(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/abs/app/login.php"})
	require.True(t, shouldResumeCategoryScanFromPhaseA(scan))
}

func TestShouldResumeCategoryScanFromPhaseA_EmptyPhaseA(t *testing.T) {
	scan := newScanState()
	require.False(t, shouldResumeCategoryScanFromPhaseA(scan))
}

func TestShouldResumeCategoryScanFromPhaseA_AlreadyInPhaseB(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/abs/app/sink.php"})
	scan.CommitToAudit()
	require.False(t, shouldResumeCategoryScanFromPhaseA(scan))
}
