package loopinfra

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const directlyCallToolParamsNodeID = "directly_call_tool_params"

func getDirectlyCallToolParamPayload(action *aicommon.Action) (string, aitool.InvokeParams) {
	raw := action.GetString("directly_call_tool_params")
	obj := action.GetInvokeParams("directly_call_tool_params")
	if raw != "" || len(obj) > 0 {
		return raw, obj
	}
	nextAction := action.GetInvokeParams("next_action")
	return nextAction.GetString("directly_call_tool_params"), nextAction.GetObject("directly_call_tool_params")
}

func normalizeDirectlyCallToolParams(raw string, obj aitool.InvokeParams) (aitool.InvokeParams, []string) {
	var notes []string
	if strings.TrimSpace(raw) != "" {
		params, parseNotes := unwrapDirectlyCallToolParamsValue(raw, 0)
		notes = append(notes, parseNotes...)
		if len(params) > 0 {
			return params, notes
		}
		notes = append(notes, "directly_call_tool_params string parse did not yield a usable params object; falling back to structured extraction")
	}
	if len(obj) > 0 {
		params, parseNotes := unwrapDirectlyCallToolParamsValue(obj, 0)
		notes = append(notes, parseNotes...)
		if len(params) > 0 {
			return params, notes
		}
	}
	return nil, notes
}

func unwrapDirectlyCallToolParamsValue(value any, depth int) (aitool.InvokeParams, []string) {
	if depth > 4 || value == nil {
		return nil, nil
	}

	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		parsed := make(aitool.InvokeParams)
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, []string{fmt.Sprintf("invalid JSON string for directly_call_tool_params: %v", err)}
		}
		params, notes := unwrapDirectlyCallToolParamsMap(parsed, depth+1)
		return params, append([]string{"parsed directly_call_tool_params from JSON string"}, notes...)
	default:
		obj := aitool.InvokeParams(utils.InterfaceToGeneralMap(value))
		if len(obj) == 0 {
			return nil, nil
		}
		return unwrapDirectlyCallToolParamsMap(obj, depth+1)
	}
}

func unwrapDirectlyCallToolParamsMap(obj aitool.InvokeParams, depth int) (aitool.InvokeParams, []string) {
	if depth > 4 || len(obj) == 0 {
		return nil, nil
	}

	if nextAction := obj.GetObject("next_action"); len(nextAction) > 0 {
		params, notes := unwrapDirectlyCallToolParamsMap(nextAction, depth+1)
		if len(params) > 0 {
			return params, append([]string{"unwrapped next_action wrapper"}, notes...)
		}
	}

	if nested := obj.GetObject("directly_call_tool_params"); len(nested) > 0 {
		params, notes := unwrapDirectlyCallToolParamsMap(nested, depth+1)
		if len(params) > 0 {
			return params, append([]string{"unwrapped nested directly_call_tool_params object"}, notes...)
		}
	}
	if nestedRaw := obj.GetString("directly_call_tool_params"); strings.TrimSpace(nestedRaw) != "" {
		params, notes := unwrapDirectlyCallToolParamsValue(nestedRaw, depth+1)
		if len(params) > 0 {
			return params, append([]string{"unwrapped nested directly_call_tool_params string"}, notes...)
		}
	}

	if nestedTool := obj.GetObject("tool"); len(nestedTool) > 0 {
		if nestedParams := nestedTool.GetObject("params"); len(nestedParams) > 0 {
			params, notes := unwrapDirectlyCallToolParamsMap(nestedParams, depth+1)
			if len(params) > 0 {
				return params, append([]string{"unwrapped legacy tool.params wrapper"}, notes...)
			}
		}
	}

	if nestedParams := obj.GetObject("params"); len(nestedParams) > 0 && looksLikeWrappedDirectlyCallPayload(obj) {
		params, notes := unwrapDirectlyCallToolParamsMap(nestedParams, depth+1)
		if len(params) > 0 {
			return params, append([]string{"unwrapped legacy params wrapper"}, notes...)
		}
	}

	dropWrapperKeys := looksLikeWrappedDirectlyCallPayload(obj)
	cleaned := cleanDirectlyCallToolParams(obj, dropWrapperKeys)
	if len(cleaned) == 0 {
		return nil, nil
	}
	if dropWrapperKeys {
		return cleaned, []string{"discarded legacy directly_call_tool wrapper fields"}
	}
	return cleaned, []string{"using directly_call_tool_params object as-is"}
}

func looksLikeWrappedDirectlyCallPayload(params aitool.InvokeParams) bool {
	if len(params) == 0 {
		return false
	}
	if params.GetString("@action") != "" || params.GetString("tool") != "" || params.GetString("tool_name") != "" {
		return true
	}
	if params.GetString("type") == schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL {
		return true
	}
	if len(params.GetObject("params")) > 0 || len(params.GetObject("tool")) > 0 || len(params.GetObject("next_action")) > 0 {
		return true
	}
	return false
}

func cleanDirectlyCallToolParams(params aitool.InvokeParams, dropWrapperKeys bool) aitool.InvokeParams {
	cleaned := make(aitool.InvokeParams)
	for key, value := range params {
		if isDirectlyCallInternalKey(key) {
			continue
		}
		if dropWrapperKeys && isDirectlyCallWrapperKey(key) {
			continue
		}
		cleaned[key] = value
	}
	return cleaned
}

func isDirectlyCallInternalKey(key string) bool {
	switch key {
	case "__DEFAULT__", "__FALLBACK__", "__[yaklang-raw]__":
		return true
	default:
		return false
	}
}

func isDirectlyCallWrapperKey(key string) bool {
	switch key {
	case "@action", "tool", "tool_name", "params", "type", "next_action", "directly_call_tool_name", "directly_call_tool_params":
		return true
	default:
		return false
	}
}

func directlyCallParamKeys(params aitool.InvokeParams) []string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var loopAction_directlyCallTool = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
	Description: "directly call a recently used tool (skip require & param-generation phases). " +
		"Only tools listed in the CACHE_TOOL_CALL block are eligible. " +
		"Provide directly_call_tool_name AND directly_call_tool_params together.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"directly_call_tool_name",
			aitool.WithParam_Description(`MUST set when @action is "directly_call_tool". The name of the tool to call. Must be one of the cached recently-used tools.`),
		),
		aitool.WithStringParam(
			"directly_call_tool_params",
			aitool.WithParam_Description(`MUST set when @action is "directly_call_tool". A JSON object containing the tool invocation parameters. Refer to the cached tool's Params Schema in the CACHE_TOOL_CALL block for the correct structure.`),
		),
		aitool.WithStringParam(
			"directly_call_identifier",
			aitool.WithParam_Description(`short snake_case label describing the PURPOSE of this tool call, e.g. "scan_port_443", "query_large_file". Used for report file naming.`),
		),
		aitool.WithStringParam(
			"directly_call_expectations",
			aitool.WithParam_Description(`estimated timing and fallback strategy, e.g. "~3s, force stop if >10s". Used for interval review during execution.`),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		toolName := action.GetString("directly_call_tool_name")
		if toolName == "" {
			toolName = action.GetInvokeParams("next_action").GetString("directly_call_tool_name")
		}
		if toolName == "" {
			return utils.Error("directly_call_tool_name is required for directly_call_tool but empty")
		}

		mgr := loop.GetConfig().GetAiToolManager()
		if mgr == nil || !mgr.IsRecentlyUsedTool(toolName) {
			return utils.Errorf("tool '%s' is not in the recently-used cache; use require_tool instead", toolName)
		}

		hasParams := len(action.GetInvokeParams("directly_call_tool_params")) > 0 ||
			action.GetString("directly_call_tool_params") != ""
		if !hasParams {
			nextAction := action.GetInvokeParams("next_action")
			hasParams = len(nextAction.GetObject("directly_call_tool_params")) > 0
		}
		if !hasParams {
			return utils.Error("directly_call_tool_params is required for directly_call_tool but empty")
		}

		loop.Set("directly_call_tool_name", toolName)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		toolName := loop.Get("directly_call_tool_name")
		if toolName == "" {
			operator.Feedback(utils.Error("directly_call_tool requires tool_name"))
			return
		}

		invoker := loop.GetInvoker()
		ctx := invoker.GetConfig().GetContext()
		if t := loop.GetCurrentTask(); t != nil {
			ctx = t.GetContext()
		}

		emitStatus := func(string) {}
		if emitter := loop.GetEmitter(); emitter != nil && operator.GetTask() != nil {
			pr, pw := utils.NewPipe()
			emitter.EmitDefaultStreamEvent(directlyCallToolParamsNodeID, pr, operator.GetTask().GetId())
			defer pw.Close()
			emitStatus = func(msg string) {
				pw.WriteString(msg)
				pw.WriteString("\n")
			}
		}
		reportStatus := func(msg string) {
			invoker.AddToTimeline("[DIRECT_CALL_PARAMS]", msg)
			emitStatus(msg)
		}

		reportStatus(fmt.Sprintf("preparing directly_call_tool params for '%s'", toolName))
		raw, objParams := getDirectlyCallToolParamPayload(action)
		params, notes := normalizeDirectlyCallToolParams(raw, objParams)
		for _, note := range notes {
			reportStatus(note)
		}
		if len(params) == 0 {
			reportStatus("directly_call_tool params extraction failed")
			operator.Feedback(utils.Error("directly_call_tool: no valid params found"))
			operator.Continue()
			return
		}

		paramKeys := directlyCallParamKeys(params)
		reportStatus(fmt.Sprintf("normalized %d param fields: %s", len(paramKeys), strings.Join(paramKeys, ", ")))
		operator.Feedback(fmt.Sprintf("Prepared directly_call_tool params for '%s': %d fields [%s]", toolName, len(paramKeys), strings.Join(paramKeys, ", ")))

		// 2. inject reserved keys from directly_call_ prefixed fields
		if id := action.GetString("directly_call_identifier"); id != "" {
			params[aicommon.ReservedKeyIdentifier] = id
		}
		if ce := action.GetString("directly_call_expectations"); ce != "" {
			params[aicommon.ReservedKeyCallExpectations] = ce
		}
		reportStatus(fmt.Sprintf("calling cached tool '%s'", toolName))

		// 3. execute
		result, directly, callErr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)

		if callErr == nil && result != nil && result.Success {
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolName); lookupErr == nil {
				loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.SaveRecentToolCache()
				}
			}
		}

		handleToolCallResult(loop, ctx, invoker, toolName, result, directly, callErr, operator)
	},
}
