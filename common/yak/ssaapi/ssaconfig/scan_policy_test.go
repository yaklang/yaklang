package ssaconfig

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleGroupTaxonomyIsVersionedAndComplete(t *testing.T) {
	taxonomy := GetRuleGroupTaxonomy()
	require.Equal(t, RuleGroupTaxonomySchemaVersion, taxonomy.SchemaVersion)
	require.Equal(t, "1.0", taxonomy.Version)
	require.NotEmpty(t, taxonomy.Categories)

	groups := make(map[string]RuleGroupTaxonomyItem)
	categories := make(map[string]RuleGroupTaxonomyCategory)
	for _, category := range taxonomy.Categories {
		require.NotEmpty(t, category.ID)
		require.NotEmpty(t, category.DisplayName)
		_, duplicateCategory := categories[category.ID]
		require.False(t, duplicateCategory, "duplicate taxonomy category %q", category.ID)
		categories[category.ID] = category
		for _, group := range category.Groups {
			require.NotEmpty(t, group.Name)
			require.NotEmpty(t, group.DisplayName)
			_, duplicateGroup := groups[group.Name]
			require.False(t, duplicateGroup, "duplicate taxonomy group %q", group.Name)
			groups[group.Name] = group
		}
	}

	// A matching count does not prove coverage: the exported raw names come
	// from engine language values, not policy aliases such as go/javascript.
	for _, language := range append(GetAllSupportedLanguages(), General.String()) {
		require.Contains(t, groups, language, "engine language group must have exact taxonomy metadata")
	}
	for rawName, canonical := range map[string]string{
		"go": "go", "golang": "go", "javascript": "javascript", "js": "javascript",
		"general": "general", "yak": "yak", "ts": "typescript",
	} {
		require.Equal(t, canonical, groups[rawName].CanonicalName, "raw group %q", rawName)
	}
	require.Equal(t, "java", groups["Language Library - Java"].CanonicalName)
	require.Equal(t, 3, categories["language"].Order)
	require.Equal(t, 1, groups["java"].Order)
	require.Equal(t, "信息", groups["info"].DisplayName)
	require.Contains(t, categories, "code-quality")
	require.ElementsMatch(t, GetAllStandardGroupNames(), mapKeys(groups))
	for policyName, policy := range GetScanPoliciesConfig().Policies {
		for _, groupName := range policy.RuleGroups {
			require.Contains(t, groups, groupName, "policy %q references a non-standard group", policyName)
		}
	}
}

func TestTaxonomyDoesNotChangeExistingPolicyGroupSelection(t *testing.T) {
	require.Equal(t, []string{
		"critical", "high", "middle", "java", "go", "php", "javascript", "python",
		"Framework - Spring", "Framework - Apache Shiro", "Framework - ThinkPHP", "SCA - Dependency Check",
	}, (&ScanPolicyConfig{PolicyType: PolicyTypeFullStack}).MapToGroups())
}

func mapKeys[K comparable, V any](values map[K]V) []K {
	keys := make([]K, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

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
