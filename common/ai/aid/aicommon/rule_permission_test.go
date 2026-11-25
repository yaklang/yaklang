package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

func TestParseRulePermissionsKeepsBashOnly(t *testing.T) {
	rules, err := ParseRulePermissions(`{
		"permissions": {
			"allow": ["Bash(git *)", "Read(./public/**)"],
			"ask": ["Bash(git push:*)"],
			"deny": ["Bash(rm -rf *)", "WebFetch(domain:*.example.com)", "mcp__danger__exec"]
		}
	}`)
	require.NoError(t, err)
	require.Len(t, rules, 3)
	for _, rule := range rules {
		require.True(t, isBashPermissionTool(rule.Tool), rule.Raw)
	}
}

func TestBashPermissionCompilesToSyntaxFlow(t *testing.T) {
	rules, err := ParseRulePermissions(`{"permissions":{"deny":["Bash(rm -rf *)"],"ask":["Bash(git push:*)"]}}`)
	require.NoError(t, err)
	sfRules := CompileBashPermissionSyntaxFlowRules(rules)
	require.Len(t, sfRules, 2)
	require.Contains(t, sfRules[0].Content, `/^rm$/ as $cmd`)
	require.Contains(t, sfRules[0].Content, `have: "-rf"`)
	require.Contains(t, sfRules[0].Content, `alert $hit`)
	require.Contains(t, sfRules[1].Content, `/^git$/ as $cmd`)
	require.Contains(t, sfRules[1].Content, `have: "push"`)

	result := runBashSyntaxFlow(context.Background(), "rm -rf /tmp/x", sfRules[:1])
	require.Equal(t, schema.SFR_ACTION_DENY, result.Action)

	ask := runBashSyntaxFlow(context.Background(), "git push origin main", sfRules[1:])
	require.Equal(t, schema.SFR_ACTION_ASK, ask.Action)

	safe := runBashSyntaxFlow(context.Background(), "ls -la", sfRules)
	require.Equal(t, schema.SFR_ACTION_ALLOW, safe.Action)
}

func TestRulePermissionParseErrors(t *testing.T) {
	_, err := ParseRulePermissions(`{"permissions":{"deny":["Bash(rm -rf *"]}}`)
	require.Error(t, err)

	first, err := ParseRulePermissions(`{"permissions":{"deny":["Bash(rm -rf *)"]}}`)
	require.NoError(t, err)
	second, err := ParseRulePermissions(`{"permissions":{"deny":["Bash(rm -rf *)"]}}`)
	require.NoError(t, err)
	require.Equal(t, first[0].RuleID, second[0].RuleID)
}
