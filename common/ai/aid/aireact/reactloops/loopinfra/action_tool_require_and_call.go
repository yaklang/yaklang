package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	FunctionCallAction: nativeToolSchemaLoadAction,
	ActionType:         schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description:        nativeToolSchemaLoadAction.Description,
	Options:            []aitool.ToolOption{requireToolPayloadOption(false), aitool.WithStringParam("tool_call_reason", aitool.WithParam_Description("可选。简述加载工具定义的用途，不表示工具已执行。"))},
	OutputExamples:     requireToolOutputExamples,
	ActionVerifier:     verifyRequireToolSchemaLoad, ActionHandler: loadToolSchemas,
}

// Scalar text names may stream before EOF. The handler validates the complete
// proposal before loading any schema; arrays always wait for the full object.
func verifyRequireToolSchemaLoad(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, nil)
	raw, ok := action.LookupParam(requireToolPayloadField)
	if !ok {
		raw, ok = action.LookupParam("tool_require_payload")
	}
	if name, scalar := raw.(string); ok && scalar {
		names, err := toolSchemaNames(name, toolBatchMaxCalls(loop))
		if err != nil {
			return err
		}
		reactloops.MaybeWarnBashBeforeEdit(loop, names[0])
		loop.SetActionExecutionValue(action, actionStateToolSchemaNames, names)
		return nil
	}
	return verifyToolSchemaLoad(loop, action)
}
