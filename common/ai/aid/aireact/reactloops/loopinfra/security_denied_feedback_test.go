package loopinfra

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestSecurityDeniedFeedbackFromResult(t *testing.T) {
	t.Run("includes-process-error", func(t *testing.T) {
		result := &aitool.ToolResult{Data: map[string]any{
			"security_denied": true,
			"process_error":   "command denied by rule permission policy. The command was not executed.",
		}}

		feedback := securityDeniedFeedbackFromResult("bash", result)
		require.Contains(t, feedback, "规则权限策略拒绝了工具")
		require.Contains(t, feedback, "command denied by rule permission policy")
		require.Contains(t, feedback, "请自行判断下一步")
	})

	t.Run("falls-back-without-process-error", func(t *testing.T) {
		feedback := securityDeniedFeedbackFromResult("bash", &aitool.ToolResult{})
		require.Contains(t, feedback, "rule permission policy blocked the command before execution")
	})
}
