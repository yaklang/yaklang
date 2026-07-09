package phase2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
)

func TestFormatAuditVulnerabilityTypesSummary(t *testing.T) {
	categories := []model.VulnCategory{
		{ID: "sql_injection", Name: "SQL 注入"},
		{ID: "xss_injection", Name: "XSS"},
	}
	summary := formatAuditVulnerabilityTypesSummary(categories)
	require.Contains(t, summary, "2 类漏洞类型")
	require.Contains(t, summary, "SQL 注入")
	require.Contains(t, summary, "xss_injection")
}
