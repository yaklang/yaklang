package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleFindings() []*Finding {
	return []*Finding{
		{ID: "VULN-001", Title: "SQL A", File: "a.php", Line: 1, Severity: "HIGH"},
		{ID: "VULN-002", Title: "SQL B", File: "b.php", Line: 2, Severity: "MEDIUM"},
		{ID: "VULN-003", Title: "XSS C", File: "c.php", Line: 3, Severity: "LOW"},
	}
}

func TestAuditState_UpsertAndEnsureCoverage(t *testing.T) {
	state := NewAuditState()
	for _, f := range sampleFindings() {
		state.AddFinding(f)
	}
	state.UpsertVerifiedFinding(&VerifiedFinding{
		Finding: sampleFindings()[0],
		Status:  VerifyConfirmed,
		Reason:  "ok",
	})
	state.UpsertVerifiedFinding(&VerifiedFinding{
		Finding: sampleFindings()[0],
		Status:  VerifySafe,
		Reason:  "replaced",
	})
	require.Equal(t, 1, len(state.GetVerifiedVulns()))
	require.Equal(t, VerifySafe, state.GetVerifiedFindingByID("VULN-001").Status)

	state.DedupeVerifiedVulns()
	state.AddVerifiedFinding(&VerifiedFinding{Finding: sampleFindings()[1], Status: VerifyConfirmed})
	state.AddVerifiedFinding(&VerifiedFinding{Finding: sampleFindings()[0], Status: VerifyConfirmed})
	require.Equal(t, 1, state.DedupeVerifiedVulns())
	require.Equal(t, 2, len(state.GetVerifiedVulns()))

	filled := state.EnsureVerifyCoverage()
	require.Equal(t, []string{"VULN-003"}, filled)
	require.Nil(t, state.MissingVerifiedFindingIDs())
	require.Equal(t, VerifyUncertain, state.GetVerifiedFindingByID("VULN-003").Status)
}

func TestRepairAuditReportCoverage_AppendsMissing(t *testing.T) {
	vulns := []*VerifiedFinding{
		{
			Finding:  &Finding{ID: "VULN-001", Title: "SQL A", File: "a.php", Line: 10, Severity: "HIGH"},
			Status:   VerifyConfirmed,
			Reason:   "confirmed",
			DataFlow: "flow",
		},
		{
			Finding: &Finding{ID: "VULN-002", Title: "XSS B", File: "b.php", Line: 20, Severity: "MEDIUM"},
			Status:  VerifyUncertain,
			Reason:  "needs review",
		},
	}
	report := "# Audit Report\n\n## 漏洞详情\n\n### VULN-001 SQL A\n\n已覆盖。\n"
	repaired, missing := RepairAuditReportCoverage(report, vulns)
	require.Equal(t, []string{"VULN-002"}, missing)
	require.True(t, strings.Contains(repaired, "VULN-002"))
	require.True(t, strings.Contains(repaired, "报告补录"))
	require.True(t, strings.Contains(repaired, "XSS B"))
}

func TestFindingMentionedInReport_ByIDOrTitle(t *testing.T) {
	f := &Finding{ID: "VULN-015", Title: "Path Traversal", File: "view.php", Line: 60}
	require.True(t, findingMentionedInReport("see vuln-015 here", f))
	require.True(t, findingMentionedInReport("path traversal issue", f))
	require.False(t, findingMentionedInReport("unrelated content", f))
}

func TestReportableVerifiedVulns_ExcludesSafe(t *testing.T) {
	state := NewAuditState()
	state.UpsertVerifiedFinding(&VerifiedFinding{
		Finding: &Finding{ID: "VULN-001"},
		Status:  VerifySafe,
	})
	state.UpsertVerifiedFinding(&VerifiedFinding{
		Finding: &Finding{ID: "VULN-002"},
		Status:  VerifyConfirmed,
	})
	require.Len(t, state.ReportableVerifiedVulns(), 1)
	require.Equal(t, "VULN-002", state.ReportableVerifiedVulns()[0].Finding.ID)
}
