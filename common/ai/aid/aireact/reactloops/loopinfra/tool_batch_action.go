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
	directlyCallToolBatchField = "directly_call_tool_calls"
	requireToolBatchField      = "tool_require_calls"

	loopVarDirectToolBatch  = "directly_call_tool_batch"
	loopVarRequireToolBatch = "tool_require_batch"
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
  "directly_call_tool_params": {"path": "/workspace/go.mod"},
  "directly_call_identifier": "read_go_mod",
  "directly_call_expectations": "~1s",
  "directly_call_reason": "读取模块定义"
}`

const directlyCallToolBatchOutputExampleJSON = `{
  "@action": "directly_call_tool",
  "identifier": "parallel_project_reads",
  "human_readable_thought": "并发读取两个独立文件",
  "directly_call_tool_calls": [
    {
      "tool_name": "read_file",
      "params": {"path": "/workspace/go.mod"},
      "identifier": "read_go_mod",
      "expectations": "~1s",
      "reason": "读取模块定义"
    },
    {
      "tool_name": "read_file",
      "params": {"path": "/workspace/README.md"},
      "identifier": "read_readme",
      "expectations": "~1s",
      "reason": "读取项目说明"
    }
  ]
}`

const requireToolScalarOutputExampleJSON = `{
  "@action": "require_tool",
  "identifier": "search_auth_handlers",
  "human_readable_thought": "搜索认证处理函数",
  "tool_require_payload": "grep",
  "tool_call_reason": "搜索认证处理逻辑"
}`

const requireToolBatchOutputExampleJSON = `{
  "@action": "require_tool",
  "identifier": "parallel_project_search",
  "human_readable_thought": "并发准备两个独立搜索",
  "tool_require_calls": [
    {
      "tool_name": "grep",
      "identifier": "find_auth_handlers",
      "reason": "搜索认证处理逻辑"
    },
    {
      "tool_name": "read_file",
      "identifier": "read_project_config",
      "reason": "读取独立项目配置"
    }
  ]
}`

const directlyCallToolScalarOutputExamples = `
### directly_call_tool 单次调用

仅当本轮恰好一个已明确调用时，使用标量字段 directly_call_tool_name 和 directly_call_tool_params，不要为了一个调用创建 directly_call_tool_calls 数组。只有在工具已启用且参数能够按照工具 Schema 完整给出时才使用 directly_call_tool；参数不确定时改用 require_tool。

下面的 JSON 是完整可解析格式（工具名和参数值应替换为当前可用工具的真实 Schema）：

` + directlyCallToolScalarOutputExampleJSON + `
`

const directlyCallToolBatchOutputExamples = `
### directly_call_tool 并发调用

先枚举本轮已经明确、可立即执行的真实调用。当存在 2-8 个已启用工具调用，且这些调用彼此独立、互不干扰、都能立即给出完整 JSON 参数时，优先使用一个 directly_call_tool action 和 directly_call_tool_calls 数组；不要为了沿用单工具而拆成多个 ReAct 轮次。多 URL、多文件、多目标和多个只读探测是典型场景。优先选择 CACHE_TOOL_CALL 中已展示参数 Schema 的工具；运行时也能解析已启用但未缓存的工具。数组中的调用会并发执行，并在全部结束后统一进入下一轮。不要为凑数量发明调用，不要放置有先后依赖的调用，不要同时输出旧的 directly_call_tool_name 字段。批量参数不支持 AI-TAG；包含长文本参数的调用改用单次调用。

下面的 JSON 是完整可解析格式（工具名和参数值应替换为当前已启用的真实工具，优先使用 CACHE_TOOL_CALL 中已展示 Schema 的工具）：

` + directlyCallToolBatchOutputExampleJSON + `
`

const requireToolScalarOutputExamples = `
### require_tool 单次调用

仅当本轮恰好一个已明确调用且仍需运行时生成参数时，使用标量字段 tool_require_payload，不要为了一个调用创建 tool_require_calls 数组。tool_require_payload 只填写工具名，严禁在该字段中携带参数。

下面的 JSON 是完整可解析格式（工具名应替换为当前可用的真实工具）：

` + requireToolScalarOutputExampleJSON + `
`

const requireToolBatchOutputExamples = `
### require_tool 并发调用

先枚举本轮已经明确、可立即执行的真实调用。当存在 2-8 个彼此独立、互不干扰，但都需要运行时分别生成参数的调用时，优先使用一个 require_tool action 和 tool_require_calls 数组；不要为了沿用单工具而拆成多个 ReAct 轮次。多 URL、多文件、多目标和多个只读探测是典型场景。每项只提供工具名、identifier 和 reason，严禁提供 params。运行时会并发生成参数并有界并发执行。不要为凑数量发明调用；有依赖的调用必须拆到后续 ReAct 轮次；不要同时输出旧的 tool_require_payload 字段。

下面的 JSON 是完整可解析格式（工具名应替换为当前可用的真实工具）：

` + requireToolBatchOutputExampleJSON + `
`

const directlyCallToolOutputExamples = directlyCallToolBatchOutputExamples + directlyCallToolScalarOutputExamples

const requireToolOutputExamples = requireToolBatchOutputExamples + requireToolScalarOutputExamples

func directlyCallToolBatchSchemaOption() aitool.ToolOption {
	return aitool.WithStructArrayParam(
		directlyCallToolBatchField,
		[]aitool.PropertyOption{
			aitool.WithParam_Description("当本轮已明确 2-8 个互不依赖、互不干扰且参数完整的直接调用时，优先使用本数组，不要为了沿用单工具而拆成多轮。必须与 directly_call_tool_name/directly_call_tool_params 二选一，严禁混用。每项必须包含已启用工具的准确名称和完整内联 JSON 参数；优先选择 CACHE_TOOL_CALL 中已展示 Params Schema 的工具，未缓存但已启用的工具仍可解析并产生告警。批量参数不支持 AI-TAG；不要为凑数量发明调用。下面是经过 CI 校验且可执行的格式：\n" + directlyCallToolBatchOutputExampleJSON),
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
		}, aitool.WithParam_Required(true), aitool.WithParam_Description("该 child 工具调用的完整内联 JSON 参数。")),
		aitool.WithStringParam("identifier",
			aitool.WithParam_Description("可选。该 child 调用的唯一 snake_case 目的标识。")),
		aitool.WithStringParam("expectations",
			aitool.WithParam_Description("可选。该 child 调用的预计耗时和回退策略。")),
		aitool.WithStringParam("reason",
			aitool.WithParam_Description("可选。用简短短语说明该 child 调用具体做什么。")),
	)
}

func requireToolBatchSchemaOption() aitool.ToolOption {
	return aitool.WithStructArrayParam(
		requireToolBatchField,
		[]aitool.PropertyOption{
			aitool.WithParam_Description("当本轮已明确 2-8 个互不依赖、互不干扰且都需要生成参数的调用时，优先使用本数组，不要为了沿用单工具而拆成多轮。必须与 tool_require_payload 二选一，严禁混用。每项只填写工具名、identifier 和 reason，严禁提供 params；运行时会分别生成参数。不要为凑数量发明调用。下面是经过 CI 校验且可执行的格式：\n" + requireToolBatchOutputExampleJSON),
			aitool.WithParam_Raw("minItems", 2),
			aitool.WithParam_Raw("maxItems", aicommon.DefaultToolBatchMaxCalls),
		},
		[]aitool.PropertyOption{
			aitool.WithParam_Raw("additionalProperties", false),
		},
		aitool.WithStringParam("tool_name",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("需要由运行时生成参数的工具准确名称。")),
		aitool.WithStringParam("identifier",
			aitool.WithParam_Description("可选。该 child 调用的唯一 snake_case 目的标识。")),
		aitool.WithStringParam("reason",
			aitool.WithParam_Description("可选。用简短短语说明该 child 调用具体做什么。")),
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
		return utils.Errorf("%s requires at least 2 independent calls; use the legacy scalar fields for one call", field)
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

func parseDirectToolBatchAction(loop *reactloops.ReActLoop, action *aicommon.Action) (*aicommon.ToolBatchRequest, bool, error) {
	if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
		return nil, false, utils.Wrap(err, "directly_call_tool action parse failed")
	}

	items, hasBatch, err := parseCanonicalBatchItems(action, directlyCallToolBatchField)
	if err != nil || !hasBatch {
		return nil, hasBatch, err
	}
	if hasAnyCanonicalActionParam(action,
		"directly_call_tool_name",
		"directly_call_tool_params",
		"directly_call_identifier",
		"directly_call_expectations",
		"directly_call_reason",
	) {
		return nil, true, utils.Errorf("%s cannot be combined with legacy directly_call_tool_* fields", directlyCallToolBatchField)
	}
	if hasAnyCanonicalActionParam(action,
		requireToolBatchField,
		"tool_require_payload",
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
	request := &aicommon.ToolBatchRequest{Calls: make([]aicommon.ToolBatchCall, 0, len(items))}
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
			return nil, true, utils.Errorf("%s[%d].params is required (use {} for a parameterless tool)", directlyCallToolBatchField, index)
		}
		params, err := strictBatchParams(rawParams)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", directlyCallToolBatchField, index)
		}

		tool, err := mgr.GetToolByName(toolName)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d].tool_name %q is unavailable", directlyCallToolBatchField, index, toolName)
		}
		valid, validationErrors := tool.ValidateParams(params)
		if !valid {
			return nil, true, utils.Errorf("%s[%d].params are invalid for %q: %s", directlyCallToolBatchField, index, toolName, strings.Join(validationErrors, "; "))
		}

		if !mgr.IsRecentlyUsedTool(toolName) && loop.GetEmitter() != nil {
			loop.GetEmitter().EmitWarning("tool '%s' in %s[%d] is not in the recently-used cache; runtime will resolve it", toolName, directlyCallToolBatchField, index)
		}
		reactloops.MaybeWarnBashBeforeEdit(loop, toolName)
		request.Calls = append(request.Calls, aicommon.ToolBatchCall{
			Index:        index,
			Mode:         aicommon.ToolCallModeDirect,
			ToolName:     toolName,
			Params:       params,
			Identifier:   identifier,
			Expectations: expectations,
			Reason:       reason,
		})
	}
	return request, true, nil
}

func parseRequireToolBatchAction(loop *reactloops.ReActLoop, action *aicommon.Action) (*aicommon.ToolBatchRequest, bool, error) {
	if err := action.WaitParseResult(toolBatchVerifierContext(loop)); err != nil {
		return nil, false, utils.Wrap(err, "require_tool action parse failed")
	}

	items, hasBatch, err := parseCanonicalBatchItems(action, requireToolBatchField)
	if err != nil || !hasBatch {
		return nil, hasBatch, err
	}
	if hasAnyCanonicalActionParam(action, "tool_require_payload", "tool_call_reason") {
		return nil, true, utils.Errorf("%s cannot be combined with legacy tool_require_payload/tool_call_reason fields", requireToolBatchField)
	}
	if hasAnyCanonicalActionParam(action,
		directlyCallToolBatchField,
		"directly_call_tool_name",
		"directly_call_tool_params",
		"directly_call_identifier",
		"directly_call_expectations",
		"directly_call_reason",
	) {
		return nil, true, utils.Errorf("%s cannot be combined with directly_call_tool fields", requireToolBatchField)
	}
	if err := validateBatchLength(loop, requireToolBatchField, items); err != nil {
		return nil, true, err
	}

	mgr := loop.GetConfig().GetAiToolManager()
	if mgr == nil {
		return nil, true, utils.Error("tool manager is unavailable")
	}
	allowed := map[string]struct{}{
		"tool_name": {}, "identifier": {}, "reason": {},
	}
	identifiers := make(map[string]int)
	request := &aicommon.ToolBatchRequest{Calls: make([]aicommon.ToolBatchCall, 0, len(items))}
	for index, item := range items {
		if err := rejectUnknownBatchFields(item, allowed); err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", requireToolBatchField, index)
		}
		toolName, err := strictBatchString(item, "tool_name", true)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", requireToolBatchField, index)
		}
		identifier, err := strictBatchString(item, "identifier", false)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", requireToolBatchField, index)
		}
		if identifier != "" {
			if first, duplicate := identifiers[identifier]; duplicate {
				return nil, true, utils.Errorf("%s[%d].identifier duplicates %s[%d].identifier %q", requireToolBatchField, index, requireToolBatchField, first, identifier)
			}
			identifiers[identifier] = index
		}
		reason, err := strictBatchString(item, "reason", false)
		if err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d]", requireToolBatchField, index)
		}
		if _, err := mgr.GetToolByName(toolName); err != nil {
			return nil, true, utils.Wrapf(err, "%s[%d].tool_name %q is unavailable", requireToolBatchField, index, toolName)
		}
		reactloops.MaybeWarnBashBeforeEdit(loop, toolName)
		request.Calls = append(request.Calls, aicommon.ToolBatchCall{
			Index:      index,
			Mode:       aicommon.ToolCallModeRequire,
			ToolName:   toolName,
			Identifier: identifier,
			Reason:     reason,
		})
	}
	return request, true, nil
}

func executeVerifiedToolBatch(
	loop *reactloops.ReActLoop,
	stateKey string,
	operator *reactloops.LoopActionHandlerOperator,
) bool {
	raw := loop.GetVariable(stateKey)
	request, ok := raw.(*aicommon.ToolBatchRequest)
	if !ok || request == nil || len(request.Calls) == 0 {
		return false
	}

	invoker := loop.GetInvoker()
	ctx := invoker.GetConfig().GetContext()
	task := loop.GetCurrentTask()
	if task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}

	loopInfraStatus(loop, fmt.Sprintf("准备并发工具调用（%d 项） / Preparing Parallel Tool Calls...", len(request.Calls)))
	batchRuntime, supported := invoker.(aicommon.ToolBatchInvokeRuntime)
	var (
		result *aicommon.ToolBatchResult
		err    error
	)
	if supported {
		result, err = batchRuntime.ExecuteToolBatch(ctx, task, request)
	} else {
		// Compatibility fallback for third-party runtimes and old test doubles.
		// It deliberately stays serial because their emitter/task implementation
		// has not declared itself concurrency-safe.
		invoker.AddToTimeline("[TOOL_BATCH_COMPAT]", "runtime does not implement ToolBatchInvokeRuntime; executing the verified batch serially")
		result = executeToolBatchSerialFallback(ctx, invoker, request)
	}
	handleToolBatchActionResult(loop, ctx, invoker, request, result, err, operator)
	return true
}

func executeToolBatchSerialFallback(
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	request *aicommon.ToolBatchRequest,
) *aicommon.ToolBatchResult {
	result := &aicommon.ToolBatchResult{BatchID: request.BatchID, Outcomes: make([]aicommon.ToolCallOutcome, len(request.Calls))}
	cancelRemaining := func(start int, cause error) {
		if cause == nil {
			cause = context.Canceled
		}
		for remaining := start; remaining < len(request.Calls); remaining++ {
			pending := request.Calls[remaining]
			result.Outcomes[remaining] = aicommon.ToolCallOutcome{
				Index:         pending.Index,
				RequestedTool: pending.ToolName,
				FinalTool:     pending.ToolName,
				Stage:         aicommon.ToolCallStageCancelled,
				Err:           cause,
			}
		}
	}
	for index, call := range request.Calls {
		if ctx != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				cancelRemaining(index, ctxErr)
				break
			}
		}
		var (
			toolResult     *aitool.ToolResult
			directlyAnswer bool
			err            error
		)
		opts := []aicommon.ToolCallerOption{
			aicommon.WithToolCaller_Reason(call.Reason),
			aicommon.WithToolCaller_DestinationIdentifier(call.Identifier),
		}
		if call.Expectations != "" {
			opts = append(opts, aicommon.WithToolCaller_CallExpectations(call.Expectations))
		}
		if call.Mode == aicommon.ToolCallModeRequire {
			toolResult, directlyAnswer, err = invoker.ExecuteToolRequiredAndCall(ctx, call.ToolName, opts...)
		} else {
			params, cloneErr := deepCloneInvokeParams(call.Params)
			if cloneErr != nil {
				err = cloneErr
			} else {
				if call.Identifier != "" {
					params[aicommon.ReservedKeyIdentifier] = call.Identifier
				}
				if call.Expectations != "" {
					params[aicommon.ReservedKeyCallExpectations] = call.Expectations
				}
				toolResult, directlyAnswer, err = invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, call.ToolName, params, opts...)
			}
		}

		stage := aicommon.ToolCallStageDone
		cancelled := errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		if directlyAnswer || cancelled {
			stage = aicommon.ToolCallStageCancelled
		} else if toolResult == nil {
			stage = aicommon.ToolCallStageInvokeFailed
			if err == nil {
				err = utils.Error("tool invocation returned no result")
			}
		} else if err != nil || !toolResult.Success {
			stage = aicommon.ToolCallStageInvokeFailed
		}
		result.Outcomes[index] = aicommon.ToolCallOutcome{
			Index:          call.Index,
			RequestedTool:  call.ToolName,
			FinalTool:      call.ToolName,
			Stage:          stage,
			Result:         toolResult,
			Err:            err,
			DirectlyAnswer: directlyAnswer,
		}
		if directlyAnswer {
			result.DirectlyAnswer = true
			cancelRemaining(index+1, context.Canceled)
			break
		}
		if cancelled {
			cancelRemaining(index+1, err)
			break
		}
	}
	return result
}

func handleToolBatchActionResult(
	loop *reactloops.ReActLoop,
	ctx context.Context,
	invoker aicommon.AIInvokeRuntime,
	request *aicommon.ToolBatchRequest,
	result *aicommon.ToolBatchResult,
	err error,
	operator *reactloops.LoopActionHandlerOperator,
) {
	if err != nil {
		msg := fmt.Sprintf("tool batch execution failed before completion: %v", err)
		invoker.AddToTimeline("[TOOL_BATCH_ERROR]", msg)
		operator.Feedback(msg)
		operator.Continue()
		return
	}
	if result == nil {
		operator.Feedback("tool batch returned no result")
		operator.Continue()
		return
	}
	if result.DirectlyAnswer {
		answer, answerErr := invoker.DirectlyAnswer(ctx,
			"在并发工具调用审批中，用户中断了该批次并要求直接回答。不要继续执行该批次中的其他工具。", nil)
		if answerErr != nil {
			operator.Fail(utils.Wrap(answerErr, "DirectlyAnswer after tool batch"))
			return
		}
		invoker.AddToTimeline("directly-answer", answer)
		operator.Exit()
		return
	}

	outcomes := append([]aicommon.ToolCallOutcome(nil), result.Outcomes...)
	sort.SliceStable(outcomes, func(i, j int) bool { return outcomes[i].Index < outcomes[j].Index })
	lines := []string{fmt.Sprintf("Tool batch finished: %d calls", len(request.Calls))}
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
		}
		if outcome.Err != nil {
			status += ": " + outcome.Err.Error()
		} else if outcome.Result != nil && outcome.Result.Error != "" {
			status += ": " + outcome.Result.Error
		}
		lines = append(lines, fmt.Sprintf("%d. %s: %s", outcome.Index+1, toolName, status))

		if outcome.Result != nil && outcome.Result.Success {
			reactloops.MarkEditBeforeExecutionCompleted(loop, toolName)
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolName); lookupErr == nil {
				if cfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					cfg.RecordRecentlyUsedTool(cachedTool)
				} else {
					loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				}
			}
		}
	}
	summary := strings.Join(lines, "\n")
	invoker.AddToTimeline("[TOOL_BATCH_RESULT]", summary)
	operator.Feedback(summary)
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
	toolNames := make([]string, 0, len(request.Calls))
	for _, call := range request.Calls {
		toolNames = append(toolNames, call.ToolName)
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
