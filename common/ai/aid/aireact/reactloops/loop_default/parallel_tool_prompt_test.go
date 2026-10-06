package loop_default

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Keep the scalar-first safety rule in the prompt layers closest to model output.
func TestDefaultPromptTeachesScalarBeforeOptionalBatch(t *testing.T) {
	const batchField = "tool_require_calls"

	for name, test := range map[string]struct {
		prompt           string
		scalarMarker     string
		directBatchRule  string
		requireBatchRule string
	}{
		"instruction": {
			prompt:           instruction,
			scalarMarker:     "标量 `tool_require_payload`",
			directBatchRule:  "严格给出每个完整 JSON 参数",
			requireBatchRule: "仅加载缺少的定义",
		},
		"output_example": {
			prompt:           outputExample,
			scalarMarker:     `"tool_require_payload":"..[your-toolname].."`,
			directBatchRule:  "每层参数全部明确",
			requireBatchRule: "不生成参数、不执行工具",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, test.prompt, batchField)
			assert.Contains(t, test.prompt, test.scalarMarker)
			assert.Contains(t, test.prompt, test.directBatchRule)
			assert.Contains(t, test.prompt, test.requireBatchRule)
		})
	}

	assert.NotContains(t, instruction, "多工具 + 明确依赖图（串行或并行）")
	assert.NotContains(t, outputExample, "已知多工具且存在明确依赖图")
	assert.Contains(t, instruction, "后序参数依赖前序真实输出")
	assert.Contains(t, instruction, "严禁输出 `<@action=...>`")
	assert.Contains(t, outputExample, "禁止 `<@action=...>`")
	assert.Contains(t, instruction, "`require_tool` 只准备定义，实际执行必须使用 `directly_call_tool`")
	assert.Contains(t, outputExample, "本轮工具 action 就是执行机会")
	assert.Contains(t, instruction, "默认使用单调用")
	assert.Contains(t, instruction, "嵌套 wrapper")
}

func TestDefaultPromptFallsBackToScalarAfterBatchSchemaFailure(t *testing.T) {
	for name, prompt := range map[string]string{
		"instruction":    instruction,
		"output_example": outputExample,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, prompt, "保留成功")
			assert.Contains(t, prompt, "修正后的单调用")
			assert.Contains(t, prompt, "原样")
			assert.Contains(t, prompt, "批次")
		})
	}
}
