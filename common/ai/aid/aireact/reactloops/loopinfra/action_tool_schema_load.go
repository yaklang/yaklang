package loopinfra

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const actionStateToolSchemaNames = "native_tool_schema_names"
const requireToolPayloadField = "require_tool_payload"

func requireToolPayloadOption(native bool) aitool.ToolOption {
	description := "一个工具名或工具名数组；仅加载真实 Schema 到 AI TOOL CACHE，不包含执行参数。加载后按 CACHE_TOOL_CALL 立即构参并通过 directly_call_tool 继续执行。"
	if native {
		description += ` arguments 示例：{"require_tool_payload":"grep"} 或 {"require_tool_payload":["grep","read_file"]}。`
	} else {
		description += "\n" + requireToolScalarOutputExampleJSON + "\n" + requireToolBatchOutputExampleJSON
	}
	return aitool.WithRawParam(requireToolPayloadField, map[string]any{
		"oneOf": []any{
			map[string]any{"type": "string", "minLength": 1},
			map[string]any{"type": "array", "minItems": 1, "maxItems": aicommon.DefaultToolBatchMaxCalls, "items": map[string]any{"type": "string", "minLength": 1}},
		},
	}, aitool.WithParam_Required(true), aitool.WithParam_Description(description))
}

var nativeToolSchemaLoadAction = &reactloops.LoopAction{
	ActionType:     schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description:    "只加载业务工具完整参数 Schema 到 AI TOOL CACHE，不生成参数、不执行工具。require_tool_payload 支持一个工具名或名称数组。加载后立即按 Schema 构参并通过 directly_call_tool 完成本任务；独立调用可用参数分组，有依赖的调用按顺序执行。已有 Schema 直接复用。",
	Options:        []aitool.ToolOption{requireToolPayloadOption(true)},
	ActionVerifier: verifyToolSchemaLoad, ActionHandler: loadToolSchemas,
}

func toolSchemaNames(raw any, max int) ([]string, error) {
	var names []string
	switch value := raw.(type) {
	case string:
		names = []string{value}
	case []string:
		names = value
	case []any:
		for _, item := range value {
			name, ok := item.(string)
			if !ok {
				return nil, utils.Error("require_tool_payload array must contain only tool names")
			}
			names = append(names, name)
		}
	default:
		return nil, utils.Error("require_tool_payload must be a tool name or an array of tool names")
	}
	if len(names) == 0 || len(names) > max {
		return nil, utils.Errorf("require_tool_payload must contain 1-%d tool names", max)
	}
	seen := make(map[string]bool)
	unique := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, utils.Error("require_tool_payload contains an empty tool name")
		}
		if !seen[name] {
			seen[name] = true
			unique = append(unique, name)
		}
	}
	return unique, nil
}
func verifyToolSchemaLoad(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, nil)
	if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
		return err
	}
	if hasAnyCanonicalActionParam(action, retiredRequireToolBatchField, "directly_call_tool_calls", directlyCallToolBatchField, "directly_call_tool_name", "directly_call_tool_params", "directly_call_identifier", "directly_call_expectations", "directly_call_reason") {
		return utils.Error("reason: require_tool only loads schemas; retry: provide require_tool_payload without retired batch or direct-call fields")
	}
	raw, ok := lookupCanonicalActionParam(action, requireToolPayloadField)
	old, legacy := lookupCanonicalActionParam(action, "tool_require_payload")
	if ok && legacy {
		return utils.Error("require_tool_payload cannot be combined with tool_require_payload")
	}
	if !ok {
		raw = old
	}
	names, err := toolSchemaNames(raw, toolBatchMaxCalls(loop))
	if err != nil {
		return err
	}
	loop.SetActionExecutionValue(action, actionStateToolSchemaNames, names)
	return nil
}
func loadToolSchemas(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	if err := verifyToolSchemaLoad(loop, action); err != nil {
		operator.Feedback("Tool schema loading rejected: " + err.Error())
		operator.Continue()
		return
	}
	names, _ := loop.GetActionExecutionValue(action, actionStateToolSchemaNames).([]string)
	loadToolSchemasByNames(loop, names, operator)
}

// Both schema loading entries honor the same MCP policy and cache budget.
func loadToolSchemasByNames(loop *reactloops.ReActLoop, names []string, operator *reactloops.LoopActionHandlerOperator) bool {
	config := loop.GetConfig()
	if len(names) == 0 || config == nil || config.GetAiToolManager() == nil {
		operator.Feedback("Tool schema loading unavailable; no tools were executed.")
		operator.Continue()
		return false
	}
	results := loop.LoadToolSchemas(toolBatchVerifierContext(loop), names)
	allLoaded := true
	for _, entry := range results {
		if entry["status"] != "schema_loaded" {
			allLoaded = false
		}
	}
	body, _ := json.Marshal(results)
	hint := "根据刚加载到 CACHE_TOOL_CALL 的 Schema 构参，立即调用 directly_call_tool；独立操作可用 directly_call_tool_params_group 一次提交，依赖操作按顺序执行。只使用加载成功的工具；本次未执行，不要结束任务或等待用户继续。"
	loop.GetInvoker().AddToTimeline("tool_schema_load", string(body)+"\n"+hint)
	operator.Feedback(string(body) + "\n" + hint)
	operator.Continue()
	return allLoaded
}
