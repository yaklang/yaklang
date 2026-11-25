package aicommon

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/schema"
)

func TestSecurityDeniedFeedbackPolicy(t *testing.T) {
	t.Run("typed-error-feedback-explains-scope", func(t *testing.T) {
		denied := &SecurityRuleDeniedError{
			ToolName: "bash",
			RuleName: "Bash(rm -rf *)",
			RuleID:   "tool-permission-test",
			Severity: schema.SFR_SEVERITY_HIGH,
			Reason:   "destructive command",
		}

		feedback := denied.RetryFeedback()
		require.Contains(t, feedback, "不是整个任务的终态")
		require.Contains(t, feedback, "请自行判断下一步")
		require.Contains(t, feedback, "同一操作")
		require.Contains(t, feedback, "无关的独立工作")
		require.Contains(t, feedback, "cmd/powershell")
		require.Contains(t, feedback, "MCP")
		require.Contains(t, feedback, "子智能体")
		require.NotContains(t, feedback, "这是终态")
		require.True(t, IsSecurityRuleDenied(denied))
		require.False(t, IsSecurityRuleDenied(errors.New("ordinary tool error")))
	})

	t.Run("execution-policy-does-not-own-tool-feedback", func(t *testing.T) {
		policy := (&Config{}).GetExecutionPolicy()
		require.NotContains(t, policy, "安全拒绝")
	})
}
