package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareDiscoveryGateForPhaseB(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/tmp/a.php", "/tmp/b.php", "/tmp/c.php"})
	scan.MarkSpotChecked("/tmp/a.php")
	scan.AddTargetFiles([]string{"/tmp/a.php"})

	autoLocked, skipped := scan.PrepareDiscoveryGateForPhaseB()
	require.Equal(t, 2, autoLocked)
	require.Equal(t, 0, skipped)
	require.Len(t, scan.UnresolvedDiscovery(), 0)
	require.Equal(t, 3, scan.TargetFileCount())
}

func TestScanState_DiscoveryCandidatesUnresolved(t *testing.T) {
	scan := newScanState()
	paths := []string{"/tmp/a.php", "/tmp/b.php", "/tmp/c.php"}
	scan.AddDiscoveryCandidates(paths)

	require.Equal(t, 3, scan.DiscoveryCandidateCount())
	require.Len(t, scan.UnresolvedDiscovery(), 3)

	scan.MarkSpotChecked("/tmp/a.php")
	scan.AddTargetFiles([]string{"/tmp/a.php"})

	unresolved := scan.UnresolvedDiscovery()
	require.Len(t, unresolved, 2)
	require.Contains(t, unresolved, "/tmp/b.php")
	require.Contains(t, unresolved, "/tmp/c.php")
}

func TestValidatePhaseALockTargetFiles_AutoRelaxesWhenTargetsLocked(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/tmp/low.php", "/tmp/medium.php"})
	scan.MarkSpotChecked("/tmp/low.php")
	scan.AddTargetFiles([]string{"/tmp/low.php"})

	ok, msg := validatePhaseALockTargetFiles(scan, nil, true)
	require.True(t, ok)
	require.Contains(t, msg, "广度优先")
	require.Equal(t, 2, scan.TargetFileCount())
}

func TestValidatePhaseALockTargetFiles_BlocksDoneTrueWithNoTargets(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/tmp/low.php", "/tmp/medium.php"})

	ok, msg := validatePhaseALockTargetFiles(scan, nil, true)
	require.False(t, ok)
	require.Contains(t, msg, "禁止 lock_target_files(done=true)")
}

func TestValidatePhaseALockTargetFiles_AllowsLockWithoutSpotRead(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/tmp/low.php"})

	ok, msg := validatePhaseALockTargetFiles(scan, []string{"/tmp/low.php"}, false)
	require.True(t, ok)
	require.Empty(t, msg)
}

func TestValidatePhaseALockTargetFiles_AllowsDoneTrueWhenAllLocked(t *testing.T) {
	scan := newScanState()
	paths := []string{"/tmp/low.php", "/tmp/medium.php"}
	scan.AddDiscoveryCandidates(paths)
	for _, p := range paths {
		scan.MarkSpotChecked(p)
		scan.AddTargetFiles([]string{p})
	}

	ok, msg := validatePhaseALockTargetFiles(scan, nil, true)
	require.True(t, ok)
	require.Empty(t, msg)
}

func TestValidatePhaseALockTargetFiles_NonDiscoveryPathWithoutRead(t *testing.T) {
	scan := newScanState()
	ok, msg := validatePhaseALockTargetFiles(scan, []string{"/tmp/manual_grep_hit.php"}, false)
	require.True(t, ok)
	require.Empty(t, msg)
}

func TestValidatePhaseALockTargetFiles_AllowsLockAfterSpotRead(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/tmp/low.php"})
	scan.MarkSpotChecked("/tmp/low.php")

	ok, msg := validatePhaseALockTargetFiles(scan, []string{"/tmp/low.php"}, false)
	require.True(t, ok)
	require.Empty(t, msg)
}
