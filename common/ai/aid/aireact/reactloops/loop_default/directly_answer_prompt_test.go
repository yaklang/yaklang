package loop_default

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestOutputExampleMakesDirectAnswerCarriersMutuallyExclusive(t *testing.T) {
	shortRule := "JSON 结束后立即停止，禁止再输出 `FINAL_ANSWER` AI Tag"
	longRule := "省略 `answer_payload` 或将其保持为空，仅使用 `FINAL_ANSWER` AI Tag"
	shortExample := `{"@action": "directly_answer", "answer_payload": "...[your-answer not a markdown].."}`
	longExample := `<|FINAL_ANSWER_CURRENT_NONCE|>`

	shortRuleIndex := strings.Index(aicommon.AgentPrompts("security").DefaultOutputExample, shortRule)
	longRuleIndex := strings.Index(aicommon.AgentPrompts("security").DefaultOutputExample, longRule)
	shortExampleIndex := strings.Index(aicommon.AgentPrompts("security").DefaultOutputExample, shortExample)
	longExampleIndex := strings.Index(aicommon.AgentPrompts("security").DefaultOutputExample, longExample)

	require.NotEqual(t, -1, shortRuleIndex)
	require.NotEqual(t, -1, longRuleIndex)
	require.NotEqual(t, -1, shortExampleIndex)
	require.NotEqual(t, -1, longExampleIndex)
	assert.Less(t, shortRuleIndex, shortExampleIndex)
	assert.Less(t, longRuleIndex, longExampleIndex)
	assert.Contains(t, aicommon.AgentPrompts("security").DefaultOutputExample, "两个示例是互斥的独立响应")
	assert.Contains(t, aicommon.AgentPrompts("security").DefaultOutputExample, "任一响应只能出现其中一种答案内容")
}

func TestDefaultInstructionUsesRequestOutputMode(t *testing.T) {
	textInstruction, err := utils.RenderTemplate(aicommon.AgentPrompts("security").DefaultInstruction, map[string]any{"FunctionCallMode": false})
	require.NoError(t, err)
	functionInstruction, err := utils.RenderTemplate(aicommon.AgentPrompts("security").DefaultInstruction, map[string]any{"FunctionCallMode": true})
	require.NoError(t, err)
	require.Contains(t, textInstruction, `{"@action":"require_tool"`)
	require.NotContains(t, functionInstruction, `{"@action":"require_tool"`)
	require.Contains(t, functionInstruction, "通过原生工具调用选择 action")
	require.Contains(t, functionInstruction, "正文放在工具参数 `answer_payload`")
	for _, prompt := range []string{aicommon.AgentPrompts("security").DefaultInstruction, aicommon.AgentPrompts("security").DefaultOutputExample, textInstruction, functionInstruction} {
		for _, forbidden := range []string{"R2", "# Tool Context", "# Blueprint Context", "call-tool", "call-ai-blueprint", "OLD_PARAMS", "参数生成场景", "第二章"} {
			require.NotContains(t, prompt, forbidden)
		}
	}
	for _, prompt := range []string{textInstruction, functionInstruction} {
		require.Contains(t, prompt, "Shell Safety Rules")
		require.Contains(t, prompt, "提交回执与收尾约束")
	}
}
