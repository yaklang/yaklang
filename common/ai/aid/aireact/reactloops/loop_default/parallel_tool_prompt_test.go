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
