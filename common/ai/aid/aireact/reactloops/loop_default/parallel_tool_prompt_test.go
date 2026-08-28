package loop_default

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keep the batch-first rule in the prompt layers closest to model output.
// Earlier generic guidance is not enough: a later scalar-only example used to
// outweigh it and made otherwise independent calls run in separate ReAct
// rounds.
func TestDefaultPromptTeachesIndependentBatchBeforeScalarFallback(t *testing.T) {
	const batchField = "tool_require_calls"

	for name, test := range map[string]struct {
		prompt       string
		scalarMarker string
		noGuessRule  string
	}{
		"instruction": {
			prompt:       instruction,
			scalarMarker: "标量 `tool_require_payload`",
			noGuessRule:  "严禁猜测",
		},
		"output_example": {
			prompt:       outputExample,
			scalarMarker: `"tool_require_payload":"..[your-toolname].."`,
			noGuessRule:  "禁止猜参数",
		},
	} {
		t.Run(name, func(t *testing.T) {
			batchIndex := strings.Index(test.prompt, batchField)
			scalarIndex := strings.Index(test.prompt, test.scalarMarker)
			require.NotEqual(t, -1, batchIndex, "prompt must show the batch action field")
			require.NotEqual(t, -1, scalarIndex, "prompt must retain a one-call fallback")
			assert.Less(t, batchIndex, scalarIndex, "batch form must be taught before scalar fallback")
			assert.Contains(t, test.prompt, test.noGuessRule)
		})
	}

	assert.NotContains(t, instruction, "多工具 + 明确依赖图（串行或并行）")
	assert.NotContains(t, outputExample, "已知多工具且存在明确依赖图")
	assert.Contains(t, instruction, "后序参数依赖前序真实输出")
	assert.Contains(t, instruction, "严禁输出 `<@action=...>`")
	assert.Contains(t, outputExample, "禁止 `<@action=...>`")
	assert.Contains(t, instruction, "本轮输出 `require_tool` / `directly_call_tool` action 本身就是获得工具执行机会")
	assert.Contains(t, outputExample, "本轮工具 action 就是执行机会")
	assert.Contains(t, instruction, "恰好 1 个用标量；2-8 个且独立用批次")
}

func TestDefaultPromptKeepsBatchingAfterCorrectableFailures(t *testing.T) {
	for name, prompt := range map[string]string{
		"instruction":    instruction,
		"output_example": outputExample,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, prompt, "只否定本次载荷或假设")
			assert.Contains(t, prompt, "不否定并发策略")
			assert.Contains(t, prompt, "禁止原样")
			assert.Contains(t, prompt, "历史证据")
			assert.Contains(t, prompt, "永久单工具")
		})
	}

	assert.Contains(t, instruction, "2-4 个低成本、安全且可独立验证的候选修复或探测方案")
	assert.Contains(t, instruction, "batch admission failed")
	assert.Contains(t, instruction, "重新枚举本轮仍独立的 2-8 个调用继续批量提交")
	assert.Contains(t, outputExample, "2-4 个有区分力的候选方案继续组成批次")
	assert.Contains(t, outputExample, "“之前批量失败过”本身不是标量选择条件")
}
