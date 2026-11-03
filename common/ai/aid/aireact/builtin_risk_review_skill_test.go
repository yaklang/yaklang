package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRiskReviewSkillResources(t *testing.T) {
	fs := GetBuiltinSkillsFS()
	require.NotNil(t, fs)

	for _, tc := range []struct {
		path     string
		contains []string
	}{
		{
			path: "skills/risk-review/SKILL.md",
			contains: []string{
				"query_cybersecurity_risk",
				"get_cybersecurity_risk_evidence",
				"do_http_request",
				"triage_cybersecurity_risks",
				"@risk-review/references/duplicate-rules.md",
			},
		},
		{
			path: "skills/risk-review/references/duplicate-rules.md",
			contains: []string{
				"Canonical 选择",
				"禁止重复链和自引用",
			},
		},
		{
			path: "skills/risk-review/references/verification-playbook.md",
			contains: []string{
				"IDOR / 越权",
				"标记 `inconclusive`",
			},
		},
		{
			path: "skills/risk-review/references/report-format.md",
			contains: []string{
				"Duplicate Groups",
				"False Positives",
				"Database Changes",
			},
		},
	} {
		content, err := fs.ReadFile(tc.path)
		require.NoError(t, err, tc.path)
		text := string(content)
		for _, expected := range tc.contains {
			require.Contains(t, text, expected, tc.path)
		}
	}
}
