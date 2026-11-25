package aicommon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestRulePermissionPolicy(t *testing.T) {
	gate, err := CompileRulePermissions(`{
		"permissions": {
			"allow": ["Bash(git *)", "Bash(ls *)", "Read(./public/**)"],
			"ask": ["Bash(git push:*)"],
			"deny": [
				"Bash(rm -rf *)",
				"Bash(git push --force *)",
				"Read(./.env)",
				"WebFetch(domain:*.example.com)",
				"mcp__danger__exec"
			]
		}
	}`)
	require.NoError(t, err)
	require.NotNil(t, gate)

	t.Run("deny-read-env", func(t *testing.T) {
		decision := gate(context.Background(), "read_file", aitool.InvokeParams{"file": `C:\work\project\.env`})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("deny-web-domain", func(t *testing.T) {
		decision := gate(context.Background(), "webfetch", aitool.InvokeParams{"url": "https://a.example.com/x"})
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("deny-generic-tool", func(t *testing.T) {
		decision := gate(context.Background(), "mcp__danger__exec", nil)
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("bash-handled-by-syntaxflow-not-gate", func(t *testing.T) {
		// Bash entries are compiled to SyntaxFlow; the non-bash gate ignores them.
		require.Nil(t, gate(context.Background(), "bash", aitool.InvokeParams{"command": "rm -rf /tmp/x"}))
	})
}

func TestBashPermissionCompilesToSyntaxFlow(t *testing.T) {
	rules, err := ParseRulePermissions(`{"permissions":{"deny":["Bash(rm -rf *)"],"ask":["Bash(git push:*)"]}}`)
	require.NoError(t, err)
	sfRules := CompileBashPermissionSyntaxFlowRules(rules)
	require.Len(t, sfRules, 2)
	require.Contains(t, sfRules[0].Content, `as $hit`)
	require.Contains(t, sfRules[0].Content, `alert $hit`)

	result := checkBashCommandWithRulesForTest(context.Background(), "rm -rf /tmp/x", sfRules[:1])
	require.Equal(t, schema.SFR_ACTION_DENY, result.Action)
}

// checkBashCommandWithRulesForTest exposes runBashSyntaxFlow for focused tests.
func checkBashCommandWithRulesForTest(ctx context.Context, cmd string, rules []*schema.SyntaxFlowRule) BashRuleVerdict {
	return runBashSyntaxFlow(ctx, cmd, rules)
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

func TestComposeRulePermissionGates(t *testing.T) {
	builtin := func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
		return &ToolCallRuleDecision{Action: schema.SFR_ACTION_DENY, RuleName: "builtin"}
	}
	userAllow := func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
		return &ToolCallRuleDecision{Action: schema.SFR_ACTION_ALLOW, RuleName: "user-allow"}
	}
	decision := ComposeToolCallRuleGates(builtin, userAllow)(context.Background(), "bash", nil)
	require.NotNil(t, decision)
	require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
}
