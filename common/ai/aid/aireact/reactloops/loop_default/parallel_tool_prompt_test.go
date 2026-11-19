package loop_default

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Keep the scalar-first safety rule in the prompt layers closest to model output.
func TestDefaultPromptTeachesScalarBeforeOptionalBatch(t *testing.T) {
	const batchField = "require_tool_payload"

	for name, test := range map[string]struct {
		prompt           string
		scalarMarker     string
		directBatchRule  string
		requireBatchRule string
	}{
		"instruction": {
			prompt:           aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction,
			scalarMarker:     "标量 `require_tool_payload`",
			directBatchRule:  "严格给出每个完整 JSON 参数",
			requireBatchRule: "仅加载缺少的定义",
		},
		"output_example": {
			prompt:           aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultOutputExample,
			scalarMarker:     `"require_tool_payload":"..[your-toolname].."`,
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

	assert.NotContains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction, "多工具 + 明确依赖图（串行或并行）")
	assert.NotContains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultOutputExample, "已知多工具且存在明确依赖图")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction, "后序参数依赖前序真实输出")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction, "严禁输出 `<@action=...>`")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultOutputExample, "禁止 `<@action=...>`")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction, "`require_tool` 只准备定义，实际执行必须使用 `directly_call_tool`")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultOutputExample, "本轮工具 action 就是执行机会")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction, "默认使用单调用")
	assert.Contains(t, aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction, "嵌套 wrapper")
}

func TestDefaultPromptFallsBackToScalarAfterBatchSchemaFailure(t *testing.T) {
	for name, prompt := range map[string]string{
		"instruction":    aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultInstruction,
		"output_example": aicommon.AgentPrompts(ypb.AgentModeSecurity).DefaultOutputExample,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, prompt, "保留成功")
			assert.Contains(t, prompt, "修正后的单调用")
			assert.Contains(t, prompt, "原样")
			assert.Contains(t, prompt, "批次")
		})
	}
}
