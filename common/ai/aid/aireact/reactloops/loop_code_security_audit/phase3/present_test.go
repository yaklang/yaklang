package phase3

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestFindingsByCategoryMap(t *testing.T) {
	findings := []*model.Finding{
		{ID: "VULN-001", Category: "sql_injection"},
		{ID: "VULN-002", Category: "xss_injection"},
		{ID: "VULN-003", Category: "sql_injection"},
	}
	byCat := findingsByCategoryMap(findings)
	require.Len(t, byCat, 2)
	require.Equal(t, []string{"VULN-001", "VULN-003"}, byCat["sql_injection"])
	require.Equal(t, []string{"VULN-002"}, byCat["xss_injection"])

	summary := formatVerifyScopeSummary(findings)
	require.Contains(t, summary, "3 个 finding")
	require.Contains(t, summary, "sql_injection")
}
