package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestAuditSummaryClaimsVulnerability(t *testing.T) {
	require.True(t, auditSummaryClaimsVulnerability("发现命令注入漏洞：str_replace 仅过滤 && 和 ;"))
	require.False(t, auditSummaryClaimsVulnerability("使用白名单机制，不存在命令注入漏洞"))
	require.False(t, auditSummaryClaimsVulnerability("未发现可利用漏洞"))
}

func TestHasFindingForAbsPath(t *testing.T) {
	state := &model.AuditState{ProjectPath: "/proj"}
	state.AddFinding(&model.Finding{
		Category: "cmd_injection",
		File:     "vulnerabilities/exec/source/medium.php",
	})

	abs := "/proj/vulnerabilities/exec/source/medium.php"
	require.True(t, hasFindingForAbsPath(state, "cmd_injection", abs, "/proj"))
	require.False(t, hasFindingForAbsPath(state, "cmd_injection", "/proj/vulnerabilities/exec/source/high.php", "/proj"))
}

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
