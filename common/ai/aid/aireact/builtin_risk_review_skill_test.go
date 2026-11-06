package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRiskReviewSkillIsSelfContained(t *testing.T) {
	fs := GetBuiltinSkillsFS()
	require.NotNil(t, fs)

	content, err := fs.ReadFile("skills/risk-review/SKILL.md")
	require.NoError(t, err)
	text := string(content)
	for _, expected := range []string{
		"query_cybersecurity_risk",
		"get_cybersecurity_risk_evidence",
		"do_http_request",
		"triage_cybersecurity_risks",
		"canonical",
		"false_positive",
		"inconclusive",
	} {
		require.Contains(t, text, expected)
	}
	require.NotContains(t, text, "@risk-review/references/")

	entries, err := fs.ReadDir("skills/risk-review")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "SKILL.md", entries[0].Name())
}
