package reactloops

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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

//go:embed prompts/todo_list.txt
var todoListTemplate string

func (r *ReActLoop) generateSchemaString(disallowExit bool) (string, error) {
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
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE)
		disableActionList = append(disableActionList, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	}

	// directly_call_tool 只要 toolManager 存在就保留在 schema 中, 不再依赖
	// HasRecentlyUsedTools 的 0->1 跳变. 这是 P2.1 schema 字节稳定化的核心:
	// 第一次工具调用前后 schema enum / desc 都不变, semi-dynamic 段 hash 跨 turn
	// 一致, 让 dashscope prefix 缓存能持续命中.
	//
	// 安全兜底: 当 LLM 在没有 recent tools 时选 directly_call_tool, 该 action
	// 的 ActionVerifier (loopinfra/action_directly_call_tool.go) 会通过
	// IsRecentlyUsedTool 检查报错 "tool 'xxx' is not in the recently-used cache;
	// use require_tool instead", 触发 aiTransaction 重试, 让 LLM 改选 require_tool,
	// 行为与原 disable 路径等价.
	//
	// 关键词: P2.1, schema 字节稳定, HasRecentlyUsedTools 跳变消除, verifier 兜底
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
	for _, v := range values {
		if !slices.Contains(disableActionList, v.ActionType) && filterFunc(v) {
			filteredValues = append(filteredValues, v)
		} else {
			log.Infof("action[%s] is removed from schema because loop exit is disallowed or init disabled", v.ActionType)
		}
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

	// Mark init constraints as applied after first schema generation
	if !r.initActionApplied && (len(r.initActionMustUse) > 0 || len(r.initActionDisabled) > 0) {
		r.initActionApplied = true
	}

	schema := buildSchema(filteredValues...)
	return schema, nil
}

// generateLoopPrompt 生成 ReActLoop 一轮的完整 prompt。
//
// 参数:
//   - nonce: 当前 turn 的 nonce, 用于动态段标签
//   - userInput: 进入 dynamic 段 USER_QUERY 块的用户原始输入 / 当前任务 query
//     (跨 turn 不必稳定, 例如普通 ReAct 的用户当前轮次输入)
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
func (r *ReActLoop) generateLoopPrompt(
	nonce string,
	userInput string,
	frozenUserContext string,
	frozenPartitions []aicommon.FrozenBlockPartition,
	memory string,
	operator *LoopActionHandlerOperator,
) (string, error) {
	var tools []*aitool.Tool
	if r.toolsGetter == nil {
		tools = []*aitool.Tool{}
	} else {
		tools = r.toolsGetter()
	}

	schema, err := r.generateSchemaString(operator.disallowLoopExit)
	if err != nil {
		return "", err
	}
	r.lastLoopSchema = schema

	var persistent string
	if r.persistentInstructionProvider != nil {
		persistent, err = r.persistentInstructionProvider(r, "") // persistent context not use nonce
		if err != nil {
			r.lastLoopSchema = schema
			return "", utils.Wrap(err, "build persistent context failed")
		}
	}

	var outputExample string
	if r.reflectionOutputExampleProvider != nil {
		outputExample, err = r.reflectionOutputExampleProvider(r, "") // persistent context not use nonce
		if err != nil {
			return "", utils.Wrap(err, "build output example failed")
		}
	}

	var reactiveData string
	if r.reactiveDataBuilder != nil {
		reactiveData, err = r.reactiveDataBuilder(r, operator.GetFeedback(), nonce)
		if err != nil {
			return "", utils.Wrap(err, "build reactive data failed")
		}
		if reactiveData != "" {
			utils.Debug(func() {
				fmt.Println("---------- Reactive Data ----------")
				fmt.Println(reactiveData)
				fmt.Println("---------- Reactive Data ----------")
			})
		}
	}

	// Render skills context if the manager is available.
	// 三态分离: SkillsContext (SemiDynamic 1, 含 catalog) + ForcedSkills (frozen_block
	// 顶部) + AutoLoadedSkills (SemiDynamic 2 尾部).
	var skillsContext string
	var forcedSkillsBlock string
	var autoSkillsBlock string
	if r.skillsContextManager != nil {
		skillsContext = r.skillsContextManager.RenderStable()
		forcedSkillsBlock = r.skillsContextManager.RenderForcedSkills()
		autoSkillsBlock = r.skillsContextManager.RenderAutoLoadedSkills()
	}

	// Render extra capabilities discovered via intent recognition
	var extraCapabilities string
	if r.extraCapabilities != nil && r.extraCapabilities.HasCapabilities() {
		extraCapabilities = r.extraCapabilities.Render(nonce)
	}

	// 全局 TODO 块: 与 SessionEvidence 同处 timeline-open 段, 物理位置紧跟
	// SessionEvidence 之后. 任何 loop iteration 都能看到, 不再受限于 Verify
	// 调用时机. 数据源是 SessionPromptState 的 VerificationTodoStore, 由
	// VerifyUserSatisfaction 通过 ApplyVerificationTodoOps 增量写入.
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
		return "", utils.Error("invoker is nil in ReActLoop.generateLoopPrompt")
	}

	result, err := r.invoker.AssembleLoopPrompt(tools, &LoopPromptAssemblyInput{
		Nonce:             nonce,
		Lightweight:       r.useSpeedPriorityAI,
		UserQuery:         userInput,
		FrozenUserContext: frozenUserContext,
		FrozenPartitions:  frozenPartitions,
		TaskInstruction:   persistent,
		OutputExample:     outputExample,
		Schema:            schema,
		SkillsContext:     skillsContext,
		ForcedSkills:      forcedSkillsBlock,
		AutoLoadedSkills:  autoSkillsBlock,
		ExtraCapabilities: extraCapabilities,
		TodoSnapshot:      todoSnapshot,
		ReactiveData:      reactiveData,
		InjectedMemory:    memory,
	})
	if err != nil {
		return "", utils.Wrap(err, "assemble loop prompt failed")
	}
	if result == nil {
		return "", utils.Error("assemble loop prompt returned nil result")
	}
	observation := BuildPromptObservation(r.loopName, nonce, result.Prompt, getLoopPromptObservationSections(result))
	r.SetLastPromptObservation(observation)
	// 传 0 走 defaultPromptSummaryBytes; 当前默认 = 0 = 段内容完整透传.
	// 用户实测段体量在数 KB ~ 数十 KB 量级, 本地 ipc 完全可承受;
	// 想换成截断模式时改成显式正数即可.
	// 关键词: BuildStatus 不截断, 上下文成分完整展示
	status := observation.BuildStatus(0)
	r.SetLastPromptObservationStatus(status)
	r.emitPromptObservationStatus(status)
	if r.isDebugModeEnabled() {
		log.Infof("prompt section build report:\n%s", observation.RenderCLIReport(120))
	}
	return result.Prompt, nil
}

func getLoopPromptObservationSections(result *LoopPromptAssemblyResult) []*PromptSectionObservation {
	if result == nil {
		return nil
	}
	if sections, ok := result.Sections.([]*PromptSectionObservation); ok {
		return sections
	}
	return nil
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
