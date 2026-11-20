package aicommon

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

func TestSecurityDeniedTerminalPolicy(t *testing.T) {
	t.Run("typed-error-feedback-forbids-bypass", func(t *testing.T) {
		denied := &SecurityRuleDeniedError{
			ToolName: "bash",
			RuleName: "Bash(rm -rf *)",
			RuleID:   "claude-code-test",
			Severity: schema.SFR_SEVERITY_HIGH,
			Reason:   "destructive command",
		}

		feedback := denied.RetryFeedback()
		require.Contains(t, feedback, "终态")
		require.Contains(t, feedback, "禁止通过以下方式绕过")
		require.Contains(t, feedback, "cmd/powershell")
		require.Contains(t, feedback, "MCP")
		require.Contains(t, feedback, "子智能体")
		require.True(t, IsSecurityRuleDenied(denied))
		require.False(t, IsSecurityRuleDenied(errors.New("ordinary tool error")))
	})

	t.Run("execution-policy-contains-terminal-rule", func(t *testing.T) {
		policy := (&Config{}).GetExecutionPolicy()
		require.Contains(t, policy, "安全拒绝是终态")
		require.Contains(t, policy, "禁止更换 shell")
		require.Contains(t, policy, "安全拒绝不可重试")
	})
}
