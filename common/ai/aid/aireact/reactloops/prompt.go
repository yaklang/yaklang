package reactloops

import (
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"slices"
	"strings"
)

const directlyCallToolParamsNodeID = "directly_call_tool_params"

// renderRecentToolRoutingHint 渲染"快速工具路由提示", 整体使用占位符字面量
// nonce (aicommon.RecentToolCacheStableNonce, "[current-nonce]"), 跨 turn
// 字节稳定, 让该段进入 prefix cache. 占位符语义可让 LLM 自然把它关联到
// 当前 turn nonce.
//
// 历史: 该提示曾使用 turn nonce 渲染, 与 CACHE_TOOL_CALL 一并位于 dynamic 段
// REFLECTION 内, 每轮变化无法缓存. 现已与 CACHE_TOOL_CALL 一起迁到 semi-dynamic
// 段并用稳定 nonce 渲染.
//
// 关键词: renderRecentToolRoutingHint, DIRECT_TOOL_ROUTING, stable nonce,
//
//	semi-dynamic 段
func renderRecentToolRoutingHint() string {
	return utils.MustRenderTemplate(`
<|DIRECT_TOOL_ROUTING_{{ .Nonce }}|>
# Fast Tool Routing
- Before using require_tool, check CACHE_TOOL_CALL first.
- If the exact tool you need is already listed in CACHE_TOOL_CALL, prefer directly_call_tool for faster execution.
- Use require_tool only when the needed tool is not in the recent cache, or when you still need normal tool discovery.
<|DIRECT_TOOL_ROUTING_END_{{ .Nonce }}|>
	`, map[string]any{
		"Nonce": aicommon.RecentToolCacheStableNonce,
	})
}

//go:embed prompts/session_evidence.txt
var sessionEvidenceTemplate string

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
//     落在所有 cache 边界之外. 字段名虽含 "frozen", 但因 EvidenceOps 嵌入
//     root user input + 子任务切换实际会抖动, 故主动放弃自身缓存以保护上游
//     SYSTEM / FROZEN / SEMI 三段缓存命中.
//     普通场景传空串即可, 此时 timeline-open 段 PlanContext 子块自然不渲染,
//     段位置稳定.
//   - memory: 注入 memory 段
//   - operator: loop 运行时操作句柄
//
// 关键词: generateLoopPrompt, frozenUserContext, PLAN_CONTEXT 段,
//        prefix cache, PE-TASK 缓存优化
func (r *ReActLoop) generateLoopPrompt(
	nonce string,
	userInput string,
	frozenUserContext string,
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

	var persistent string
	if r.persistentInstructionProvider != nil {
		persistent, err = r.persistentInstructionProvider(r, "") // persistent context not use nonce
		if err != nil {
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

	// Render skills context if the manager is available
	var skillsContext string
	if r.skillsContextManager != nil {
		skillsContext = r.skillsContextManager.RenderStable()
	}

	// Render extra capabilities discovered via intent recognition
	var extraCapabilities string
	if r.extraCapabilities != nil && r.extraCapabilities.HasCapabilities() {
		extraCapabilities = r.extraCapabilities.Render(nonce)
	}

	var sessionEvidence string
	if evidenceContent := r.config.GetSessionEvidenceRendered(); evidenceContent != "" {
		sessionEvidence, err = utils.RenderTemplate(sessionEvidenceTemplate, map[string]any{
			"Nonce":    nonce,
			"Evidence": evidenceContent,
		})
		if err != nil {
			log.Warnf("render session evidence template failed: %v", err)
			sessionEvidence = ""
		}
	}

	// CACHE_TOOL_CALL 块的渲染. 整段都用稳定 nonce
	// aicommon.RecentToolCacheStableNonce, 让该段跨 turn 字节稳定; 物理位置从
	// dynamic/REFLECTION 迁到 semi-dynamic 段 (经 LoopPromptAssemblyInput.
	// RecentToolsCache 字段透传, 由 semi_dynamic_section.txt 模板渲染).
	//
	// 关键词: CACHE_TOOL_CALL 物理迁移, semi-dynamic 段, 稳定 nonce 渲染
	var recentToolsCacheBlock string
	if tm := r.config.GetAiToolManager(); tm != nil && tm.HasRecentlyUsedTools() {
		r.syncRecentToolParamAITagFields(tm.GetRecentToolParamNames())
		var sb strings.Builder
		sb.WriteString(renderRecentToolRoutingHint())
		// nonce 参数已被 GetRecentToolsSummary 内部忽略, 实际渲染使用稳定 nonce.
		// 这里仍然传 nonce 仅为了不破坏老接口签名.
		if summary := tm.GetRecentToolsSummary(tm.GetRecentToolCacheMaxTokens(), nonce); summary != "" {
			cacheBlock := utils.MustRenderTemplate(`
<|CACHE_TOOL_CALL_{{ .Nonce }}>
{{ .Summary }}
<|CACHE_TOOL_CALL_END_{{ .Nonce }}>
			`, map[string]interface{}{
				"Nonce":   aicommon.RecentToolCacheStableNonce,
				"Summary": summary,
			})
			sb.WriteString(cacheBlock)
		}
		recentToolsCacheBlock = sb.String()
	}

	if r.invoker == nil {
		return "", utils.Error("invoker is nil in ReActLoop.generateLoopPrompt")
	}

	result, err := r.invoker.AssembleLoopPrompt(tools, &LoopPromptAssemblyInput{
		Nonce:             nonce,
		UserQuery:         userInput,
		FrozenUserContext: frozenUserContext,
		TaskInstruction:   persistent,
		OutputExample:     outputExample,
		Schema:            schema,
		SkillsContext:     skillsContext,
		ExtraCapabilities: extraCapabilities,
		SessionEvidence:   sessionEvidence,
		ReactiveData:      reactiveData,
		InjectedMemory:    memory,
		RecentToolsCache:  recentToolsCacheBlock,
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
