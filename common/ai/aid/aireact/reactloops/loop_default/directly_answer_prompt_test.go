package loop_default

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputExampleMakesDirectAnswerCarriersMutuallyExclusive(t *testing.T) {
	shortRule := "JSON 结束后立即停止，禁止再输出 `FINAL_ANSWER` AI Tag"
	longRule := "省略 `answer_payload` 或将其保持为空，仅使用 `FINAL_ANSWER` AI Tag"
	shortExample := `{"@action": "directly_answer", "answer_payload": "...[your-answer not a markdown].."}`
	longExample := `<|FINAL_ANSWER_CURRENT_NONCE|>`

	shortRuleIndex := strings.Index(outputExample, shortRule)
	longRuleIndex := strings.Index(outputExample, longRule)
	shortExampleIndex := strings.Index(outputExample, shortExample)
	longExampleIndex := strings.Index(outputExample, longExample)

	require.NotEqual(t, -1, shortRuleIndex)
	require.NotEqual(t, -1, longRuleIndex)
	require.NotEqual(t, -1, shortExampleIndex)
	require.NotEqual(t, -1, longExampleIndex)
	assert.Less(t, shortRuleIndex, shortExampleIndex)
	assert.Less(t, longRuleIndex, longExampleIndex)
	assert.Contains(t, outputExample, "两个示例是互斥的独立响应")
	assert.Contains(t, outputExample, "任一响应只能出现其中一种答案内容")
}
