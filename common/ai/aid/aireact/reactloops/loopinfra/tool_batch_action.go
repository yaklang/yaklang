package loopinfra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	directlyCallToolBatchField   = "directly_call_tool_params_group"
	retiredRequireToolBatchField = "tool_require_calls"

	actionStateDirectToolBatch = "directly_call_tool_batch"
)

// These are the exact scalar and batch examples taught by the tool-call actions.
// The scalar examples are rendered in the legacy field descriptions and the
// batch examples in the array field descriptions inside the loop's semi-dynamic
// Schema prompt section. They are also kept together on LoopAction.OutputExamples
// for embedders that render per-action examples. tool_batch_action_test.go feeds
// the same bytes through the production parser, verifier, handler and real tool
// callbacks, so prompt drift cannot silently teach the model an unparseable or
// unexecutable wire format.
const directlyCallToolScalarOutputExampleJSON = `{
  "@action": "directly_call_tool",
  "identifier": "read_project_config",
  "human_readable_thought": "读取单个项目配置",
  "directly_call_tool_name": "read_file",
  "directly_call_tool_params": {"file": "/workspace/go.mod"},
  "directly_call_identifier": "read_go_mod",
  "directly_call_expectations": "~1s",
  "directly_call_reason": "读取模块定义"
}`

const directlyCallToolBatchOutputExampleJSON = `{
  "@action": "directly_call_tool",
  "identifier": "parallel_project_reads",
  "human_readable_thought": "并发读取两个独立文件",
  "directly_call_tool_params_group": [
    {
      "tool_name": "read_file",
      "params": {"file": "/workspace/go.mod"},
      "identifier": "read_go_mod",
      "expectations": "~1s",
      "reason": "读取模块定义"
    },
    {
      "tool_name": "read_file",
      "params": {"file": "/workspace/README.md"},
      "identifier": "read_readme",
      "expectations": "~1s",
      "reason": "读取项目说明"
    }
  ]
}`

const requireToolScalarOutputExampleJSON = `{
  "@action": "require_tool",
  "identifier": "search_auth_handlers",
  "human_readable_thought": "加载搜索工具参数定义",
  "require_tool_payload": "grep",
  "tool_call_reason": "搜索认证处理逻辑"
}`

const requireToolBatchOutputExampleJSON = `{
  "@action": "require_tool",
  "identifier": "load_project_tools",
  "human_readable_thought": "加载搜索与文件读取工具定义",
  "require_tool_payload": ["grep", "read_file"]
}`

const directlyCallToolScalarOutputExamples = `
### directly_call_tool 单次调用

默认使用标量字段 directly_call_tool_name 和 directly_call_tool_params。只有在工具已启用且参数能够按照工具 Schema 完整给出时才使用 directly_call_tool；缺少 Schema 时先用 require_tool 加载，参数不确定时获取缺少的信息后再执行。

下面的 JSON 是完整可解析格式（工具名和参数值应替换为当前可用工具的真实 Schema）：

` + directlyCallToolScalarOutputExampleJSON + `
`

const directlyCallToolBatchOutputExamples = `
### directly_call_tool 参数分组

这是可选的延迟优化。仅当 2-8 个已启用工具调用都低风险、彼此独立且每层完整 JSON 参数都已从真实 Schema 确定时，才使用 directly_call_tool_params_group。嵌套 wrapper、页面状态操作、长文本参数或任一参数不确定时改用单调用。不要为凑数量发明调用，不要放置有先后依赖的调用，不要同时输出旧的 directly_call_tool_name 字段。参数分组使用内联 JSON，不支持外置 AI-TAG。

下面的 JSON 是完整可解析格式（工具名和参数值应替换为当前已启用的真实工具，优先使用 CACHE_TOOL_CALL 中已展示 Schema 的工具）：

` + directlyCallToolBatchOutputExampleJSON + `
`

const requireToolScalarOutputExamples = `
### require_tool 加载工具定义

require_tool_payload 填写一个工具名或工具名数组。只加载完整 Schema 到 AI TOOL CACHE，不生成参数、不执行工具。加载后在当前任务立即按真实 Schema 构造参数，用 directly_call_tool 完成操作；独立调用可通过参数分组一次提交，有依赖的调用按顺序执行。已有 Schema 直接复用。

` + requireToolScalarOutputExampleJSON + `
`
const requireToolBatchOutputExamples = `
` + requireToolBatchOutputExampleJSON + `
`

const directlyCallToolOutputExamples = directlyCallToolScalarOutputExamples + directlyCallToolBatchOutputExamples

const requireToolOutputExamples = requireToolScalarOutputExamples + requireToolBatchOutputExamples

func directlyCallToolBatchSchemaOption() aitool.ToolOption {
	return aitool.WithStructArrayParam(
		directlyCallToolBatchField,
		[]aitool.PropertyOption{
			aitool.WithParam_Description("可选的延迟优化。仅当本轮已明确 2-8 个低风险、互不依赖、互不干扰且参数完整的直接调用时使用。必须与 directly_call_tool_name/directly_call_tool_params 二选一，严禁混用。每项必须包含已启用工具的准确名称和从真实 Schema 确定的完整内联 JSON 参数；嵌套 wrapper 的每层参数都必须完整。参数分组使用内联 JSON，不支持外置 AI-TAG；不要为凑数量发明调用。下面是经过 CI 校验且可执行的格式：\n" + directlyCallToolBatchOutputExampleJSON),
			aitool.WithParam_Raw("minItems", 2),
			aitool.WithParam_Raw("maxItems", aicommon.DefaultToolBatchMaxCalls),
		},
		[]aitool.PropertyOption{
			aitool.WithParam_Raw("additionalProperties", false),
		},
		aitool.WithStringParam("tool_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("已启用工具的准确名称；优先选择 CACHE_TOOL_CALL 中已展示 Params Schema 的工具。")),
		aitool.WithRawParam("params", map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		}, aitool.WithParam_Required(true), aitool.WithParam_Description("该项 工具调用的完整内联 JSON 参数。")),
		aitool.WithStringParam("identifier",
			aitool.WithParam_Description("可选。该项 调用的唯一 snake_case 目的标识。")),
		aitool.WithStringParam("expectations",
			aitool.WithParam_Description("可选。该项 调用的预计耗时和回退策略。")),
		aitool.WithStringParam("reason",
			aitool.WithParam_Description("可选。直接展示在该工具卡片上；用简短短语说明这个 child 具体做什么，同名工具的不同调用也要分别描述，不能照搬整批任务的理由。")),
	)
}

func toolBatchVerifierContext(loop *reactloops.ReActLoop) context.Context {
	if loop != nil {
		if task := loop.GetCurrentTask(); task != nil && task.GetContext() != nil {
			return task.GetContext()
		}
		if cfg := loop.GetConfig(); cfg != nil && cfg.GetContext() != nil {
			return cfg.GetContext()
		}
	}
	return context.Background()
}

// lookupCanonicalActionParam deliberately never reads Action's flattened
// compatibility cache. Nested array members share field names there, so doing
// so could combine tool_name from one item with params from another.
func lookupCanonicalActionParam(action *aicommon.Action, key string) (any, bool) {
	if action == nil {
		return nil, false
	}
	if value, ok := action.LookupCanonicalParam(key); ok {
		return value, true
	}
	nextRaw, ok := action.LookupCanonicalParam("next_action")
	if !ok || nextRaw == nil {
		return nil, false
	}
	switch next := nextRaw.(type) {
	case map[string]any:
		value, exists := next[key]
		return value, exists
	case aitool.InvokeParams:
		value, exists := next[key]
		return value, exists
	default:
		return nil, false
	}
}

func hasAnyCanonicalActionParam(action *aicommon.Action, keys ...string) bool {
	for _, key := range keys {
		if _, ok := lookupCanonicalActionParam(action, key); ok {
			return true
		}
	}
	return false
}

func parseCanonicalBatchItems(action *aicommon.Action, key string) ([]aitool.InvokeParams, bool, error) {
	raw, exists := lookupCanonicalActionParam(action, key)
	if !exists {
		return nil, false, nil
	}
	items, err := aicommon.DecodeStrictObjectArray(raw)
	if err != nil {
		return nil, true, utils.Wrapf(err, "%s must be an array of objects", key)
	}
	return items, true, nil
}

func toolBatchMaxCalls(loop *reactloops.ReActLoop) int {
	maxCalls := aicommon.DefaultToolBatchMaxCalls
	if loop != nil && loop.GetConfig() != nil {
		// A number of focused unit tests and embedders construct Config literals
		// without the optional KV store. Do not dereference its promoted methods.
		if concrete, ok := loop.GetConfig().(*aicommon.Config); !ok || concrete.KeyValueConfig != nil {
			maxCalls = loop.GetConfig().GetConfigInt(aicommon.ConfigKeyToolBatchMaxCalls, maxCalls)
		}
	}
	if maxCalls < 2 {
		return 2
	}
	if maxCalls > aicommon.DefaultToolBatchMaxCalls {
		return aicommon.DefaultToolBatchMaxCalls
	}
	return maxCalls
}

func validateBatchLength(loop *reactloops.ReActLoop, field string, items []aitool.InvokeParams) error {
	if len(items) < 2 {
		return utils.Errorf("%s requires at least 2 independent calls; use directly_call_tool_name and directly_call_tool_params for one call", field)
	}
	if maxCalls := toolBatchMaxCalls(loop); len(items) > maxCalls {
		return utils.Errorf("%s contains %d calls, exceeding the configured maximum of %d", field, len(items), maxCalls)
	}
	return nil
}

func strictBatchString(item aitool.InvokeParams, field string, required bool) (string, error) {
	raw, exists := item[field]
	if !exists {
		if required {
			return "", utils.Errorf("%s is required", field)
		}
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", utils.Errorf("%s must be a string", field)
	}
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", utils.Errorf("%s must not be empty", field)
	}
	return value, nil
}

func rejectUnknownBatchFields(item aitool.InvokeParams, allowed map[string]struct{}) error {
	unknown := make([]string, 0)
	for key := range item {
		if _, ok := allowed[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return utils.Errorf("unknown fields: %s", strings.Join(unknown, ", "))
}

func strictBatchParams(raw any) (aitool.InvokeParams, error) {
	items, err := aicommon.DecodeStrictObjectArray([]any{raw})
	if err != nil {
		return nil, utils.Wrap(err, "params must be a non-null JSON object")
	}
	if len(items) != 1 {
		return nil, utils.Error("params must be a non-null JSON object")
	}
	return deepCloneInvokeParams(items[0])
}

func deepCloneInvokeParams(params aitool.InvokeParams) (aitool.InvokeParams, error) {
	// Action payloads are JSON data. A marshal round-trip is intentional here:
	// ValidateParams applies defaults and some tools mutate nested parameters;
	// no child may retain aliases into Action's canonical parse tree.
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, utils.Wrap(err, "marshal tool params")
	}
	cloned := make(aitool.InvokeParams)
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, utils.Wrap(err, "unmarshal tool params")
	}
	return cloned, nil
}

func parseDirectToolBatchAction(loop *reactloops.ReActLoop, action *aicommon.Action) (*aicommon.ToolCallGroupRequest, bool, error) {
	return parseDirectToolBatchActionWithMetadata(loop, action, false)
}

func parseDirectToolBatchActionWithMetadata(loop *reactloops.ReActLoop, action *aicommon.Action, allowMetadata bool) (*aicommon.ToolCallGroupRequest, bool, error) {
	if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
		return nil, false, utils.Wrap(err, "directly_call_tool action parse failed")
	}

	if hasAnyCanonicalActionParam(action, "directly_call_tool_calls", retiredRequireToolBatchField) {
		return nil, false, utils.Error("reason: retired batch fields; retry: use require_tool_payload for schema loading or directly_call_tool_params_group for explicit calls")
	}
	items, hasBatch, err := parseCanonicalBatchItems(action, directlyCallToolBatchField)
	if err != nil || !hasBatch {
		return nil, hasBatch, err
	}
	if hasAnyCanonicalActionParam(action, "directly_call_tool_name", "directly_call_tool_params") ||
		(!allowMetadata && hasAnyCanonicalActionParam(action, "directly_call_identifier", "directly_call_expectations", "directly_call_reason")) {
		return nil, true, utils.Errorf("%s cannot be combined with legacy directly_call_tool_* fields", directlyCallToolBatchField)
	}
	if hasAnyCanonicalActionParam(action,
		retiredRequireToolBatchField,
		"require_tool_payload", "tool_require_payload",
		"tool_call_reason",
	) {
		return nil, true, utils.Errorf("%s cannot be combined with require_tool fields", directlyCallToolBatchField)
	}
	if err := validateBatchLength(loop, directlyCallToolBatchField, items); err != nil {
		return nil, true, err
	}

	mgr := loop.GetConfig().GetAiToolManager()
	if mgr == nil {
		return nil, true, utils.Error("tool manager is unavailable")
	}
	allowed := map[string]struct{}{
		"tool_name": {}, "params": {}, "identifier": {}, "expectations": {}, "reason": {},
	}
	identifiers := make(map[string]int)
	request := &aicommon.ToolCallGroupRequest{Calls: make([]aicommon.ToolCallGroupCall, 0, len(items))}
	for index, item := range items {
		if err := rejectUnknownBatchFields(item, allowed); err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index)
		}
		toolName, err := strictBatchString(item, "tool_name", true)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index)
		}
		identifier, err := strictBatchString(item, "identifier", false)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index)
		}
		if identifier != "" {
			if first, duplicate := identifiers[identifier]; duplicate {
				return nil, true, utils.Errorf("%s[%d].identifier duplicates %s[%d].identifier %q", directlyCallToolBatchField, index, directlyCallToolBatchField, first, identifier)
			}
			identifiers[identifier] = index
		}
		expectations, err := strictBatchString(item, "expectations", false)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index)
		}
		reason, err := strictBatchString(item, "reason", false)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index)
		}

		rawParams, exists := item["params"]
		if !exists {
			return nil, true, directToolParameterError(loop, toolName, utils.Errorf("%s[%d].params is required (use {} for a parameterless tool)", directlyCallToolBatchField, index))
		}
		params, err := strictBatchParams(rawParams)
		if err != nil {
			return nil, true, directToolParameterError(loop, toolName, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index))
		}

		tool, err := mgr.GetToolByName(toolName)
		if err != nil {
			return nil, true, directToolParameterError(loop, toolName, utils.Wrapf(err, "%s[%d].tool_name %q is unavailable", directlyCallToolBatchField, index, toolName))
		}
		valid, validationErrors := tool.ValidateParams(params)
		if !valid {
			return nil, true, directToolParameterError(loop, toolName, utils.Errorf("%s[%d].params are invalid for %q: %s", directlyCallToolBatchField, index, toolName, strings.Join(validationErrors, "; ")))
		}

		if !mgr.IsRecentlyUsedTool(toolName) && loop.GetEmitter() != nil {
			loop.GetEmitter().EmitWarning("tool '%s' in %s[%d] is not in the recently-used cache; runtime will resolve it", toolName, directlyCallToolBatchField, index)
		}
		reactloops.MaybeWarnBashBeforeEdit(loop, toolName)
		request.Calls = append(request.Calls, aicommon.ToolCallGroupCall{
			Index:        index,
			ToolName:     toolName,
			Params:       params,
			Identifier:   identifier,
			Expectations: expectations,
			Reason:       reason,
		})
	}
	if allowMetadata {
		if raw, exists := lookupCanonicalActionParam(action, "directly_call_reason"); exists {
			if _, err := strictBatchString(aitool.InvokeParams{"directly_call_reason": raw}, "directly_call_reason", false); err != nil {
				return nil, true, err
			}
		}
		// Accept the old top-level field for compatibility, but never turn a
		// batch-wide reason into each child's visible reason. A missing child
		// reason is generated with that child's identifier as context.
	}
	return request, true, nil
}

func executeVerifiedToolBatch(
	loop *reactloops.ReActLoop,
	action *aicommon.Action,
	stateKey string,
	operator *reactloops.LoopActionHandlerOperator,
) bool {
	raw := loop.GetActionExecutionValue(action, stateKey)
	request, ok := raw.(*aicommon.ToolCallGroupRequest)
	if !ok || request == nil || len(request.Calls) == 0 {
		return false
	}

	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	task := loop.GetCurrentTask()
	if task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}

	toolNames := make([]string, 0, len(request.Calls))
	for _, call := range request.Calls {
		toolNames = append(toolNames, call.ToolName)
	}
	batchRuntime, supported := invoker.(aicommon.ToolCallGroupInvokeRuntime)
	var (
		result *aicommon.ToolCallGroupResult
		err    error
	)
	if supported {
		emitToolsPreparingStatus(loop, toolNames)
		emitToolBatchRunningStatus(loop, toolNames)
		result, err = batchRuntime.ExecuteToolCallGroup(ctx, task, request)
	} else {
		emitToolCallGroupResultStatus(loop, request, nil)
		for _, call := range request.Calls {
			operator.Feedback(directToolRetryFeedback(loop, call.ToolName, "runtime does not support explicit parameter groups; submit one directly_call_tool at a time", false))
		}
		operator.Continue()
		return true
	}
	handleToolBatchActionResult(loop, ctx, invoker, request, result, err, operator)
	return true
}

func handleToolBatchActionResult(
	loop *reactloops.ReActLoop,
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	request *aicommon.ToolCallGroupRequest,
	result *aicommon.ToolCallGroupResult,
	err error,
	operator *reactloops.LoopActionHandlerOperator,
) {
	if err != nil {
		var outcomes []aicommon.ToolCallOutcome
		if result != nil {
			outcomes = result.Outcomes
		}
		emitToolCallGroupResultStatus(loop, request, outcomes)
		msg := fmt.Sprintf("tool batch execution failed before completion: %v", err)
		invoker.AddToTimeline("[TOOL_BATCH_ERROR]", msg)
		operator.Feedback(msg)
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			for _, call := range request.Calls {
				operator.Feedback(directToolRetryFeedback(loop, call.ToolName, msg, true))
			}
		}
		operator.Continue()
		return
	}
	if result == nil {
		emitToolCallGroupResultStatus(loop, request, nil)
		for _, call := range request.Calls {
			operator.Feedback(directToolRetryFeedback(loop, call.ToolName, "tool batch returned no result", true))
		}
		operator.Continue()
		return
	}
	if result.DirectlyAnswer {
		loop.StopSubAgentsForUser()
		answer, answerErr := invoker.DirectlyAnswer(ctx,
			"在并发工具调用审批中，用户中断了该批次并要求直接回答。不要继续执行该批次中的其他工具。", nil)
		if answerErr != nil {
			operator.Fail(utils.Wrap(answerErr, "DirectlyAnswer after tool batch"))
			return
		}
		invoker.AddToTimeline("directly-answer", answer)
		operator.ExitForUser()
		return
	}

	outcomes := append([]aicommon.ToolCallOutcome(nil), result.Outcomes...)
	sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].Index < outcomes[j].Index })
	var executionSucceeded, executionFailed, executionUnknown, protocolFailed, notRun int
	for i := range outcomes {
		outcome := &outcomes[i]
		if outcome.Result == nil {
			notRun++
			continue
		}
		if !outcome.Result.Success || outcome.Stage == aicommon.ToolCallStageInvokeFailed {
			protocolFailed++
			continue
		}
		if outcome.ExecutionStatus == "" {
			outcome.ExecutionStatus, _ = outcome.Result.GetExecutionStatus()
		}
		switch outcome.ExecutionStatus {
		case aitool.ToolExecutionStatusSucceeded:
			executionSucceeded++
		case aitool.ToolExecutionStatusFailed:
			executionFailed++
		default:
			executionUnknown++
		}
	}
	lines := []string{fmt.Sprintf(
		"Tool batch settled: %d calls; execution_succeeded=%d; execution_failed=%d; execution_unknown=%d; protocol_failed=%d; not_run=%d",
		len(request.Calls), executionSucceeded, executionFailed, executionUnknown, protocolFailed, notRun,
	)}
	executedToolCallCount := 0
	for _, outcome := range outcomes {
		// Result is assigned only after the ToolCaller returns from the plugin
		// callback. It is therefore the objective execution boundary: success and
		// tool-level failure both count, while admission/review/cancel outcomes have
		// nil Result and do not.
		if outcome.Result != nil {
			executedToolCallCount++
		}
		toolName := outcome.FinalTool
		if toolName == "" {
			toolName = outcome.RequestedTool
		}
		if toolName == "" && outcome.Index >= 0 && outcome.Index < len(request.Calls) {
			toolName = request.Calls[outcome.Index].ToolName
		}
		status := string(outcome.Stage)
		if status == "" {
			status = "unknown"
		} else if outcome.Stage == aicommon.ToolCallStageDone {
			executionStatus := outcome.ExecutionStatus
			detail := ""
			if outcome.Result != nil {
				if executionStatus == "" {
					executionStatus, detail = outcome.Result.GetExecutionStatus()
				} else {
					_, detail = outcome.Result.GetExecutionStatus()
				}
			}
			switch executionStatus {
			case aitool.ToolExecutionStatusSucceeded:
				status = "protocol-completed; execution-succeeded"
			case aitool.ToolExecutionStatusFailed:
				status = "protocol-completed; execution-failed"
			default:
				status = "protocol-completed; execution-outcome-unknown; inspect execution_result"
			}
			if detail != "" {
				status += " (" + detail + ")"
			}
		} else if outcome.Stage == aicommon.ToolCallStageInvokeFailed {
			status = "protocol-error"
		}
		if outcome.Err != nil {
			status += ": " + outcome.Err.Error()
		} else if outcome.Result != nil && outcome.Result.Error != "" {
			status += ": " + outcome.Result.Error
		}
		lines = append(lines, fmt.Sprintf("%d. %s: %s", outcome.Index+1, toolName, status))
		var reconsider *aicommon.ToolReviewReconsiderError
		if errors.As(outcome.Err, &reconsider) {
			lines = append(lines, reconsider.Feedback)
		} else if (outcome.Stage == aicommon.ToolCallStageInvokeFailed || outcome.Stage == aicommon.ToolCallStagePrepareFailed || outcome.Stage == aicommon.ToolCallStageValidationFailed || outcome.ExecutionStatus == aitool.ToolExecutionStatusFailed) && !errors.Is(outcome.Err, context.Canceled) && !errors.Is(outcome.Err, context.DeadlineExceeded) {
			lines = append(lines, directToolRetryFeedback(loop, toolName, status, outcome.Result != nil))
		}

		if outcome.Result != nil && outcome.Result.Success {
			reactloops.MarkEditBeforeExecutionCompleted(loop, toolName)
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolName); lookupErr == nil {
				loop.RecordRecentlyUsedTool(cachedTool)
			}
		}
	}
	summary := strings.Join(lines, "\n")
	invoker.AddToTimeline("[TOOL_BATCH_RESULT]", summary)
	operator.Feedback(summary)
	emitToolCallGroupResultStatus(loop, request, outcomes)
	toolNames := make([]string, 0, len(request.Calls))
	for _, call := range request.Calls {
		toolNames = append(toolNames, call.ToolName)
	}
	justExecutedTool := executedToolCallCount > 0
	if justExecutedTool {
		operator.MarkToolExecuted(executedToolCallCount)
	}

	task := loop.GetCurrentTask()
	// Satisfaction is meaningful only after at least one callback actually
	// settled. A syntactically valid batch that was wholly rejected at admission
	// must remain visible in history/feedback, but must not pretend work happened.
	if task == nil || !justExecutedTool {
		operator.Continue()
		return
	}
	verifyResult, triggered, verifyErr := loop.MaybeVerifyUserSatisfaction(ctx, task.GetUserInput(), true, strings.Join(toolNames, ","))
	if verifyErr != nil {
		operator.Fail(verifyErr)
		return
	}
	if triggered && verifyResult != nil && !verifyResult.Satisfied {
		operator.Feedback(fmt.Sprintf("[Verification] Task not yet satisfied.\nReasoning: %s", verifyResult.Reasoning))
	}
	operator.Continue()
}
