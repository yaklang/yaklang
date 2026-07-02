package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestValidateMarkFileDoneDisposition_FindingRequiresAddFinding(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/vuln/low.php"})
	scan.CommitToAudit()
	state := &model.AuditState{ProjectPath: "/proj"}

	ok, msg := validateMarkFileDoneDisposition(scan, state, "sqli", "/proj/vuln/low.php", "/proj", "finding")
	require.False(t, ok)
	require.Contains(t, msg, "尚无 add_finding")
}

func TestValidateMarkFileDoneDisposition_NotVulWithFindingConflict(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/vuln/low.php"})
	scan.CommitToAudit()
	scan.NoteFinding("/proj/vuln/low.php")
	state := &model.AuditState{ProjectPath: "/proj"}

	ok, msg := validateMarkFileDoneDisposition(scan, state, "sqli", "/proj/vuln/low.php", "/proj", "not_vul")
	require.False(t, ok)
	require.Contains(t, msg, "冲突")
}

func TestValidateMarkFileDoneDisposition_FindingOK(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/vuln/low.php"})
	scan.CommitToAudit()
	scan.NoteFinding("/proj/vuln/low.php")

	ok, msg := validateMarkFileDoneDisposition(scan, nil, "sqli", "/proj/vuln/low.php", "/proj", "finding")
	require.True(t, ok)
	require.Empty(t, msg)
}

func TestValidateAllTargetsAttributed_BlocksIncomplete(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/a.php", "/proj/b.php"})
	scan.CommitToAudit()
	scan.MarkFileDoneWithDisposition("/proj/a.php", FileDispositionNotVul)

	ok, msg := validateAllTargetsAttributed(scan, nil, "sqli", "/proj")
	require.False(t, ok)
	require.Contains(t, msg, "/proj/b.php")
	require.Contains(t, msg, "not_vul")
}

func TestValidateAllTargetsAttributed_AllAttributed(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/a.php", "/proj/b.php"})
	scan.CommitToAudit()
	scan.NoteFinding("/proj/a.php")
	scan.MarkFileDoneWithDisposition("/proj/a.php", FileDispositionFinding)
	scan.MarkFileDoneWithDisposition("/proj/b.php", FileDispositionNotVul)

	ok, msg := validateAllTargetsAttributed(scan, nil, "sqli", "/proj")
	require.True(t, ok)
	require.Empty(t, msg)
}

func TestResolveTargetAbsPath(t *testing.T) {
	scan := newScanState()
	scan.AddTargetFiles([]string{"/proj/vulnerabilities/sqli/source/low.php"})

	abs := resolveTargetAbsPath("/proj", scan, "vulnerabilities/sqli/source/low.php")
	require.Equal(t, "/proj/vulnerabilities/sqli/source/low.php", abs)
}
