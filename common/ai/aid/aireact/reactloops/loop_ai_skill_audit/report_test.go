package loop_ai_skill_audit

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreferRicherFindingsText(t *testing.T) {
	short := "brief summary"
	long := "# Findings\n\n### Reverse Shell\n\ndetails..."
	require.Equal(t, long, preferRicherFindingsText(short, long, ""))
	require.Equal(t, long, preferRicherFindingsText("", long))
}

func TestComposeSkillSecurityReport_IncludesFindings(t *testing.T) {
	state := NewSkillAuditState()
	state.SkillName = "demo-skill"
	state.SkillPath = "/tmp/demo-skill"
	state.RiskLevel = "High"
	state.SetAuditResult("High", "| check | Pass |", "### Malware\n\ndetails here")
	report := composeSkillSecurityReport(state)
	require.Contains(t, report, "AI Skill 安全审计报告")
	require.Contains(t, report, "### Malware")
	require.Contains(t, report, "| check | Pass |")
	require.Contains(t, report, "**整体风险等级**: **High**")
}

func TestFinalizeSkillAuditReport_ReplacesTruncatedPhase3(t *testing.T) {
	dir := t.TempDir()
	reportPath := dir + "/skill_security_report.md"

	state := NewSkillAuditState()
	state.SkillName = "demo"
	state.SkillPath = "/tmp/demo"
	state.SetAuditResult("Medium", "", strings.Repeat("# Finding\n\n", 200))

	require.NoError(t, os.WriteFile(reportPath, []byte("# Short\n"), 0o644))

	final := finalizeSkillAuditReport(state, reportPath)
	require.Greater(t, len(final), 1000)
	require.Contains(t, final, "Finding")
	require.Equal(t, final, state.GetFinalReport())
}
