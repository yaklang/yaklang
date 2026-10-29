package reactloops

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const directlyCallToolParamsNodeID = "directly_call_tool_params"

func (r *ReActLoop) shouldRenderTodoSnapshot() bool {
	if r == nil || r.disableTodoSnapshot {
		return false
	}
	return true
}

var todoListTemplate = promptloader.MustLoad("ai/aid/aireact/reactloops/prompts/todo_list.txt")

func (r *ReActLoop) generateSchemaString(disallowExit bool, actionOperators ...*LoopActionHandlerOperator) (string, error) {
	filteredValues := r.getFilteredActions(disallowExit, actionOperators...)

	schemaText := buildSchema(filteredValues...)
	return applyToolBatchSchemaMaxItems(schemaText, r.toolBatchMaxCalls())
}

// prepareLoopActionSchemas selects actions once and builds only the schema
// representation used by this model turn.
func (r *ReActLoop) prepareLoopActionSchemas(operator *LoopActionHandlerOperator) (string, string, error) {
	filtered := r.getFilteredActions(operator != nil && operator.disallowLoopExit, operator)
	maxBatchCalls := r.toolBatchMaxCalls()
	var schema, functionCallSchemas string
	var nativeActionNames []string
	var nativeTools []aispec.Tool
	if r.functionCallMode {
		tools, err := buildActionTools(filtered, maxBatchCalls)
		if err != nil {
			return "", "", err
		}
		functionCallSchemas, err = renderFunctionCallSchemaTags(tools)
		if err != nil {
			return "", "", err
		}
		nativeActionNames = make([]string, 0, len(tools))
		for _, tool := range tools {
			nativeActionNames = append(nativeActionNames, tool.Function.Name)
		}
		nativeTools = tools
	} else {
		var err error
		schema, err = applyToolBatchSchemaMaxItems(buildSchema(filtered...), maxBatchCalls)
		if err != nil {
			return "", "", err
		}
	}
	// Rendering never consumes one-shot constraints. The first dispatched
	// business action owns that transition; maintenance and retries keep them.
	r.lastNativeActionNames = nativeActionNames
	r.lastNativeTools = nativeTools
	return schema, functionCallSchemas, nil
}

func (r *ReActLoop) toolBatchMaxCalls() int {
	maxBatchCalls := aicommon.DefaultToolBatchMaxCalls
	if concrete, ok := r.config.(*aicommon.Config); !ok || concrete.KeyValueConfig != nil {
		maxBatchCalls = r.config.GetConfigInt(aicommon.ConfigKeyToolBatchMaxCalls, maxBatchCalls)
	}
	if maxBatchCalls < 2 {
		maxBatchCalls = 2
	}
	if maxBatchCalls > aicommon.DefaultToolBatchMaxCalls {
		maxBatchCalls = aicommon.DefaultToolBatchMaxCalls
	}
	return maxBatchCalls
}

// getFilteredActions returns the list of LoopActions that should be visible
// to the model in this iteration, after applying all disable/must-use filters.
// Shared by text schema generation and per-action tool generation so both
// modes see the same action set.
func (r *ReActLoop) getFilteredActions(disallowExit bool, actionOperators ...*LoopActionHandlerOperator) []*LoopAction {
	// loop
	// build in code
	values := r.GetAllActions()
	disableActionList := []string{}
	if disallowExit {
		disableActionList = append(disableActionList, loopAction_Finish.ActionType)
	}
	if r.allowAIForge != nil && !r.allowAIForge() {
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_REQUIRE_AI_BLUEPRINT)
	}
	if r.allowPlanAndExec != nil && !r.allowPlanAndExec() {
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN)
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
	}

	if r.allowToolCall != nil && !r.allowToolCall() {
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL)
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	}

	// directly_call_tool 只要 toolManager 存在就保留在 schema 中, 不再依赖
	// HasRecentlyUsedTools 的 0->1 跳变. 这是 P2.1 schema 字节稳定化的核心:
	// 第一次工具调用前后 schema enum / desc 都不变, semi-dynamic 段 hash 跨 turn
	// 一致, 让 dashscope prefix 缓存能持续命中.
	//
	// 行为兜底: 当 LLM 在没有 recent tools 时选 directly_call_tool, verifier
	// 仍会从完整 ToolManager 解析工具并在 cache miss 时发 warning, 而不是仅因
	// recent cache 未命中而拒绝. Action description 继续建议模型在看不到可靠
	// Params Schema 时使用 require_tool; runtime 则允许名字与完整参数都正确的
	// enabled tool 直调, 避免把 recent cache 误当成执行白名单.
	//
	// 关键词: P2.1, schema 字节稳定, HasRecentlyUsedTools 跳变消除, cache miss warning
	toolManager := r.config.GetAiToolManager()
	if toolManager == nil {
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	}

	// Skills conditional actions
	if r.allowSkillLoading != nil && !r.allowSkillLoading() {
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_LOADING_SKILLS)
	}
	if r.allowSkillViewOffset != nil && !r.allowSkillViewOffset() {
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_CHANGE_SKILL_VIEW_OFFSET)
	}

	// Apply init handler action constraints (if not yet applied)
	// These constraints are only applied once after init
	if !r.initActionApplied && len(r.initActionDisabled) > 0 {
		disableActionList = append(disableActionList, r.initActionDisabled...)
		log.Infof("applied init action disabled list: %v", r.initActionDisabled)
	}
	var nextActionOperator *LoopActionHandlerOperator
	if len(actionOperators) > 0 {
		nextActionOperator = actionOperators[0]
	}
	if nextActionOperator != nil && len(nextActionOperator.GetNextActionDisabled()) > 0 {
		disableActionList = append(disableActionList, nextActionOperator.GetNextActionDisabled()...)
		log.Infof("applied next-iteration action disabled list: %v", nextActionOperator.GetNextActionDisabled())
	}

	filterFunc := func(action *LoopAction) bool {
		if r.actionFilters == nil {
			return true
		}
		for _, filter := range r.actionFilters {
			if !filter(action) {
				return false
			}
		}
		return true
	}

	var filteredValues []*LoopAction
	var asyncManagement *LoopAction
	var contextCompression *LoopAction
	for _, v := range values {
		if v.ActionType == schema.AI_REACT_LOOP_ACTION_MANAGE_ASYNC_EXECUTION_RUNTIME {
			asyncManagement = v
			continue
		}
		if v.ActionType == nativeAdjustTodolistActionName && !r.functionCallMode {
			continue
		}
		if v.ActionType == aicommon.ContextCompressionAction {
			contextCompression = v
			continue
		}
		if !slices.Contains(disableActionList, v.ActionType) && filterFunc(v) {
			filteredValues = append(filteredValues, v)
		} else {
			log.Infof("action[%s] is removed from schema because loop exit is disallowed or init disabled", v.ActionType)
		}
	}

	// 黑名单过滤结束后加回后台管理，下面的 MustUse 可以看见并点名它。
	if asyncManagement != nil {
		filteredValues = append(filteredValues, asyncManagement)
	}

	// Apply init handler must-use action constraints
	// If must-use actions are specified, only keep those actions
	if !r.initActionApplied && len(r.initActionMustUse) > 0 {
		var mustUseFiltered []*LoopAction
		for _, v := range filteredValues {
			if slices.Contains(r.initActionMustUse, v.ActionType) {
				mustUseFiltered = append(mustUseFiltered, v)
			}
		}
		if len(mustUseFiltered) > 0 {
			log.Infof("applied init action must-use list: %v, filtered from %d to %d actions",
				r.initActionMustUse, len(filteredValues), len(mustUseFiltered))
			filteredValues = mustUseFiltered
		} else {
			log.Warnf("init action must-use list %v did not match any available actions, keeping all", r.initActionMustUse)
		}
	}

	// Action handlers may narrow the immediately following model turn. This is
	// evaluated after the normal action filters so it cannot re-enable a hidden,
	// unsafe, or unavailable action.
	if nextActionOperator != nil && len(nextActionOperator.GetNextActionMustUse()) > 0 {
		var mustUseFiltered []*LoopAction
		for _, v := range filteredValues {
			if slices.Contains(nextActionOperator.GetNextActionMustUse(), v.ActionType) {
				mustUseFiltered = append(mustUseFiltered, v)
			}
		}
		if len(mustUseFiltered) > 0 {
			log.Infof("applied next-iteration action must-use list: %v, filtered from %d to %d actions",
				nextActionOperator.GetNextActionMustUse(), len(filteredValues), len(mustUseFiltered))
			filteredValues = mustUseFiltered
		} else {
			log.Warnf("next-iteration action must-use list %v did not match any available actions, keeping all",
				nextActionOperator.GetNextActionMustUse())
		}
	}

	// Maintenance admission is controlled by the loop's prompt mode, not the
	// business action filters. Keep its contract stable across both modes.
	if contextCompression != nil {
		filteredValues = append(filteredValues, contextCompression)
	}
	return filteredValues
}

// applyToolBatchSchemaMaxItems keeps the model-visible JSON Schema aligned
// with the runtime quota. The action definitions publish the protocol hard
// cap (8); deployments may lower it per Config without teaching the model an
// action that will inevitably be rejected by the verifier.
func applyToolBatchSchemaMaxItems(schemaText string, maxCalls int) (string, error) {
	if maxCalls >= aicommon.DefaultToolBatchMaxCalls {
		return schemaText, nil
	}
	var root map[string]any
	if err := json.Unmarshal([]byte(schemaText), &root); err != nil {
		return "", utils.Wrap(err, "decode loop schema for tool batch limit")
	}
	properties, ok := root["properties"].(map[string]any)
	if !ok {
		return schemaText, nil
	}
	changed := false
	for _, field := range []string{"directly_call_tool_params_group", "require_tool_payload"} {
		property, ok := properties[field].(map[string]any)
		if !ok {
			continue
		}
		if branches, ok := property["oneOf"].([]any); ok {
			for _, raw := range branches {
				if branch, ok := raw.(map[string]any); ok && branch["type"] == "array" {
					branch["maxItems"] = maxCalls
				}
			}
		} else {
			property["maxItems"] = maxCalls
		}
		changed = true
	}
	if !changed {
		return schemaText, nil
	}
	encoded, err := json.Marshal(root)
	if err != nil {
		return "", utils.Wrap(err, "encode loop schema with tool batch limit")
	}
	return string(encoded), nil
}

// generateLoopPrompt 生成 ReActLoop 一轮的完整 prompt。
//
// 参数:
//   - nonce: 当前 turn 的 nonce, 用于动态段标签
//   - userInput: 为历史压缩提供任务目标；主循环的用户输入原文只从 Timeline 读取。
//   - frozenUserContext: PE-TASK 的 PARENT_TASK + CURRENT_TASK + INSTRUCTION
//     三联块等"用户上下文块". 注入 timeline-open 段最末尾 (UserHistory 之后),
//     落在所有 cache 边界之外. 字段名虽含 "frozen", 但 PE-TASK 子任务切换
//     会让 CURRENT_TASK 内容抖动, 故主动放弃自身缓存以保护上游 SYSTEM /
//     FROZEN / SEMI 三段缓存命中.
//     普通场景传空串即可, 此时 timeline-open 段 PlanContext 子块自然不渲染,
//     段位置稳定.
//   - frozenPartitions: 调用方显式传入的 frozen-block 分区; config 上注册的
//     FrozenBlockPartitionProducer 会在 PromptMaterials 构造时追加.
//   - memory: 注入 memory 段
//   - operator: loop 运行时操作句柄
//
// 关键词: generateLoopPrompt, frozenUserContext, PLAN_CONTEXT 段,
//
//	prefix cache, PE-TASK 缓存优化
//
// loopPromptBuildInput pins the one-shot inputs of a single iteration so an
// overflow retry can rebuild the prompt without consuming them twice.
type loopPromptBuildInput struct {
	nonce             string
	userInput         string
	frozenUserContext string
	frozenPartitions  []aicommon.FrozenBlockPartition
	memory            string
	operator          *LoopActionHandlerOperator

	// Captured exactly once per iteration: these reads mutate loop state.
	subAgentInput    string
	subAgentRevision uint64
	todoCheckpoint   string
	actionSchemas    *loopPromptActionSchemas
	// overflowCompression asks this rebuild to compress only through the last
	// provider-accepted timeline boundary.
	overflowCompression bool
}

type loopPromptActionSchemas struct {
	text, native      string
	nativeActionNames []string
	nativeTools       []aispec.Tool
}

// captureLoopPromptBuildInput fixes the iteration inputs. The returned value is
// reused by every prompt rebuild of this iteration.
func (r *ReActLoop) captureLoopPromptBuildInput(
	nonce string,
	userInput string,
	frozenUserContext string,
	frozenPartitions []aicommon.FrozenBlockPartition,
	memory string,
	operator *LoopActionHandlerOperator,
) *loopPromptBuildInput {
	subAgentInput, subAgentRevision := r.prepareSubAgentPrompt()
	return &loopPromptBuildInput{
		nonce:             nonce,
		userInput:         userInput,
		frozenUserContext: frozenUserContext,
		frozenPartitions:  frozenPartitions,
		memory:            memory,
		operator:          operator,
		subAgentInput:     subAgentInput,
		subAgentRevision:  subAgentRevision,
		todoCheckpoint:    r.consumeTodoCheckpoint(),
	}
}

func (r *ReActLoop) generateLoopPrompt(
	nonce string,
	userInput string,
	frozenUserContext string,
	frozenPartitions []aicommon.FrozenBlockPartition,
	memory string,
	operator *LoopActionHandlerOperator,
) (string, error) {
	result, err := r.generateLoopPromptFromInput(r.captureLoopPromptBuildInput(
		nonce, userInput, frozenUserContext, frozenPartitions, memory, operator,
	))
	if err != nil {
		return "", err
	}
	return result.Prompt, nil
}

// Assembly returns the prompt, mode and section snapshot. Once the normal
// prompt exceeds the limit, it folds redundant history and rebuilds directly in
// compression mode. Observations are published only for real decision requests.
func (r *ReActLoop) generateLoopPromptFromInput(input *loopPromptBuildInput) (*LoopPromptAssemblyResult, error) {
	if input == nil {
		return nil, utils.Error("loop prompt build input is nil")
	}
	nonce, frozenUserContext := input.nonce, input.frozenUserContext
	frozenPartitions, memory, operator := input.frozenPartitions, input.memory, input.operator
	subAgentInput, subAgentRevision := input.subAgentInput, input.subAgentRevision
	var tools []*aitool.Tool
	if r.toolsGetter == nil {
		tools = []*aitool.Tool{}
	} else {
		tools = r.toolsGetter()
	}

	var err error
	var persistent string
	persistentProvider := r.persistentInstructionProvider
	if r.functionCallMode && r.functionCallInstructionProvider != nil {
		persistentProvider = r.functionCallInstructionProvider
	}
	if persistentProvider != nil {
		persistent, err = persistentProvider(r, "") // persistent context not use nonce
		if err != nil {
			return nil, utils.Wrap(err, "build persistent context failed")
		}
	}

	var outputExample string
	if !r.functionCallMode && r.outputExampleProvider != nil {
		outputExample, err = r.outputExampleProvider(r, "") // persistent context not use nonce
		if err != nil {
			return nil, utils.Wrap(err, "build output example failed")
		}
	}

	var reactiveData string
	if r.reactiveDataBuilder != nil {
		reactiveData, err = r.reactiveDataBuilder(r, operator.GetFeedback(), nonce)
		if err != nil {
			return nil, utils.Wrap(err, "build reactive data failed")
		}
		if reactiveData != "" {
			utils.Debug(func() {
				fmt.Println("---------- Reactive Data ----------")
				fmt.Println(reactiveData)
				fmt.Println("---------- Reactive Data ----------")
			})
		}
	}
	// Keep deliveries in the formal dynamic section so prompt analysis and the
	// model request agree about which evidence belongs to this iteration.
	reactiveData += subAgentInput
	todoCheckpoint := input.todoCheckpoint

	// Render extra capabilities discovered via intent recognition
	var extraCapabilities string
	if r.extraCapabilities != nil && r.extraCapabilities.HasCapabilities() {
		extraCapabilities = r.extraCapabilities.Render(nonce)
	}

	// TODO 快照跟在 Open Timeline 之后，位于 timeline-open 段。
	// 数据源是 SessionPromptState 的 VerificationTodoStore, 由
	// 正常 ReAct action 通过 ApplyTodoDelta 增量写入；verification 保持只读.
	// 关键词: TodoSnapshot 渲染, timeline-open 全局可见, SessionPromptState
	var todoSnapshot string
	if r.shouldRenderTodoSnapshot() {
		if todoContent := r.config.GetVerificationTodoRendered(aicommon.BuildVerificationTodoScope(r.GetCurrentTask())); todoContent != "" {
			todoSnapshot, err = utils.RenderTemplate(todoListTemplate, map[string]any{
				"Nonce": nonce,
				"Todo":  todoContent,
			})
			if err != nil {
				log.Warnf("render todo list template failed: %v", err)
				todoSnapshot = ""
			}
		}
	}

	// Execution authorization remains in AiToolManager. Prompt visibility is now
	// projected from Timeline Open / promoted Semi1, so do not inject a second
	// full summary on every prompt build.
	if tm := r.config.GetAiToolManager(); tm != nil && tm.HasRecentlyUsedTools() {
		r.syncRecentToolParamAITagFields(tm.GetRecentToolParamNames())
	}

	if r.invoker == nil {
		return nil, utils.Error("invoker is nil in ReActLoop.generateLoopPrompt")
	}

	// Providers may change the available actions (for example, the last planning
	// iteration removes exploration tools). Build both protocols only after those
	// transitions so the emitted schema matches the handlers for this iteration.
	if input.actionSchemas == nil {
		text, native, err := r.prepareLoopActionSchemas(operator)
		if err != nil {
			return nil, err
		}
		input.actionSchemas = &loopPromptActionSchemas{
			text: text, native: native,
			nativeActionNames: slices.Clone(r.lastNativeActionNames),
			nativeTools:       slices.Clone(r.lastNativeTools),
		}
	}
	schema, functionCallSchemas := input.actionSchemas.text, input.actionSchemas.native
	r.lastNativeActionNames = slices.Clone(input.actionSchemas.nativeActionNames)
	r.lastNativeTools = slices.Clone(input.actionSchemas.nativeTools)
	r.lastLoopSchema = schema
	r.syncPromptOverflowTimelineLimit()
	var planStatus string
	if r.planStatusProvider != nil {
		planStatus = r.planStatusProvider()
	} else if task := r.GetCurrentTask(); task != nil {
		if provider, ok := task.(aicommon.PlanStatusProvider); ok {
			planStatus = provider.GetPlanStatusForPrompt()
		}
	}

	assembly := &LoopPromptAssemblyInput{
		Nonce:                    nonce,
		FunctionCallMode:         r.functionCallMode,
		IncludeLatestModelReplay: true,
		Lightweight:              r.useSpeedPriorityAI,
		FrozenUserContext:        frozenUserContext,
		FrozenPartitions:         frozenPartitions,
		TaskInstruction:          persistent,
		OutputExample:            outputExample,
		Schema:                   schema,
		FunctionCallSchemas:      functionCallSchemas,
		ExtraCapabilities:        extraCapabilities,
		AutoContext:              r.renderAsyncExecutionContext(nonce),
		PlanStatus:               planStatus,
		TodoSnapshot:             todoSnapshot,
		ReactiveData:             reactiveData,
		InjectedMemory:           memory,
		TodoCheckpoint:           todoCheckpoint,
	}
	assemble := func() (*LoopPromptAssemblyResult, error) {
		// Custom assemblers may not report a watermark. Capture a conservative
		// boundary before rendering so their in-flight writes are still protected.
		var fallbackThroughID int64
		var fallbackItemIDs []int64
		if timeline := timelineFromConfig(r.config); timeline != nil {
			if assembly.HistoryThroughID != nil {
				fallbackThroughID, fallbackItemIDs = timeline.CompressionBoundary(*assembly.HistoryThroughID)
			} else {
				fallbackThroughID, fallbackItemIDs = timeline.CompressionBoundary()
			}
		}
		result, err := r.invoker.AssembleLoopPrompt(tools, assembly)
		if err != nil {
			return nil, utils.Wrap(err, "assemble loop prompt failed")
		}
		if result == nil {
			return nil, utils.Error("assemble loop prompt returned nil result")
		}
		result.Mode = assembly.PromptMode
		if result.TimelineThroughID == nil {
			result.TimelineThroughID = &fallbackThroughID
		}
		if result.TimelineItemIDs == nil {
			result.TimelineItemIDs = fallbackItemIDs
		}
		return result, nil
	}
	limit := r.wholePromptTokenLimit()
	result, err := assemble()
	if err != nil {
		return nil, err
	}
	promptTokens := int64(aicommon.MeasureTokens(result.Prompt))
	var compressionSelectionErr error
	if operator != nil {
		compressionSelectionErr = operator.contextCompressionError
	}

	// A selection error is a correction retry, independent of prompt volume.
	// overflowCompression is a provider signal and may use the last fitting
	// timeline boundary; an ordinary over-limit prompt keeps the full history.
	if timeline := timelineFromConfig(r.config); timeline != nil &&
		(compressionSelectionErr != nil || input.overflowCompression || promptTokens > limit) {
		ctx := r.config.GetContext()
		if task := r.GetCurrentTask(); task != nil && task.GetContext() != nil {
			ctx = task.GetContext()
		}
		target := max(int64(1), limit/6)
		options := aicommon.TimelineCompressionOptions{
			Context: ctx, PromptTokens: promptTokens, TargetTokens: target,
		}
		if input.overflowCompression && r.lastFittingTimelineThroughID != nil {
			throughID := *r.lastFittingTimelineThroughID
			options.ThroughID = &throughID
			assembly.HistoryThroughID = &throughID
		}
		guide, guideErr := timeline.RenderCompressionGuide(options)
		if guideErr != nil {
			return nil, guideErr
		}
		instructions, err := aicommon.RenderPromptTemplate("timeline-compression", compressContextDynamicText,
			map[string]any{"FunctionCallMode": r.functionCallMode})
		if err != nil {
			return nil, err
		}
		assembly.PromptMode = aicommon.LoopPromptModeCompressContext
		assembly.CompressionGuide = strings.TrimSpace(instructions) + "\n\n" + guide
		if compressionSelectionErr != nil {
			assembly.CompressionGuide += "\n\n# 上次压缩选择未通过校验\n请修正以下错误，重新提交 compress_context；当前历史尚未按这次无效选择修改。\n" +
				utils.ShrinkString(compressionSelectionErr.Error(), 2048)
		}
		result, err = assemble()
		if err != nil {
			return nil, err
		}
		if aicommon.TokenCountExceeds(result.Prompt, aicommon.TimelineCompressionMaxInputTokens) {
			return nil, utils.Error("context-compression prompt exceeds the input safety limit")
		}
	}
	r.commitSubAgentPrompt(subAgentRevision)
	r.subAgentModelSeen = subAgentRevision
	return result, nil
}

// syncRecentToolParamAITagFields 给当前 react loop 的 aiTagFields 注册
// CACHE_TOOL_CALL 提示用的 TOOL_PARAM_xxx AITAG 字段 (按近期工具的参数名).
//
// 关键设计: 这些字段把字面量稳定常量 aicommon.RecentToolCacheStableNonce
// ("[current-nonce]") 加进 ExtraNonces, 与 turn nonce 并列双注册:
//   - 渲染侧 (tool_manager.go) 用 "[current-nonce]" 占位符字面量, 让承载该
//     CACHE_TOOL_CALL 块的 prompt 段保持字节稳定, 进入 prefix cache.
//   - 解析侧给 turn nonce + "[current-nonce]" 都注册 callback. LLM 既可能
//     照抄占位符字面量输出, 也可能识破替换为 turn nonce 输出, 任一都能命中.
//
// nonce 候选追加是字段级精准的, 仅作用于 TOOL_PARAM_xxx (本函数注册的字段),
// 不会扩散到其他 aiTagFields (USER_QUERY 等仍只走 turn nonce).
//
// 关键词: syncRecentToolParamAITagFields, ExtraNonces 双注册,
//
//	[current-nonce] 占位符, 精准覆盖工具缓存
func (r *ReActLoop) syncRecentToolParamAITagFields(paramNames []string) {
	if r.aiTagFields == nil {
		return
	}
	for _, paramName := range aicommon.FilterSupportedToolParamAITagNames(paramNames) {
		paramName = strings.TrimSpace(paramName)
		if paramName == "" {
			continue
		}
		tagName := fmt.Sprintf("TOOL_PARAM_%s", paramName)
		r.aiTagFields.Set(tagName, &LoopAITagField{
			TagName:      tagName,
			VariableName: aicommon.GetToolParamAITagActionKey(paramName),
			AINodeId:     directlyCallToolParamsNodeID,
			ContentType:  "default",
			ExtraNonces:  []string{aicommon.RecentToolCacheStableNonce},
		})
	}
}
