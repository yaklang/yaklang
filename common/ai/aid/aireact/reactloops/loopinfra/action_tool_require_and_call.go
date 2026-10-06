package loopinfra

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	FunctionCallAction: nativeToolSchemaLoadAction,
	ActionType:         schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "只加载业务工具完整参数 Schema 到 Timeline，不生成参数、不执行工具。" +
		"单个名称用 tool_require_payload，多个名称用 tool_require_calls。下一轮读取 CACHE_TOOL_CALL，按真实字段自行构造参数并调用 directly_call_tool；已有完整 Schema 时直接复用。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("选择单调用形式时填写；存在 tool_require_calls 时必须省略。只填写一个需要加载 Schema 的工具准确名称，严禁包含参数。下面是单工具 Schema 加载格式：\n"+requireToolScalarOutputExampleJSON),
		),
		aitool.WithStringParam(
			"tool_call_reason",
			aitool.WithParam_Description(`可选。用简短短语说明这次调用具体做什么，例如“读取配置文件的参数定义”或“加载文件搜索工具的 Schema”。不要写前序总结或过渡语；仅当 human_readable_thought 已说明原因时省略。该内容说明加载定义的用途，不表示工具已执行。`),
		),
		requireToolBatchSchemaOption(),
	},
	OutputExamples: requireToolOutputExamples,
	ActionVerifier: verifyRequireToolSchemaLoad,
	ActionHandler:  loadToolSchemas,
}

// Preserve field-level streaming for scalar text actions. The shared handler
// validates the complete action before loading schemas or mutating the cache.
func verifyRequireToolSchemaLoad(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, nil)
	raw, exists := action.LookupParam("tool_require_payload")
	if !exists {
		raw = action.GetInvokeParams("next_action")["tool_require_payload"]
	}
	if raw == nil {
		return verifyToolSchemaLoad(loop, action)
	}
	name, valid := raw.(string)
	name = strings.TrimSpace(name)
	if !valid || name == "" {
		return utils.Error("tool_require_payload must be a non-empty tool name")
	}
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, []string{name})
	reactloops.MaybeWarnBashBeforeEdit(loop, name)
	return nil
}
