package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultVulnCategories_ExpandedCoverage(t *testing.T) {
	require.GreaterOrEqual(t, len(DefaultVulnCategories), 13)

	want := []string{
		"sql_injection",
		"cmd_injection",
		"path_traversal",
		"xxe_ssrf",
		"deserialization",
		"auth_bypass",
		"xss_injection",
		"code_execution",
		"expression_injection",
		"memory_safety",
		"resource_exhaustion",
		"header_injection",
		"race_condition",
	}
	got := make(map[string]bool, len(DefaultVulnCategories))
	for _, c := range DefaultVulnCategories {
		require.NotEmpty(t, c.ID)
		require.NotEmpty(t, c.Name)
		require.NotEmpty(t, c.Instruction)
		require.NotEmpty(t, c.SinkHints)
		got[c.ID] = true
	}
	for _, id := range want {
		require.True(t, got[id], "missing category %s", id)
	}
}
