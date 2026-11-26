package aicommon

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityDeniedFeedbackPolicy(t *testing.T) {
	t.Run("typed-error-feedback-explains-scope", func(t *testing.T) {
		denied := &SecurityRuleDeniedError{
			ToolName: "bash",
			Reason:   "destructive command",
		}

		feedback := denied.RetryFeedback()
		require.Contains(t, feedback, "不是整个任务的终态")
		require.Contains(t, feedback, "请自行判断下一步")
		require.Contains(t, feedback, "判定：拒绝")
		require.Contains(t, feedback, "cmd/powershell")
		require.NotContains(t, feedback, "级别：")
		require.True(t, IsSecurityRuleDenied(denied))
		require.False(t, IsSecurityRuleDenied(errors.New("ordinary tool error")))
	})

	t.Run("execution-policy-does-not-own-tool-feedback", func(t *testing.T) {
		policy := (&Config{}).GetExecutionPolicy()
		require.NotContains(t, policy, "安全拒绝")
	})
}
