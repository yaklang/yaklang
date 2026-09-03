package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScanState_ProgressSequenceEquivalence marks 100 files in the same
// interleavings the production loop produces and asserts the Progress
// sequence is exactly 1..100 (audit equivalence for auditedCount).
func TestScanState_ProgressSequenceEquivalence(t *testing.T) {
	scan := newScanState()
	files := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		files = append(files, "/tmp/f"+string(rune('0'+i%10))+string(rune('0'+i/10))+".go")
	}
	scan.AddTargetFiles(files)
	scan.CommitToAudit()

	for i, f := range files {
		// repeated mark (idempotence) mid-sequence
		if i%10 == 5 {
			markFileDoneForTest(scan, f)
		}
		markFileDoneForTest(scan, f)
		done, total := scan.Progress()
		require.Equal(t, i+1, done)
		require.Equal(t, 100, total)
		if i < 99 {
			require.False(t, scan.AllDone())
		}
	}
	require.True(t, scan.AllDone())
	done, _ := scan.Progress()
	require.Equal(t, 100, done)
}

// TestScanState_DiscoveryGateCountEquivalence: auto-locked candidates after
// marks keep the count consistent.
func TestScanState_DiscoveryGateCountEquivalence(t *testing.T) {
	scan := newScanState()
	scan.AddDiscoveryCandidates([]string{"/tmp/d1.go", "/tmp/d2.go", "/tmp/d3.go"})
	// mark one candidate before it becomes a target (out-of-order path)
	markFileDoneForTest(scan, "/tmp/d1.go")
	scan.AddTargetFiles([]string{"/tmp/d1.go"})
	done, total := scan.Progress()
	require.Equal(t, 1, done)
	require.Equal(t, 1, total)

	// auto-lock remaining discovery candidates
	auto, _ := scan.PrepareDiscoveryGateForPhaseB()
	require.Equal(t, 2, auto)
	done, total = scan.Progress()
	require.Equal(t, 1, done)
	require.Equal(t, 3, total)

	markFileDoneForTest(scan, "/tmp/d2.go")
	markFileDoneForTest(scan, "/tmp/d3.go")
	done, total = scan.Progress()
	require.Equal(t, 3, done)
	require.Equal(t, 3, total)
	require.True(t, scan.AllDone())
}
