package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPhase2PhaseBGrepGuard_AllowsScopedContentGrep(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/src/handler/user.go"})
	scan.CommitToAudit()

	guard := buildPhase2PhaseBGrepGuard(scan, "/proj")
	params := map[string]any{
		"path":        "/proj/src/handler",
		"pattern":     "ProcessUser",
		"output-mode": "content",
	}
	allow, msg := guard("grep", params)
	require.True(t, allow, msg)
}

func TestBuildPhase2PhaseBGrepGuard_BlocksFilesWithMatches(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/src/handler/user.go"})
	scan.CommitToAudit()

	guard := buildPhase2PhaseBGrepGuard(scan, "/proj")
	params := map[string]any{
		"path":        "/proj",
		"pattern":     "mysqli_query",
		"output-mode": "files_with_matches",
	}
	allow, msg := guard("grep", params)
	require.False(t, allow)
	require.Contains(t, msg, "discovery")
}

func TestBuildPhase2PhaseBGrepGuard_BlocksOutOfScopePath(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/src/handler/user.go"})
	scan.CommitToAudit()

	guard := buildPhase2PhaseBGrepGuard(scan, "/proj")
	params := map[string]any{
		"path":    "/other/vendor",
		"pattern": "exec(",
	}
	allow, msg := guard("grep", params)
	require.False(t, allow)
	require.Contains(t, msg, "不在允许范围")
}

func TestBuildPhase2PhaseBGrepGuard_BlocksWhenBudgetExceeded(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/a.go"})
	scan.CommitToAudit()
	for i := 0; i < phase2MaxPhaseBTraceGrepsPerFile; i++ {
		scan.BumpPhaseBGrep("/proj/a.go")
	}

	guard := buildPhase2PhaseBGrepGuard(scan, "/proj")
	params := map[string]any{
		"path":    "/proj/a.go",
		"pattern": "Sink",
	}
	allow, msg := guard("grep", params)
	require.False(t, allow)
	require.Contains(t, msg, "trace grep")
}

func TestBuildPhase2PhaseBGrepParamsMutator_ForcesContentModeAndLimit(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/a.go"})
	scan.CommitToAudit()

	mutator := buildPhase2PhaseBGrepParamsMutator(scan)
	out := mutator("grep", map[string]any{
		"path":        "/proj/a.go",
		"pattern":     "main",
		"output-mode": "files_with_matches",
		"limit":       500,
	})
	require.Equal(t, "content", out["output-mode"])
	require.Equal(t, phase2MaxPhaseBTraceGrepLimit, out["limit"])
	require.Equal(t, 1, scan.PhaseBGrepCount("/proj/a.go"))
}

func TestIsPhaseBGrepPathAllowed_ProjectRoot(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/internal/dao/user.go"})
	require.True(t, isPhaseBGrepPathAllowed("/proj", scan, "/proj"))
	require.True(t, isPhaseBGrepPathAllowed("/proj/internal", scan, "/proj"))
	require.True(t, isPhaseBGrepPathAllowed("/proj/internal/dao/user.go", scan, "/proj"))
	require.False(t, isPhaseBGrepPathAllowed("/other", scan, "/proj"))
}

func TestScanState_ClearPhaseBGrepsOnMark(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/tmp/a.go"})
	scan.CommitToAudit()
	scan.BumpPhaseBGrep("/tmp/a.go")
	scan.BumpPhaseBGrep("/tmp/a.go")
	require.Equal(t, 2, scan.PhaseBGrepCount("/tmp/a.go"))

	scan.MarkFileDone("/tmp/a.go")
	scan.ClearPhaseBGreps("/tmp/a.go")
	require.Equal(t, 0, scan.PhaseBGrepCount("/tmp/a.go"))
}
