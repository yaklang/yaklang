package loopinfra

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestSecurityDeniedFeedbackFromResult(t *testing.T) {
	t.Run("includes-top-finding", func(t *testing.T) {
		result := &aitool.ToolResult{Data: map[string]any{
			"check": map[string]any{
				"findings": []any{
					map[string]any{
						"title_zh": "删除根目录",
						"title":    "rm root",
						"rule_id":  "bash-rm-rf-root",
						"severity": "high",
						"message":  "rm -rf / is blocked",
						"value":    "rm -rf /",
					},
				},
			},
		}}

		feedback := securityDeniedFeedbackFromResult("bash", result)
		require.Contains(t, feedback, "bash-rm-rf-root")
		require.Contains(t, feedback, "删除根目录")
		require.Contains(t, feedback, "high")
		require.Contains(t, feedback, "rm -rf / is blocked")
		require.Contains(t, feedback, "matched: rm -rf /")
		require.Contains(t, feedback, "请自行判断下一步")
	})

	t.Run("falls-back-without-findings", func(t *testing.T) {
		feedback := securityDeniedFeedbackFromResult("bash", &aitool.ToolResult{})
		require.Contains(t, feedback, "security policy blocked the command before execution")
	})
}
