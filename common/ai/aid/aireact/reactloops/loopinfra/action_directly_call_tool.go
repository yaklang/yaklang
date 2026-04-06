package loopinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const directlyCallToolParamsNodeID = "directly_call_tool_params"
const directlyCallToolPromptLoopKey = "last_ai_decision_prompt"
const directlyCallToolResponseLoopKey = "last_ai_decision_response"
const directlyCallToolNonceLoopKey = "last_ai_decision_nonce"

func getDirectlyCallToolParamNames(loop *reactloops.ReActLoop, toolName string) []string {
	if loop == nil || loop.GetConfig() == nil || loop.GetConfig().GetAiToolManager() == nil {
		return nil
	}
	paramNames := loop.GetConfig().GetAiToolManager().GetRecentToolParamNamesByTool(toolName)
	if len(paramNames) > 0 {
		return paramNames
	}
	return loop.GetConfig().GetAiToolManager().GetRecentToolParamNames()
}

func buildDirectlyCallParamFeedbackItems(params aitool.InvokeParams, blockParamNames []string) []string {
	blockSet := make(map[string]struct{}, len(blockParamNames))
	for _, name := range blockParamNames {
		blockSet[name] = struct{}{}
	}

	items := make([]string, 0, len(params))
	for _, key := range directlyCallParamKeys(params) {
		if key == aicommon.ReservedKeyIdentifier || key == aicommon.ReservedKeyCallExpectations {
			continue
		}
		if _, ok := blockSet[key]; ok {
			items = append(items, fmt.Sprintf("%s(BLOCK)", key))
			continue
		}
		items = append(items, key)
	}
	return items
}

func emitDirectlyCallParamProgress(emit func(string), params aitool.InvokeParams, blockParamNames []string) {
	blockSet := make(map[string]struct{}, len(blockParamNames))
	for _, name := range blockParamNames {
		blockSet[name] = struct{}{}
	}

	for _, key := range directlyCallParamKeys(params) {
		if key == aicommon.ReservedKeyIdentifier || key == aicommon.ReservedKeyCallExpectations {
			continue
		}
		if _, ok := blockSet[key]; ok {
			emit(fmt.Sprintf("%s(BLOCK): %s", key, utils.InterfaceToString(params[key])))
			continue
		}
		emit(fmt.Sprintf("%s: %s", key, utils.ShrinkString(strings.ReplaceAll(utils.InterfaceToString(params[key]), "\n", `\\n`), 80)))
	}
}

func streamDirectlyCallParamProgressFromRawResponse(ctx context.Context, rawResponse, nonce string, paramNames []string, writer io.Writer) error {
	if strings.TrimSpace(rawResponse) == "" || writer == nil {
		return nil
	}

	streamFieldNames := make([]string, 0, len(paramNames)*2+1)
	var actionOpts []aicommon.ActionMakerOption
	if nonce != "" {
		actionOpts = append(actionOpts, aicommon.WithActionNonce(nonce))
	}
	for _, paramName := range paramNames {
		streamFieldNames = append(streamFieldNames, paramName)
		if nonce == "" {
			continue
		}
		tagKey := fmt.Sprintf("__aitag__%s", paramName)
		streamFieldNames = append(streamFieldNames, tagKey)
		actionOpts = append(actionOpts, aicommon.WithActionTagToKey(fmt.Sprintf("TOOL_PARAM_%s", paramName), tagKey))
	}
	streamFieldNames = append(streamFieldNames, "directly_call_expectations")

	actionOpts = append(actionOpts,
		aicommon.WithActionFieldStreamHandler(streamFieldNames, func(key string, r io.Reader) {
			if strings.HasPrefix(key, "__aitag__") {
				_, _ = io.WriteString(writer, strings.TrimPrefix(key, "__aitag__")+"(BLOCK): ")
			} else if key == "directly_call_expectations" {
				_, _ = io.WriteString(writer, "[note] ")
			} else {
				_, _ = io.WriteString(writer, key+": ")
			}
			_, _ = io.Copy(writer, r)
			_, _ = io.WriteString(writer, " -> ")
		}),
	)

	_, err := aicommon.ExtractValidActionFromStream(ctx, strings.NewReader(rawResponse), "object", actionOpts...)
	return err
}

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
		reactloops.MaybeWarnBashBeforeEdit(loop, toolName)

		loop.Set("directly_call_tool_name", toolName)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		cacheSuccessfulTool := func(name string, result *aitool.ToolResult, callErr error) {
			if callErr != nil || result == nil || !result.Success {
				return
			}
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(name); lookupErr == nil {
				loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.SaveRecentToolCache()
				}
			}
		}
		emitProgress := func(string) {}
		finishProgress := func(string) {}
		var progressWriter io.Writer
		var progressEventID string
		if emitter := loop.GetEmitter(); emitter != nil && operator.GetTask() != nil {
			pr, pw := utils.NewPipe()
			progressWriter = pw
			event, _ := emitter.EmitDefaultStreamEvent(directlyCallToolParamsNodeID, pr, operator.GetTask().GetId())
			if event != nil {
				progressEventID = event.GetStreamEventWriterId()
				aicommon.EmitAIRequestAndResponseReferenceMaterials(
					emitter,
					progressEventID,
					loop.Get(directlyCallToolPromptLoopKey),
					loop.Get(directlyCallToolResponseLoopKey),
				)
			}
			defer pw.Close()
			emitProgress = func(msg string) {
				pw.WriteString(msg)
				pw.WriteString(" -> ")
			}
			finishProgress = func(msg string) {
				pw.WriteString(msg)
				pw.WriteString("\n")
			}
		}

		reportStatus := func(msg string) {
			invoker.AddToTimeline("[DIRECT_CALL_PARAMS]", msg)
		}

		toolName := loop.Get("directly_call_tool_name")
		if toolName == "" {
			reportStatus(strings.TrimSpace(`
Error: directly_call_tool_name is missing in loop state.
Fast-path directly_call_tool failed before execution and cannot be recovered in-place because the target tool is unknown.
Next attempt MUST either switch to require_tool or retry directly_call_tool with both directly_call_tool_name and directly_call_tool_params.

Few-shot example 1 (fallback to require_tool):
{"@action":"require_tool","tool_require_payload":"<tool_name>"}

Few-shot example 2 (valid directly_call_tool):
{"@action":"directly_call_tool","directly_call_tool_name":"<tool_name>","directly_call_identifier":"<snake_case_intent>","directly_call_expectations":"~3s, fallback to require_tool if params are uncertain","directly_call_tool_params":{"<param>":"<value>"}}
`))
			finishProgress("[failed] missing directly_call_tool_name; use require_tool or provide a complete directly_call_tool payload")
			operator.Feedback(utils.Error("directly_call_tool requires tool_name; switch to require_tool or provide directly_call_tool_name + directly_call_tool_params"))
			return
		}

		cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolName)
		if lookupErr != nil {
			reportStatus(fmt.Sprintf("cached tool lookup failed for '%s': %v", toolName, lookupErr))
			finishProgress(fmt.Sprintf("[failed] cached tool '%s' is unavailable; switch to @action=require_tool", toolName))
			msg := fmt.Sprintf("directly_call_tool cached tool lookup failed for '%s'; switch to @action=require_tool", toolName)
			operator.Feedback(utils.Error(msg))
			invoker.AddToTimeline("[DIRECT_CALL_PARAMS]", msg)
			operator.Continue()
			return
		}

		emitProgress(fmt.Sprintf("[tool:%v]", toolName))
		ctx := invoker.GetConfig().GetContext()
		if t := loop.GetCurrentTask(); t != nil {
			ctx = t.GetContext()
		}

		reportStatus(fmt.Sprintf("preparing directly_call_tool params for '%s'", toolName))
		emitProgress("[开始处理参数]")
		if progressWriter != nil {
			if err := streamDirectlyCallParamProgressFromRawResponse(ctx, loop.Get(directlyCallToolResponseLoopKey), loop.Get(directlyCallToolNonceLoopKey), getDirectlyCallToolParamNames(loop, toolName), progressWriter); err != nil {
				reportStatus(fmt.Sprintf("stream directly_call_tool params from raw response failed: %v", err))
			}
		}
		raw, objParams := getDirectlyCallToolParamPayload(action)
		params, notes := normalizeDirectlyCallToolParams(raw, objParams)
		if params == nil {
			params = make(aitool.InvokeParams)
		}
		mergedBlockParams := aicommon.MergeActionAITagParams(action, params, getDirectlyCallToolParamNames(loop, toolName))
		if len(mergedBlockParams) > 0 {
			notes = append(notes, fmt.Sprintf("merged %d AITAG block params: %s", len(mergedBlockParams), strings.Join(mergedBlockParams, ", ")))
		}
		for _, note := range notes {
			reportStatus(note)
		}

		valid, validationErrors := cachedTool.ValidateParams(params)
		if !valid {
			validationSummary := strings.Join(validationErrors, "; ")
			if validationSummary == "" {
				validationSummary = "required params do not match the tool schema"
			}
			reportStatus(strings.TrimSpace(fmt.Sprintf(`
directly_call_tool params validation failed for cached tool '%s'.
The fast path already selected a cached tool, but the generated params do not satisfy the tool schema.
Validation errors: %s
Next attempt should prefer @action=require_tool for '%s' so the runtime can re-enter normal parameter generation and review, or retry directly_call_tool with schema-matching params.

Few-shot example 1 (preferred fallback):
{"@action":"require_tool","tool_require_payload":"%s"}

Few-shot example 2 (valid direct retry):
{"@action":"directly_call_tool","directly_call_tool_name":"%s","directly_call_identifier":"<snake_case_intent>","directly_call_expectations":"~3s, fallback to require_tool if params are uncertain","directly_call_tool_params":{"<param>":"<value>"}}
`, toolName, validationSummary, toolName, toolName, toolName)))
			reportStatus(fmt.Sprintf("auto fallback: switching '%s' from directly_call_tool to @action=require_tool because schema validation failed", toolName))
			finishProgress(fmt.Sprintf("[fallback] params for '%s' failed schema validation; automatically switching to @action=require_tool", toolName))
			operator.Feedback(fmt.Sprintf("directly_call_tool params invalid for '%s': %s; automatically switching to @action=require_tool", toolName, validationSummary))

			result, directly, callErr := invoker.ExecuteToolRequiredAndCall(ctx, toolName)
			cacheSuccessfulTool(toolName, result, callErr)
			handleToolCallResult(loop, ctx, invoker, toolName, result, directly, callErr, operator)
			return
		}

		paramKeys := directlyCallParamKeys(params)
		feedbackItems := buildDirectlyCallParamFeedbackItems(params, mergedBlockParams)
		reportStatus(fmt.Sprintf("normalized %d param fields: %s", len(paramKeys), strings.Join(paramKeys, ", ")))
		operator.Feedback(fmt.Sprintf("Prepared directly_call_tool params for '%s': %d fields [%s]", toolName, len(feedbackItems), strings.Join(feedbackItems, ", ")))

		// 2. inject reserved keys from directly_call_ prefixed fields
		if id := action.GetString("directly_call_identifier"); id != "" {
			params[aicommon.ReservedKeyIdentifier] = id
		}
		if ce := action.GetString("directly_call_expectations"); ce != "" {
			params[aicommon.ReservedKeyCallExpectations] = ce
			emitProgress("[note] " + ce)
		}
		reportStatus(fmt.Sprintf("calling cached tool '%s'", toolName))
		finishProgress(fmt.Sprintf("调用缓存工具 '%s' [done]", toolName))

		// 3. execute
		result, directly, callErr := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
		cacheSuccessfulTool(toolName, result, callErr)

		handleToolCallResult(loop, ctx, invoker, toolName, result, directly, callErr, operator)
	},
}
