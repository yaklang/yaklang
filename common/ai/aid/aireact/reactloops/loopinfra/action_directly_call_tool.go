package loopinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/go-funk"
	"io"
	"sort"
	"strings"
	"time"

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

func formatDirectlyCallToolParamsTimeline(toolName string, params aitool.InvokeParams, blockParamNames []string) string {
	blockSet := make(map[string]struct{}, len(blockParamNames))
	for _, name := range blockParamNames {
		blockSet[name] = struct{}{}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ToolName %s Parameter:", toolName))
	for _, key := range directlyCallParamKeys(params) {
		if key == aicommon.ReservedKeyIdentifier || key == aicommon.ReservedKeyCallExpectations || funk.IsEmpty(params[key]) {
			continue
		}
		displayKey := key
		if _, ok := blockSet[key]; ok {
			displayKey = key + "(BLOCK)"
		}
		value := strings.TrimRight(utils.InterfaceToString(params[key]), "\r\n")
		sb.WriteString("\n")
		if strings.Contains(value, "\n") {
			sb.WriteString(fmt.Sprintf("[%s]:", displayKey))
			sb.WriteString("\n")
			sb.WriteString(value)
		} else {
			sb.WriteString(fmt.Sprintf("%s: %s", displayKey, value))
		}
	}
	return sb.String()
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
	// A tool may legitimately define a business parameter named "params"
	// (browser.capability.call does). It is a wrapper only when accompanied by
	// protocol metadata such as tool/@action/type; otherwise keep the complete
	// object so sibling fields are not discarded.
	if len(params.GetObject("tool")) > 0 || len(params.GetObject("next_action")) > 0 {
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
	Description: "直接调用已启用且参数完整的工具，跳过申请和参数生成阶段。先枚举本轮已明确的真实调用：" +
		"存在 2-8 个互不依赖、互不干扰且参数完整的调用时，优先使用 directly_call_tool_calls 一次并发提交，不要拆成多个单工具轮次；" +
		"只有本轮恰好一个调用时才同时填写 directly_call_tool_name 和 directly_call_tool_params。" +
		"优先使用 CACHE_TOOL_CALL 中已展示参数 Schema 的工具；已启用但未缓存的工具仍可解析并产生告警。" +
		"参数不确定时改用 require_tool；严禁混用单调用和批量字段，也不要为了凑数量发明调用。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"directly_call_tool_name",
			aitool.WithParam_Description("仅当本轮恰好一个直接调用时填写；存在 directly_call_tool_calls 时必须省略。填写一个已启用工具的准确名称。优先选择 CACHE_TOOL_CALL 中已展示参数 Schema 的工具；未缓存但已启用的工具仍可解析并产生告警。下面是经过 CI 校验且可执行的单调用格式：\n"+directlyCallToolScalarOutputExampleJSON),
		),
		aitool.WithRawParam("directly_call_tool_params", map[string]any{
			"type": []string{"object", "string"},
		}, aitool.WithParam_Description(`仅当本轮恰好一个直接调用时填写；存在 directly_call_tool_calls 时必须省略。优先使用 JSON object 提供该工具的完整参数；为兼容旧协议仍接受包含 JSON object 的字符串。参数结构必须符合 CACHE_TOOL_CALL 中该工具的 Params Schema。`)),
		aitool.WithStringParam(
			"directly_call_identifier",
			aitool.WithParam_Description(`可选。描述调用目的的简短 snake_case 标识，例如 "scan_port_443"、"query_large_file"；用于报告文件命名。`),
		),
		aitool.WithStringParam(
			"directly_call_expectations",
			aitool.WithParam_Description(`可选。预计耗时和回退策略，例如 "~3s，超过10s则停止"；用于执行期间的 interval review。`),
		),
		aitool.WithStringParam(
			"directly_call_reason",
			aitool.WithParam_Description(`可选。用简短短语说明这次调用具体做什么，例如“测试 /api/user 的 id 参数是否存在 IDOR”或“用 union 注入探测字段数”。不要写前序总结或过渡语；仅当 human_readable_thought 已说明原因时省略。该内容会显示在工具调用卡片上。`),
		),
		directlyCallToolBatchSchemaOption(),
	},
	OutputExamples: directlyCallToolOutputExamples,
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		loop.Delete(loopVarDirectToolBatch)

		// Keep the established scalar streaming contract. The legacy discriminator
		// is readable before the root JSON object closes, so a valid one-call action
		// must not wait for the optional batch array's canonical representation.
		// If a malformed response emits both forms, scalar wins for backward
		// compatibility; the published Schema and prompt still forbid mixing them.
		toolName := action.GetString("directly_call_tool_name")
		if toolName == "" {
			toolName = action.GetInvokeParams("next_action").GetString("directly_call_tool_name")
		}
		if toolName != "" {
			mgr := loop.GetConfig().GetAiToolManager()
			if mgr == nil || !mgr.IsRecentlyUsedTool(toolName) {
				// 工具不在 recently-used cache 中时只记录警告，不报错触发重试。
				// 后续 ActionHandler 会通过 GetToolByName 自行决定：工具存在则继续
				// 直接调用，工具不存在则走已有的 fallback 到 require_tool 路径。
				emit := loop.GetEmitter()
				if emit != nil {
					emit.EmitWarning("tool '%s' is not in the recently-used cache; handler will resolve it", toolName)
				}
				loop.GetInvoker().AddToTimeline(
					"directly_call_cache_miss",
					fmt.Sprintf(
						"[DIRECT_CALL_CACHE_MISS] directly_call_tool selected '%s' but it is not in the recently-used cache. "+
							"Letting handler resolve (call if tool exists, otherwise fall back to require_tool).",
						toolName,
					),
				)
			}
			reactloops.MaybeWarnBashBeforeEdit(loop, toolName)
			loop.Set("directly_call_tool_name", toolName)
			return nil
		}

		batch, hasBatch, batchErr := parseDirectToolBatchAction(loop, action)
		if batchErr != nil {
			return batchErr
		}
		if hasBatch {
			loop.Set(loopVarDirectToolBatch, batch)
			loop.Delete("directly_call_tool_name")
			return nil
		}

		return utils.Error("directly_call_tool requires directly_call_tool_name or directly_call_tool_calls")
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		if executeVerifiedToolBatch(loop, loopVarDirectToolBatch, operator) {
			return
		}
		invoker := loop.GetInvoker()
		cacheSuccessfulTool := func(name string, result *aitool.ToolResult, callErr error) {
			if callErr != nil || result == nil || !result.Success {
				return
			}
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(name); lookupErr == nil {
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.RecordRecentlyUsedTool(cachedTool)
				} else {
					loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				}
			}
		}
		reportStatus := func(msg string) {
			invoker.AddToTimeline("DIRECT_CALL_PARAMS", msg)
		}

		toolName := loop.Get("directly_call_tool_name")
		if toolName == "" {
			loopInfraStatus(loop, "没有找到要使用的工具", "No suitable tool was found")
			reportStatus(strings.TrimSpace(`
Error: directly_call_tool_name is missing in loop state.
Fast-path directly_call_tool failed before execution and cannot be recovered in-place because the target tool is unknown.
Next attempt MUST either switch to require_tool or retry directly_call_tool with both directly_call_tool_name and directly_call_tool_params.

Few-shot example 1 (fallback to require_tool):
{"@action":"require_tool","tool_require_payload":"<tool_name>"}

Few-shot example 2 (valid directly_call_tool):
{"@action":"directly_call_tool","directly_call_tool_name":"<tool_name>","directly_call_identifier":"<snake_case_intent>","directly_call_expectations":"~3s, fallback to require_tool if params are uncertain","directly_call_reason":"<why this call>","directly_call_tool_params":{"<param>":"<value>"}}
`))
			operator.Feedback(utils.Error("directly_call_tool requires tool_name; switch to require_tool or provide directly_call_tool_name + directly_call_tool_params"))
			return
		}
		emitToolsPreparingStatus(loop, []string{toolName})

		// Pre-card unrecoverable branch: cached tool lookup failure cannot enter the
		// "card already created" flow, so it stays here (before invoker.DirectlyCallTool).
		_, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolName)
		if lookupErr != nil {
			reportStatus(fmt.Sprintf("cached tool lookup failed for '%s': %v", toolName, lookupErr))
			loopInfraStatus(loop, "当前工具暂时不可用，正在换一种方式继续", "The current tool is unavailable; trying another approach")
			msg := fmt.Sprintf("directly_call_tool cached tool lookup failed for '%s'; switch to @action=require_tool", toolName)
			operator.Feedback(utils.Error(msg))
			invoker.AddToTimeline("DIRECT_CALL_PARAMS", msg)
			operator.Continue()
			return
		}

		ctx := invoker.GetConfig().GetContext()
		if t := loop.GetCurrentTask(); t != nil {
			ctx = t.GetContext()
		}

		resolveTool := func(name string) (*aitool.Tool, error) {
			config := loop.GetConfig()

			if buildinaitools.IsMCPToolName(name) && !aicommon.IsMCPServersAllowedConfig(config) {
				return nil, utils.Errorf("MCP tools are disabled for this runtime")
			}

			tool, err := config.GetAiToolManager().GetToolByName(name)
			if err != nil {
				return nil, utils.Errorf("tool '%s' not found: %v", name, err)
			}

			// For MCP tools, wait until the background loader replaces the DB stub with a live
			// tool (or timeout). This avoids TOOL_INITIALIZING failures right after engine start.
			if buildinaitools.IsMCPToolName(name) && buildinaitools.IsMCPPendingStub(tool) {
				tool, err = buildinaitools.WaitForMCPLiveTool(
					ctx, config.GetAiToolManager(), name,
					buildinaitools.MCPToolInitWaitTimeout,
					buildinaitools.MCPToolInitPollInterval,
					func(elapsed time.Duration) {
						loop.GetEmitter().EmitInfo("still waiting for MCP tool %q (elapsed %v)...", name, elapsed.Round(time.Second))
					},
				)
				if err != nil {
					return nil, err
				}
			}
			return tool, nil
		}

		// prepare is the loop-layer callback run AFTER the tool-call card has been
		// created (loading). It reads the streaming action's params (blocking until
		// they arrive), normalizes/merges/validates them, streams progress, and either
		// returns finalized params or signals fallbackToRequire (reusing the same card
		// and switching to the AI param-generation path).
		prepare := func(action *aicommon.Action, name string) (aitool.InvokeParams, bool, *aitool.Tool, error) {
			emitProgress := func(string) {}
			finishProgress := func(string) {}
			if emitter := loop.GetEmitter(); emitter != nil && operator.GetTask() != nil {
				pr, pw := utils.NewPipe()
				event, _ := emitter.EmitDefaultSystemStreamEvent(directlyCallToolParamsNodeID, pr, operator.GetTask().GetId())
				if event != nil {
					progressEventID := event.GetStreamEventWriterId()
					aicommon.EmitAIRequestAndResponseReferenceMaterials(
						emitter,
						progressEventID,
						loop.Get(directlyCallToolPromptLoopKey),
						loop.Get(directlyCallToolResponseLoopKey),
					)
				}
				defer pw.Close()
				emitProgress = func(msg string) {
					_, _ = pw.WriteString(msg)
					_, _ = pw.WriteString(" -> ")
				}
				finishProgress = func(msg string) {
					_, _ = pw.WriteString(msg)
					_, _ = pw.WriteString("\n")
				}
			}

			emitProgress("[解析缓存工具]")
			tool, err := resolveTool(name)
			if err != nil {
				finishProgress("[failed] cached tool resolution failed; falling back to require_tool")
				return nil, false, nil, err
			}

			emitProgress("[开始处理参数]")
			raw, objParams := getDirectlyCallToolParamPayload(action)
			params, _ := normalizeDirectlyCallToolParams(raw, objParams)
			if params == nil {
				params = make(aitool.InvokeParams)
			}
			mergedBlockParams := aicommon.MergeActionAITagParams(action, params, getDirectlyCallToolParamNames(loop, toolName))

			valid, validationErrors := tool.ValidateParams(params)
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
{"@action":"directly_call_tool","directly_call_tool_name":"%s","directly_call_identifier":"<snake_case_intent>","directly_call_expectations":"~3s, fallback to require_tool if params are uncertain","directly_call_reason":"<why this call>","directly_call_tool_params":{"<param>":"<value>"}}
`, toolName, validationSummary, toolName, toolName, toolName)))
				finishProgress("[failed] params validation failed; falling back to require_tool")
				reportStatus(fmt.Sprintf("auto fallback: switching '%s' from directly_call_tool to @action=require_tool because schema validation failed", toolName))
				reactloops.EmitStatusI18n(
					loop,
					"当前方式不太合适，正在调整调用方式",
					"Adjusting the tool invocation approach",
					aicommon.WithStatusCode("tool.adjusting"),
					aicommon.WithStatusState(aicommon.StatusStateRecovering),
				)
				operator.Feedback(fmt.Sprintf("directly_call_tool params invalid for '%s': %s; automatically switching to @action=require_tool", toolName, validationSummary))
				return nil, true, tool, nil
			}

			feedbackItems := buildDirectlyCallParamFeedbackItems(params, mergedBlockParams)
			operator.Feedback(fmt.Sprintf("Prepared directly_call_tool params for '%s': %d fields [%s]", toolName, len(feedbackItems), strings.Join(feedbackItems, ", ")))
			reportStatus(formatDirectlyCallToolParamsTimeline(toolName, params, mergedBlockParams))
			emitDirectlyCallParamProgress(emitProgress, params, mergedBlockParams)
			if ce := action.GetString("directly_call_expectations"); strings.TrimSpace(ce) != "" {
				emitProgress("[note] " + ce)
			}
			finishProgress("[done]")

			// inject reserved keys from directly_call_ prefixed fields
			if id := action.GetString("directly_call_identifier"); id != "" {
				params[aicommon.ReservedKeyIdentifier] = id
			}
			if ce := action.GetString("directly_call_expectations"); ce != "" {
				params[aicommon.ReservedKeyCallExpectations] = ce
			}
			tools := buildStatusTools(loop, []string{name}, aicommon.StatusStateRunning)
			reactloops.EmitStatusI18n(
				loop,
				fmt.Sprintf("正在使用「%s」完成这一步", statusToolNames(tools, false)),
				fmt.Sprintf("Using %s for this step", statusToolNames(tools, true)),
				aicommon.WithStatusCode("tool.running"),
				aicommon.WithStatusTools(tools...),
			)
			return params, false, tool, nil
		}

		// DirectlyCallTool emits the card (loading) first, then runs prepare (reads
		// streaming params), then invokes — reusing the same card on fallback.
		result, directly, callErr := invoker.DirectlyCallTool(ctx, toolName, action, prepare)
		cacheSuccessfulTool(toolName, result, callErr)

		handleToolCallResult(loop, ctx, invoker, toolName, result, directly, callErr, operator)
	},
}
