package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetScanPolicy(t *testing.T) {
	cfg, err := New(ModeAll)
	require.NoError(t, err)

	policy := &ScanPolicyConfig{PolicyType: PolicyTypeCriticalHigh}
	require.NoError(t, cfg.SetScanPolicy(policy))

	got := cfg.GetScanPolicy()
	require.NotNil(t, got)
	require.Equal(t, PolicyTypeCriticalHigh, got.PolicyType)
	require.ElementsMatch(t, []string{"critical", "high"}, cfg.SyntaxFlowRule.RuleFilter.GroupNames)
}

func TestWithJsonRawConfigAppliesScanPolicyToRuleFilter(t *testing.T) {
	raw := []byte(`{
		"Mode": 127,
		"ScanPolicy": {
			"policy_type": "custom",
			"custom_rules": {
				"compliance_rules": ["OWASP 2021 A03:Injection"],
				"tech_stack_rules": ["go"],
				"special_rules": ["high"]
			}
		}
	}`)

	cfg, err := New(ModeAll, WithJsonRawConfig(raw))
	require.NoError(t, err)
	require.NotNil(t, cfg.GetScanPolicy())
	require.NotNil(t, cfg.SyntaxFlowRule)
	require.NotNil(t, cfg.SyntaxFlowRule.RuleFilter)
	require.ElementsMatch(
		t,
		[]string{"OWASP 2021 A03:Injection", "go", "high"},
		cfg.SyntaxFlowRule.RuleFilter.GroupNames,
	)
}
