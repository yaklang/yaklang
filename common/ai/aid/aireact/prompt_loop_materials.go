package aireact

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const (
	promptSectionTagName    = "PROMPT_SECTION"
	promptSectionHighStatic = "high-static"
	// promptSectionSemiDynamic1 / promptSectionSemiDynamic2 是 P1.1 把单一
	// semi-dynamic 段拆成两块后, 内层 PROMPT_SECTION 用的两个 nonce. 老 nonce
	// "semi-dynamic" 保留供老路径 (liteforge / aireduce) 与历史断言识别, 但
	// aireact 主路径不再使用.
	// 关键词: promptSectionSemiDynamic1/2, semi 拆两块, P1.1
	promptSectionSemiDynamic1 = "semi-dynamic-1"
	promptSectionSemiDynamic2 = "semi-dynamic-2"
	// promptSectionTimeline 是老段名 (合并 frozen + open + workspace), 现保留但
	// 主路径不再使用。新路径用 promptSectionTimelineOpen 表示"易变尾段"。
	// 关键词: promptSectionTimeline, 老 timeline 段名, 兼容
	promptSectionTimeline = "timeline"
	// promptSectionTimelineOpen 是 "按稳定性分层" 拆分后的 timeline 易变尾段:
	// 仅含最末 interval 桶 + 当前时间 + 工作目录 + (可选) midterm 检索结果。
	// 关键词: promptSectionTimelineOpen, timeline open, midterm
	promptSectionTimelineOpen = "timeline-open"
	promptSectionDynamic      = "dynamic"
	// aiCacheSystemTagName 仅用于 high-static 段：把"跨调用稳定的系统级指令"
	// 用独立 tagName 标记，让 aicache splitter / hijacker 与上游隐式缓存的
	// system 边界对齐；其他段保持 PROMPT_SECTION。
	// 关键词: aicache, AI_CACHE_SYSTEM, high-static system 边界
	aiCacheSystemTagName = "AI_CACHE_SYSTEM"
)

//go:embed prompts/loop/timeline_section.txt
var loopTimelineSectionTemplate string

//go:embed prompts/loop/dynamic_section.txt
var loopDynamicSectionTemplate string

func (r *ReAct) GetLoopPromptBaseMaterials(tools []*aitool.Tool, nonce string) (*reactloops.LoopPromptBaseMaterials, error) {
	if r == nil || r.promptManager == nil {
		return nil, fmt.Errorf("prompt manager is nil")
	}
	return r.promptManager.GetLoopPromptBaseMaterials(tools, nonce)
}

func (r *ReAct) AssembleLoopPrompt(tools []*aitool.Tool, input *reactloops.LoopPromptAssemblyInput) (*reactloops.LoopPromptAssemblyResult, error) {
	if r == nil || r.promptManager == nil {
		return nil, fmt.Errorf("prompt manager is nil")
	}
	return r.promptManager.AssembleLoopPrompt(tools, input)
}

func (r *ReAct) NewPromptMaterials(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) *aicommon.PromptMaterials {
	if r == nil || r.promptManager == nil {
		return nil
	}
	return r.promptManager.NewPromptMaterials(base, input)
}

func (r *ReAct) AssemblePromptPrefix(materials *aicommon.PromptMaterials) (*reactloops.PromptPrefixAssemblyResult, error) {
	if r == nil || r.promptManager == nil {
		return nil, fmt.Errorf("prompt manager is nil")
	}
	return r.promptManager.AssemblePromptPrefix(materials)
}

func (pm *PromptManager) GetLoopPromptBaseMaterials(tools []*aitool.Tool, nonce string) (*reactloops.LoopPromptBaseMaterials, error) {
	if pm == nil || pm.react == nil || pm.react.config == nil {
		return nil, fmt.Errorf("prompt manager is not initialized")
	}

	materials := &reactloops.LoopPromptBaseMaterials{
		Nonce:       nonce,
		Language:    pm.react.config.GetLanguage(),
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		OSArch:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		WorkingDir:  pm.workdir,
	}
	if pm.workdir != "" {
		materials.WorkingDirGlance = pm.GetGlanceWorkdir(pm.workdir)
	}

	taskType := "react"
	if forgeName := pm.react.config.GetForgeName(); forgeName != "" {
		taskType = "forge"
		materials.ForgeName = forgeName
	}
	materials.TaskType = taskType
	materials.AutoContext = pm.AutoContextWithNonce(nonce)
	materials.UserHistory = pm.UserHistoryContextWithNonce(nonce)

	// 按稳定性分层渲染 timeline:
	//   TimelineFrozen: reducer + 非末 interval, 字节稳定, 进 AI_CACHE_FROZEN 块
	//   TimelineOpen: 最末 interval (+ midterm 检索结果, midterm 在此处一次性消费)
	//   Timeline: frozen + open 的拼接, 仅供老观测路径作 fallback 使用
	// 关键词: GetLoopPromptBaseMaterials, timeline frozen/open 拆分, midterm 一次消费
	timeline := pm.react.config.GetTimeline()
	materials.TimelineFrozen = buildTimelineFrozenForPrompt(timeline)
	materials.TimelineOpen = buildTimelineOpenWithMidtermForPrompt(pm.react, timeline)
	materials.Timeline = joinTimelineFrozenOpen(materials.TimelineFrozen, materials.TimelineOpen)

	allowPlanAndExec := pm.react.config.GetEnablePlanAndExec() && pm.react.GetCurrentPlanExecutionTask() == nil
	allowToolCall := true
	hasLoadCapability := false

	if len(tools) == 0 {
		toolMgr := pm.react.config.GetAiToolManager()
		if toolMgr != nil {
			enableTools, err := toolMgr.GetEnableTools()
			if err != nil {
				return nil, err
			}
			tools = enableTools
		}
	}

	if currentLoop := pm.react.GetCurrentLoop(); currentLoop != nil {
		if getter := currentLoop.AllowPlanAndExec(); getter != nil {
			allowPlanAndExec = allowPlanAndExec && getter()
		}
		if getter := currentLoop.AllowToolCall(); getter != nil {
			allowToolCall = getter()
		}
		if actions := currentLoop.Actions(); actions != nil {
			_, hasLoadCapability = actions.Get("load_capability")
		}
	}

	materials.AllowPlanAndExec = allowPlanAndExec
	materials.AllowToolCall = allowToolCall
	materials.HasLoadCapability = hasLoadCapability
	materials.ShowForgeInventory = allowPlanAndExec && pm.react.config.GetShowForgeListInPrompt()

	if materials.ShowForgeInventory {
		materials.AIForgeList = pm.GetAvailableAIForgeBlueprints()
	}

	if allowToolCall && len(tools) > 0 {
		topCount := pm.react.config.GetTopToolsCount()
		topTools := pm.react.getPrioritizedTools(tools, topCount)
		materials.ToolsCount = len(tools)
		materials.TopTools = topTools
		materials.TopToolsCount = len(topTools)
		materials.HasMoreTools = len(tools) > len(topTools)
	}

	return materials, nil
}

func (pm *PromptManager) AssembleLoopPrompt(tools []*aitool.Tool, input *reactloops.LoopPromptAssemblyInput) (*reactloops.LoopPromptAssemblyResult, error) {
	if pm == nil {
		return nil, fmt.Errorf("prompt manager is nil")
	}
	if input == nil {
		return nil, fmt.Errorf("loop prompt assembly input is nil")
	}

	base, err := pm.GetLoopPromptBaseMaterials(tools, input.Nonce)
	if err != nil {
		return nil, err
	}

	prefixMaterials := pm.NewPromptMaterials(base, input)
	prefix, err := pm.AssemblePromptPrefix(prefixMaterials)
	if err != nil {
		return nil, err
	}
	dynamicData := pm.buildLoopPromptSectionData(base, input)
	dynamic, err := pm.renderLoopDynamicSection(dynamicData)
	if err != nil {
		return nil, err
	}

	sections := make([]*reactloops.PromptSectionObservation, 0, len(prefix.Sections))
	sections = append(sections, prefix.Sections...)
	if dynamicSection := pm.buildDynamicObservation(base, input, dynamic); dynamicSection != nil {
		sections = append(sections, dynamicSection)
	}

	prompt := buildTaggedPromptSections(
		prefix.HighStatic,
		prefix.FrozenBlock,
		prefix.SemiDynamic1,
		prefix.SemiDynamic2,
		prefix.TimelineOpen,
		dynamic,
		base.Nonce,
	)
	return &reactloops.LoopPromptAssemblyResult{
		Prompt:   prompt,
		Sections: sections,
	}, nil
}

func (pm *PromptManager) NewPromptMaterials(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) *aicommon.PromptMaterials {
	materials := &aicommon.PromptMaterials{}

	if base != nil {
		materials.Nonce = base.Nonce
		materials.AllowToolCall = base.AllowToolCall
		materials.AllowPlanAndExec = base.AllowPlanAndExec
		materials.HasLoadCapability = base.HasLoadCapability

		materials.ToolInventory = base.AllowToolCall && base.ToolsCount > 0
		materials.ToolsCount = base.ToolsCount
		materials.TopToolsCount = base.TopToolsCount
		materials.TopTools = append([]*aitool.Tool{}, base.TopTools...)
		materials.HasMoreTools = base.HasMoreTools
		materials.ForgeInventory = base.ShowForgeInventory && strings.TrimSpace(base.AIForgeList) != ""
		materials.AIForgeList = base.AIForgeList

		materials.Timeline = base.Timeline
		materials.TimelineFrozen = base.TimelineFrozen
		materials.TimelineOpen = base.TimelineOpen
		materials.CurrentTime = base.CurrentTime
		materials.OSArch = base.OSArch
		materials.WorkingDir = base.WorkingDir
		materials.WorkingDirGlance = base.WorkingDirGlance
		materials.Workspace = strings.TrimSpace(base.OSArch+base.WorkingDir+base.WorkingDirGlance) != ""
	}

	if input != nil {
		materials.TaskInstruction = input.TaskInstruction
		materials.OutputExample = input.OutputExample
		materials.SkillsContext = input.SkillsContext
		materials.Schema = input.Schema
		// P1-C2: SessionEvidence / UserHistory 从 dynamic 段上移到 timeline-open 段
		materials.SessionEvidence = input.SessionEvidence
		// CACHE_TOOL_CALL 块从 dynamic/REFLECTION 迁到 semi-dynamic 段
		// 关键词: RecentToolsCache 透传, semi-dynamic 段
		materials.RecentToolsCache = input.RecentToolsCache
		// PE-TASK PLAN 产物 (PARENT_TASK + CURRENT_TASK + INSTRUCTION) 通过
		// FrozenUserContext 字段透传, 渲染时位于 timeline-open 段最末尾
		// (UserHistory 之后), 落在所有 cache 边界之外。早期版本曾尝试
		// frozen-block / semi-dynamic, 但 EVIDENCE 嵌入 root user input
		// + 子任务切换造成 PlanContext 内容剧烈抖动, 破坏了上游缓存命中,
		// 现采用"放弃自身缓存, 保护上游缓存"策略。
		// 关键词: FrozenUserContext 透传, PLAN_CONTEXT, timeline-open 末尾,
		//        缓存边界外
		materials.FrozenUserContext = input.FrozenUserContext
	}
	if base != nil {
		// UserHistory 来自 LoopPromptBaseMaterials (config.FormatUserInputHistoryAITag)
		materials.UserHistory = base.UserHistory
	}

	return materials
}

// AssemblePromptPrefix 按"稳定性分层"路径输出 5 段: HighStatic | FrozenBlock |
// SemiDynamic1 (Skills + CacheToolCall) | SemiDynamic2 (TaskInstruction + Schema +
// OutputExample) | TimelineOpen。Prompt 字段是 5 段拼接结果, 调用方拼上 Dynamic
// 段后形成完整 prompt。
//
// FrozenBlock 段 (Tool Inventory + Forge Inventory + Timeline-frozen) 整体字节稳定,
// 由 buildTaggedPromptSections 用 <|AI_CACHE_FROZEN_semi-dynamic|>...
// <|AI_CACHE_FROZEN_END_semi-dynamic|> 标签包裹, 供 aicache hijacker
// splitByFrozenBoundary 精准切片 user1 (frozen prefix) / user2 (open tail)。
//
// SemiDynamic 段 P1.1 拆成两块:
//   - SemiDynamic1 (Skills + CacheToolCall): 被 <|AI_CACHE_SEMI_semi|>...END 包裹,
//     由 hijacker 切到 user2 (string content, 不打 cc).
//   - SemiDynamic2 (TaskInstruction + Schema + OutputExample): 被
//     <|AI_CACHE_SEMI2_semi|>...END 包裹, 由 hijacker 切到 user3 (ephemeral cc),
//     让 dashscope 把 semi-1+semi-2 合并算 prefix cache.
//
// 兼容字段 Timeline: 等于老 timeline 段 (frozen + open + workspace + current time)
// 的合并渲染, 仅供老 caller / 测试断言使用; 新路径不读取它。
//
// 关键词: AssemblePromptPrefix, 5 段拼接, 按稳定性分层, AI_CACHE_FROZEN, AI_CACHE_SEMI(2)
func (pm *PromptManager) AssemblePromptPrefix(materials *aicommon.PromptMaterials) (*reactloops.PromptPrefixAssemblyResult, error) {
	if pm == nil {
		return nil, fmt.Errorf("prompt manager is nil")
	}
	if materials == nil {
		return nil, fmt.Errorf("prompt prefix materials is nil")
	}

	prefixBuilder := aicommon.NewDefaultPromptPrefixBuilder()
	assembled, err := prefixBuilder.AssemblePromptPrefix(materials)
	if err != nil {
		return nil, err
	}
	highStatic := assembled.HighStatic
	frozenBlock := assembled.FrozenBlock
	semiDynamic1 := assembled.SemiDynamic
	semiDynamic2 := assembled.SemiDynamic2
	timelineOpen := assembled.TimelineOpen

	// 老 Timeline 段渲染保留, 仅写入 PromptPrefixAssemblyResult.Timeline 供观测/兼容;
	// 不进入新路径的 Prompt 拼接。
	legacyTimeline, err := pm.renderLoopTimelineSection(materials)
	if err != nil {
		return nil, err
	}

	sections := []*reactloops.PromptSectionObservation{
		pm.buildHighStaticObservation(materials, highStatic),
		pm.buildFrozenBlockObservation(materials, frozenBlock),
		pm.buildSemiDynamic1Observation(materials, semiDynamic1),
		pm.buildSemiDynamic2Observation(materials, semiDynamic2),
		pm.buildTimelineOpenObservation(materials, timelineOpen),
	}
	var filtered []*reactloops.PromptSectionObservation
	for _, section := range sections {
		if section != nil {
			filtered = append(filtered, section)
		}
	}

	return &reactloops.PromptPrefixAssemblyResult{
		Prompt:       assembled.Prompt,
		HighStatic:   highStatic,
		FrozenBlock:  frozenBlock,
		SemiDynamic1: semiDynamic1,
		SemiDynamic2: semiDynamic2,
		TimelineOpen: timelineOpen,
		Timeline:     legacyTimeline,
		Sections:     filtered,
	}, nil
}

func (pm *PromptManager) buildLoopPromptSectionData(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) map[string]any {
	data := map[string]any{
		"Nonce":              "",
		"UserQuery":          "",
		"TaskInstruction":    "",
		"OutputExample":      "",
		"Schema":             "",
		"SkillsContext":      "",
		"ExtraCapabilities":  "",
		"SessionEvidence":    "",
		"ReactiveData":       "",
		"InjectedMemory":     "",
		"AllowPlanAndExec":   false,
		"AllowToolCall":      false,
		"HasLoadCapability":  false,
		"ShowForgeInventory": false,
		"CurrentTime":        "",
		"OSArch":             "",
		"WorkingDir":         "",
		"WorkingDirGlance":   "",
		"Workspace":          false,
		"AutoContext":        "",
		"UserHistory":        "",
		"ToolsCount":         0,
		"TopToolsCount":      0,
		"TopTools":           []*aitool.Tool{},
		"HasMoreTools":       false,
		"ToolInventory":      false,
		"AIForgeList":        "",
		"ForgeInventory":     false,
		"Timeline":           "",
	}
	if base != nil {
		data["Nonce"] = base.Nonce
		data["AllowPlanAndExec"] = base.AllowPlanAndExec
		data["AllowToolCall"] = base.AllowToolCall
		data["HasLoadCapability"] = base.HasLoadCapability
		data["ShowForgeInventory"] = base.ShowForgeInventory
		data["CurrentTime"] = base.CurrentTime
		data["OSArch"] = base.OSArch
		data["WorkingDir"] = base.WorkingDir
		data["WorkingDirGlance"] = base.WorkingDirGlance
		data["Workspace"] = strings.TrimSpace(base.OSArch+base.WorkingDir+base.WorkingDirGlance) != ""
		data["AutoContext"] = base.AutoContext
		data["UserHistory"] = base.UserHistory
		data["ToolsCount"] = base.ToolsCount
		data["TopToolsCount"] = base.TopToolsCount
		data["TopTools"] = base.TopTools
		data["HasMoreTools"] = base.HasMoreTools
		data["ToolInventory"] = base.AllowToolCall && base.ToolsCount > 0
		data["AIForgeList"] = base.AIForgeList
		data["ForgeInventory"] = base.ShowForgeInventory && strings.TrimSpace(base.AIForgeList) != ""
		data["Timeline"] = base.Timeline
	}
	if input != nil {
		data["Nonce"] = input.Nonce
		data["UserQuery"] = input.UserQuery
		data["TaskInstruction"] = input.TaskInstruction
		data["OutputExample"] = input.OutputExample
		data["Schema"] = input.Schema
		data["SkillsContext"] = input.SkillsContext
		data["ExtraCapabilities"] = input.ExtraCapabilities
		data["SessionEvidence"] = input.SessionEvidence
		data["ReactiveData"] = input.ReactiveData
		data["InjectedMemory"] = input.InjectedMemory
	}
	return data
}

func (pm *PromptManager) buildHighStaticObservation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.high_static",
		"Highly Static",
		reactloops.PromptSectionRoleHighStatic,
	)
	// section.high_static 已重构为完全无变量的纯静态系统提示词:
	//   - OutputExample (caller-specific) 已迁到 semi-dynamic 段 Schema 之后
	//   - TaskInstruction (PERSISTENT, caller-specific) 已迁到 semi-dynamic 段
	//   - AllowToolCall / AllowPlanAndExec / HasLoadCapability 三个能力开关已移除,
	//     新模板无条件介绍全部能力, 实际可用性以 SCHEMA enum 为准
	// 让 high-static chunk hash 跨 caller / 跨 turn 字节恒定, 最大化 AI_CACHE_SYSTEM
	// 段的 prefix cache 命中率.
	//
	// 关键词: section.high_static 完全去变量, AI_CACHE_SYSTEM 字节稳定,
	//        OutputExample / TaskInstruction 迁到 semi-dynamic
	// 子节点 Name 已去掉 "Highly Static / " 前缀: UI 字节统计面板里父容器
	// "Highly Static" 已经表达层级, 子节点重复前缀只是噪声.
	// 关键词: section.high_static 子节点 Name 去前缀, UI 信息密度
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.high_static.static_preamble",
			"Traits & Agent Systems",
			reactloops.PromptSectionRoleHighStatic,
			false,
			pm.renderHighStaticPreamble(materials),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

// buildFrozenBlockObservation 给"AI_CACHE_FROZEN 块"做观测树:
// Tool Inventory + Forge Inventory + Timeline-frozen (reducer + 非末 interval)。
//
// 段顺序: Tool/Forge 是整个 root 任务生命周期都不变的"系统级"内容, 排在最前;
// Timeline-frozen 随时间轴增长可能间歇性扩展 (新一段 reducer 块产生时), 排在
// 最后, 让前两段的前缀缓存更稳定。
//
// PlanContext (PE-TASK PLAN 产物) 历史上曾在此段渲染, 但因仅 PE-TASK 子任务
// 有内容, root task / 普通 ReAct 时为空, 这种"有时存在有时不存在"的渲染态
// 会让 frozen-block 段字节内容随 task 类型剧烈抖动, 破坏 AI_CACHE_FROZEN
// prefix cache 命中。现已迁到 buildSemiDynamicResidualObservation
// (section.semi_dynamic.plan_context, AI_CACHE_SEMI 边界包裹)。
//
// 关键词: buildFrozenBlockObservation, Tool/Forge/Timeline-frozen,
//
//	AI_CACHE_FROZEN, PlanContext 已迁出
func (pm *PromptManager) buildFrozenBlockObservation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.frozen_block",
		"Frozen Block",
		reactloops.PromptSectionRoleFrozenBlock,
	)
	// 子节点 Name 已去掉 "Frozen Block / " 前缀: UI 字节统计面板里父容器
	// "Frozen Block" 已经表达层级.
	// 关键词: section.frozen_block 子节点 Name 去前缀, UI 信息密度
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.tool_inventory",
			"Tool Inventory",
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderToolInventoryBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.forge_inventory",
			"Forge Inventory",
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderForgeInventoryBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.timeline_frozen",
			"Timeline (Frozen Prefix)",
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderTimelineFrozenBlock(materials),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

// renderPlanContextBlock 渲染 PE-TASK 的 PLAN_CONTEXT 段。
//
// FrozenUserContext 的 nonce 由 (rootIdentifier, "plan_context") 派生, 与
// task.GetUserInput 内部使用的 (rawUserInput+parentInputs, "task_user_input")
// nonce 不同, 这是有意为之: 内层 PARENT_TASK / CURRENT_TASK / INSTRUCTION
// 三个标签已经用 plan-scoped 稳定 nonce 渲染好了, 外层 PLAN_CONTEXT 包装只
// 需要给观测层一个稳定的边界标记, 与内层标签命名空间互不冲突。
//
// 物理位置: timeline-open 段最末尾 (UserHistory 之后)。timeline-open 段不被
// AI_CACHE_FROZEN / AI_CACHE_SEMI 任何缓存边界包裹, 是"易变尾段"。这样安排
// 是因为 PlanContext 内容会随两个独立维度抖动:
//   - PE-TASK 子任务切换 (CURRENT_TASK 内容变化);
//   - root user input 因 EvidenceOps 触发 syncRootTaskPlanContextDocs 嵌入
//     新 FACTS/DOCUMENT 块而变化。
//
// 早期版本试图把 PlanContext 放进 frozen-block / semi-dynamic 等"可缓存段",
// 但这种本质易变的内容不适合缓存; 保护策略改为让其落在所有 cache 边界外,
// 不再追求 PlanContext 自身缓存, 而是保护更上游的 SYSTEM / FROZEN / SEMI
// 三段缓存命中率。
//
// 关键词: renderPlanContextBlock, PLAN_CONTEXT, plan-scoped nonce,
//
//	timeline-open 末尾, 缓存边界外, 上游缓存保护
func renderPlanContextBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil {
		return ""
	}
	body := strings.TrimSpace(materials.FrozenUserContext)
	if body == "" {
		return ""
	}
	nonce := aicommon.PlanScopedNonce(body, "plan_context")
	return fmt.Sprintf("<|PLAN_CONTEXT_%s|>\n%s\n<|PLAN_CONTEXT_END_%s|>", nonce, body, nonce)
}

// buildSemiDynamic1Observation 给"PROMPT_SECTION_semi-dynamic-1 段"做观测树:
// Skills Context + Cache Tool Call. 物理上对应 hijacker 5 段切分中的 user2
// (string content, 不打 cc), 与 buildSemiDynamic2Observation 一起被 dashscope
// 视作合并 prefix cache 计算.
//
// 关键词: buildSemiDynamic1Observation, Skills Context, Cache Tool Call, P1.1
func (pm *PromptManager) buildSemiDynamic1Observation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	// Role 用 PromptSectionRoleSemiDynamic1 ("半动态段1") 而非通用 SemiDynamic,
	// 让上下文字节统计图 / 上下文成分面板把 SemiDynamic1 与 SemiDynamic2 作为
	// 独立类型分开统计, 避免两块字节抖动被合并掩盖导致面板趋势线不稳定.
	// 关键词: section.semi_dynamic_1 独立 Role, 字节统计独立分类, P1.1
	section := reactloops.NewPromptContainerSection(
		"section.semi_dynamic_1",
		"Semi Dynamic 1",
		reactloops.PromptSectionRoleSemiDynamic1,
	)
	// 子节点 Name 已去掉 "Semi Dynamic 1 / " 前缀: UI 字节统计面板里父容器
	// 已经表达层级.
	// 关键词: section.semi_dynamic_1 子节点 Name 去前缀, UI 信息密度
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic_1.skills_context",
			"Skills Context",
			reactloops.PromptSectionRoleSemiDynamic1,
			true,
			materials.SkillsContext,
		),
		// CACHE_TOOL_CALL 块从 dynamic/REFLECTION 迁到此处, 用稳定 nonce 渲染.
		// 关键词: Semi Dynamic 1 / Cache Tool Call, RecentToolsCache 观测节点
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic_1.cache_tool_call",
			"Cache Tool Call",
			reactloops.PromptSectionRoleSemiDynamic1,
			true,
			materials.RecentToolsCache,
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

// buildSemiDynamic2Observation 给"PROMPT_SECTION_semi-dynamic-2 段"做观测树:
// TaskInstruction + Schema + OutputExample. 物理上对应 hijacker 5 段切分中的
// user3 (ephemeral cc), 与 buildSemiDynamic1Observation 一起被 dashscope 视作
// 合并 prefix cache 计算 (cc 锚点落在本段末尾, prefix 跨过 semi-1).
//
// PlanContext 已彻底迁出本段 (历史曾位于 semi-dynamic 段, 但 EVIDENCE 嵌入 root
// user input 导致内容抖动破坏 AI_CACHE_SEMI 命中; 现已迁到 timeline-open 段末尾,
// 落在所有 cache 边界之外, 见 buildTimelineOpenObservation)。
//
// 关键词: buildSemiDynamic2Observation, TaskInstruction, Schema, OutputExample,
//
//	AI_CACHE_SEMI2 cc, P1.1, PlanContext 已迁出至 timeline-open 末尾
func (pm *PromptManager) buildSemiDynamic2Observation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	// Role 用 PromptSectionRoleSemiDynamic2 ("半动态段2") 而非通用 SemiDynamic,
	// 让上下文字节统计图 / 上下文成分面板把 SemiDynamic1 与 SemiDynamic2 作为
	// 独立类型分开统计.
	// 关键词: section.semi_dynamic_2 独立 Role, 字节统计独立分类, P1.1
	section := reactloops.NewPromptContainerSection(
		"section.semi_dynamic_2",
		"Semi Dynamic 2",
		reactloops.PromptSectionRoleSemiDynamic2,
	)
	// 子节点 Name 已去掉 "Semi Dynamic 2 / " 前缀: UI 字节统计面板里父容器
	// 已经表达层级.
	// 关键词: section.semi_dynamic_2 子节点 Name 去前缀, UI 信息密度
	children := []*reactloops.PromptSectionObservation{
		// section.semi_dynamic_2.task_instruction 从 high-static 段迁入:
		// TaskInstruction 是 caller 注入的 PERSISTENT 指令, caller-specific,
		// 跨同一 caller 的 turn 字节稳定. 留在 high-static 段会污染
		// AI_CACHE_SYSTEM 边界, 因此下沉到 semi-dynamic 段.
		// 关键词: section.semi_dynamic_2.task_instruction, PERSISTENT 迁入,
		//        high-static 反污染
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic_2.task_instruction",
			"Task Instruction",
			reactloops.PromptSectionRoleSemiDynamic2,
			true,
			renderStaticTaggedBlock("PERSISTENT", materials.TaskInstruction),
		),
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic_2.schema",
			"Schema",
			reactloops.PromptSectionRoleSemiDynamic2,
			true,
			renderSchemaBlock(materials.Schema),
		),
		// section.semi_dynamic_2.output_example 从 high-static 段迁入:
		// OutputExample 是 caller-specific 字段, 不同 forge / loop 注入的内容
		// 差异较大, 留在 high-static 段会破坏 AI_CACHE_SYSTEM 段的 hash 稳定性.
		// 关键词: section.semi_dynamic_2.output_example, OutputExample 迁入,
		//        high-static 反污染
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic_2.output_example",
			"Output Example",
			reactloops.PromptSectionRoleSemiDynamic2,
			true,
			renderStaticTaggedBlock("OUTPUT_EXAMPLE", materials.OutputExample),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

// buildTimelineOpenObservation 给"PROMPT_SECTION_timeline-open 段"做观测树:
// Timeline 末桶 (+ midterm 检索结果) + Current Time + Workspace +
// SessionEvidence + UserHistory + PlanContext (末尾)。
//
// PlanContext 在本段最末尾, 是因为 PE-TASK PLAN 产物本质易变 (子任务切换 +
// FACTS/DOCUMENT 嵌入 root user input), 不适合放任何 cache 边界内。
// timeline-open 段位于 system / frozen / semi 三段缓存之外, 让 PlanContext
// 抖动不再污染上游缓存命中率。
//
// 关键词: buildTimelineOpenObservation, Timeline 末桶, Current Time, Workspace,
//
//	SessionEvidence, UserHistory, PlanContext 末尾, 缓存边界外
func (pm *PromptManager) buildTimelineOpenObservation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.timeline_open",
		"Timeline Open & Workspace",
		reactloops.PromptSectionRoleTimelineOpen,
	)
	// 子节点 Name 已去掉 "Timeline Open / " 前缀: UI 字节统计面板里父容器
	// "Timeline Open & Workspace" 已经表达层级.
	// 关键词: section.timeline_open 子节点 Name 去前缀, UI 信息密度
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.timeline_open",
			"Timeline (Open Tail)",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			renderTimelineOpenBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.current_time",
			"Current Time",
			reactloops.PromptSectionRoleTimelineOpen,
			false,
			renderCurrentTimeBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.workspace",
			"Workspace",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			renderWorkspaceBlock(materials),
		),
		// P1-C2: SessionEvidence (SESSION_ARTIFACTS) 上移到 timeline-open
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.session_evidence",
			"Session Evidence",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			materials.SessionEvidence,
		),
		// P1-C2: UserHistory (PREV_USER_INPUT) 上移到 timeline-open
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.user_history",
			"User History",
			reactloops.PromptSectionRoleTimelineOpen,
			false,
			materials.UserHistory,
		),
		// PlanContext (PE-TASK PLAN 产物) 末尾注入: 该字段仅 PE-TASK 子任务
		// 非空, 内容随子任务切换 + EvidenceOps 嵌入 root user input 抖动剧烈,
		// 不适合放任何 cache 边界内。放 timeline-open 段最末让其落在所有
		// cache 边界之外, 不污染上游 system / frozen / semi 三段缓存命中。
		// 关键词: section.timeline_open.plan_context, PLAN_CONTEXT 末尾,
		//        缓存边界外, 上游缓存保护
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.plan_context",
			"Plan Context (PE-TASK PLAN Output)",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			renderPlanContextBlock(materials),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

func (pm *PromptManager) buildDynamicObservation(
	base *reactloops.LoopPromptBaseMaterials,
	input *reactloops.LoopPromptAssemblyInput,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.dynamic",
		"Pure Dynamic",
		reactloops.PromptSectionRoleDynamic,
	)
	// 子节点 Name 已去掉 "Pure Dynamic / " 前缀: UI 字节统计面板里父容器
	// "Pure Dynamic" 已经表达层级.
	// 关键词: section.dynamic 子节点 Name 去前缀, UI 信息密度
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.dynamic.user_query",
			"User Query",
			reactloops.PromptSectionRoleDynamic,
			false,
			renderUserQueryBlock(input.Nonce, input.UserQuery),
		),
		reactloops.NewPromptSectionObservation(
			"section.dynamic.auto_context",
			"Auto Context",
			reactloops.PromptSectionRoleDynamic,
			true,
			base.AutoContext,
		),
		// P1-C2: user_history 已上移到 section.timeline_open.user_history,
		// 此处 dynamic 段不再渲染 PREV_USER_INPUT.
		reactloops.NewPromptSectionObservation(
			"section.dynamic.extra_capabilities",
			"Extra Capabilities",
			reactloops.PromptSectionRoleDynamic,
			true,
			renderTaggedBlock("EXTRA_CAPABILITIES", input.Nonce, input.ExtraCapabilities),
		),
		// P1-C2: session_evidence 已上移到 section.timeline_open.session_evidence,
		// 此处 dynamic 段不再渲染 SESSION_ARTIFACTS.
		reactloops.NewPromptSectionObservation(
			"section.dynamic.reactive_data",
			"Reactive Data",
			reactloops.PromptSectionRoleDynamic,
			true,
			renderTaggedBlock("REFLECTION", input.Nonce, input.ReactiveData),
		),
		reactloops.NewPromptSectionObservation(
			"section.dynamic.injected_memory",
			"Injected Memory",
			reactloops.PromptSectionRoleDynamic,
			true,
			renderInjectedMemoryBlock(input.Nonce, input.InjectedMemory),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

// renderHighStaticPreamble 渲染 high-static 段的"前导文" (TRAITS + 方法论
// 协议块 + 能力系统介绍). 当前 high_static_section.txt 已重构为完全无变量的
// 纯静态系统提示词, HighStaticData() 返回空 map, 这里只是把模板原文 trim 后返回.
// 若以后又向 HighStaticData 注入 caller-specific 字段, 需要重新审视: 任何
// caller-specific 内容都会破坏 AI_CACHE_SYSTEM 段的 prefix cache, 应优先放
// SemiDynamic1Data / SemiDynamic2Data 而不是 HighStaticData.
//
// 关键词: renderHighStaticPreamble, high-static 纯静态, AI_CACHE_SYSTEM 反污染
func (pm *PromptManager) renderHighStaticPreamble(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil {
		return ""
	}
	rendered, err := aicommon.RenderPromptTemplate("loop-high-static-preamble", aicommon.SharedPlanAndExecHighStaticTemplate, materials.HighStaticData())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(rendered)
}

func (pm *PromptManager) AutoContext() string {
	if pm == nil || pm.cpm == nil || pm.react == nil || pm.react.config == nil {
		return ""
	}
	return pm.cpm.ExecuteStable(pm.react.config, pm.react.config.Emitter)
}

func (pm *PromptManager) AutoContextWithNonce(nonce string) string {
	if pm == nil || pm.cpm == nil || pm.react == nil || pm.react.config == nil {
		return ""
	}
	return pm.cpm.ExecuteWithNonce(pm.react.config, pm.react.config.Emitter, nonce)
}

func (pm *PromptManager) UserHistoryContext() string {
	if pm == nil || pm.react == nil || pm.react.config == nil {
		return ""
	}
	return pm.react.config.FormatUserInputHistory()
}

func (pm *PromptManager) UserHistoryContextWithNonce(nonce string) string {
	if pm == nil || pm.react == nil || pm.react.config == nil {
		return ""
	}
	return pm.react.config.FormatUserInputHistoryAITag(nonce, prevUserInputTagMaxTokens)
}

func filterIncludedPromptSections(items []*reactloops.PromptSectionObservation) []*reactloops.PromptSectionObservation {
	var result []*reactloops.PromptSectionObservation
	for _, item := range items {
		if item != nil && item.IsIncluded() {
			result = append(result, item)
		}
	}
	return result
}

func renderTaggedBlock(tag string, nonce string, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tag, nonce, body, tag, nonce)
}

func renderStaticTaggedBlock(tag string, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("<|%s|>\n%s\n<|%s_END|>", tag, body, tag)
}

func renderSchemaBlock(schema string) string {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		return ""
	}
	return fmt.Sprintf("响应格式输出JSON和<|TAG...|>，请遵守如下Schema ：\n\n<|SCHEMA|>\n```jsonschema\n%s\n```\n<|SCHEMA|>", schema)
}

func renderWorkspaceBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil {
		return ""
	}
	var lines []string
	if !materials.Workspace || (strings.TrimSpace(materials.OSArch) == "" && strings.TrimSpace(materials.WorkingDir) == "" && strings.TrimSpace(materials.WorkingDirGlance) == "") {
		return ""
	}
	lines = append(lines, "# Workspace Context")
	if materials.OSArch != "" {
		lines = append(lines, "OS/Arch: "+materials.OSArch)
	}
	if materials.WorkingDir != "" {
		lines = append(lines, "working dir: "+materials.WorkingDir)
	}
	if materials.WorkingDirGlance != "" {
		lines = append(lines, "working dir glance: "+materials.WorkingDirGlance)
	}
	return strings.Join(lines, "\n")
}

func renderToolInventoryBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || !materials.ToolInventory || materials.ToolsCount <= 0 || len(materials.TopTools) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("# Tool Inventory\nYou have access to %d built-in tools. Top %d tools:", materials.ToolsCount, materials.TopToolsCount))
	for _, tool := range materials.TopTools {
		if tool == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("* `%s`: %s", tool.Name, tool.Description))
	}
	if materials.HasMoreTools {
		lines = append(lines, "... use `search_capabilities` to discover more tools and related capabilities.")
	}
	return strings.Join(lines, "\n")
}

func renderForgeInventoryBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || !materials.ForgeInventory || strings.TrimSpace(materials.AIForgeList) == "" {
		return ""
	}
	return "# AI Blueprint Inventory\n以下是当前可直接调用的 AI 蓝图列表：\n" + materials.AIForgeList
}

func renderTimelineBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.Timeline) == "" {
		return ""
	}
	return "# Timeline Memory\n" + materials.Timeline
}

// renderTimelineFrozenBlock 渲染 timeline 冻结前缀 (reducer + 非末 interval)。
// 用于 FrozenBlock 段的观测树, 不带 frozen 边界 tag (边界由 wrapAICacheFrozen 统一加)。
//
// 关键词: renderTimelineFrozenBlock, Timeline frozen, FrozenBlock 观测
func renderTimelineFrozenBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.TimelineFrozen) == "" {
		return ""
	}
	return "# Timeline Memory (Frozen Prefix)\n" + materials.TimelineFrozen
}

// renderTimelineOpenBlock 渲染 timeline 开放尾段 (最末 interval + midterm prefix)。
// 用于 TimelineOpen 段的观测树。
//
// 关键词: renderTimelineOpenBlock, Timeline open, midterm
func renderTimelineOpenBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.TimelineOpen) == "" {
		return ""
	}
	return "# Timeline Memory (Open Tail)\n" + materials.TimelineOpen
}

// joinTimelineFrozenOpen 把 frozen + open 两半 timeline 合成一条字符串, 仅供老
// 观测路径作 fallback。两半都非空时以单空行分隔, 任一半为空则返回另一半。
//
// 注意: 此函数不再向输出中注入 AI_CACHE_FROZEN 边界标签 (边界由 wrapAICacheFrozen
// 在更高层用统一 tag 包裹)。如果有外部代码期望旧 Dump() 风格 (含 boundary tag),
// 请改用 timeline.Dump()。
//
// 关键词: joinTimelineFrozenOpen, Timeline frozen + open 合并, 兼容字段
func joinTimelineFrozenOpen(frozen, open string) string {
	frozen = strings.TrimRight(frozen, "\n")
	open = strings.TrimLeft(open, "\n")
	switch {
	case frozen == "" && open == "":
		return ""
	case frozen == "":
		return open
	case open == "":
		return frozen
	default:
		return frozen + "\n" + open
	}
}

func renderCurrentTimeBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.CurrentTime) == "" {
		return ""
	}
	return "# Current Time\n" + materials.CurrentTime
}

func renderUserQueryBlock(nonce string, userQuery string) string {
	userQuery = strings.TrimSpace(userQuery)
	if userQuery == "" {
		return ""
	}
	return fmt.Sprintf("<|USER_QUERY_%s|>\n%s\n<|USER_QUERY_END_%s|>", nonce, userQuery, nonce)
}

func renderInjectedMemoryBlock(nonce string, memory string) string {
	memory = strings.TrimSpace(memory)
	if memory == "" {
		return ""
	}
	return fmt.Sprintf("<|INJECTED_MEMORY_%s|>\n# Memory Context\nThese are the memories automatically retrieved by the system that are most relevant to the current input.\n%s\n<|INJECTED_MEMORY_END_%s|>", nonce, memory, nonce)
}

func (pm *PromptManager) renderLoopHighStaticSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return aicommon.RenderPromptTemplate("loop-high-static", aicommon.SharedPlanAndExecHighStaticTemplate, materials.HighStaticData())
}

// renderLoopSemiDynamic1Section 渲染 P1.1 拆分后的 semi-dynamic 第一块:
// SkillsContext + RecentToolsCache. 物理上对应 hijacker 5 段切分中的 user2
// (string content, 不打 cc), 由 wrapAICacheSemi 包一层 AI_CACHE_SEMI 边界.
//
// 关键词: renderLoopSemiDynamic1Section, semi_dynamic_section_1.txt, P1.1
func (pm *PromptManager) renderLoopSemiDynamic1Section(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return aicommon.RenderPromptTemplate("loop-semi-dynamic-1", aicommon.SharedSemiDynamic1Template, materials.SemiDynamic1Data())
}

// renderLoopSemiDynamic2Section 渲染 P1.1 拆分后的 semi-dynamic 第二块:
// TaskInstruction + Schema + OutputExample. 物理上对应 hijacker 5 段切分中的
// user3 (ephemeral cc), 由 wrapAICacheSemi2 包一层 AI_CACHE_SEMI2 边界, dashscope
// 把 semi-1+semi-2 视作合并 prefix cache 计算.
//
// 关键词: renderLoopSemiDynamic2Section, semi_dynamic_section_2.txt, P1.1,
//
//	AI_CACHE_SEMI2 cc
func (pm *PromptManager) renderLoopSemiDynamic2Section(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return aicommon.RenderPromptTemplate("loop-semi-dynamic-2", aicommon.SharedTaskInstructionSchemaExampleTemplate, materials.SemiDynamic2Data())
}

// renderLoopFrozenBlockSection 渲染"按稳定性分层"路径下的 FrozenBlock 段
// (Tool Inventory + Forge Inventory + Timeline-frozen)。模板中只引用稳定字段。
//
// 关键词: renderLoopFrozenBlockSection, frozen_block_section.txt
func (pm *PromptManager) renderLoopFrozenBlockSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return aicommon.RenderPromptTemplate("loop-frozen-block", aicommon.SharedFrozenBlockTemplate, materials.FrozenBlockData())
}

// renderLoopTimelineOpenSection 渲染"按稳定性分层"路径下的 TimelineOpen 段
// (Timeline 末桶 + Current Time + Workspace)。
//
// 关键词: renderLoopTimelineOpenSection, timeline_open_section.txt
func (pm *PromptManager) renderLoopTimelineOpenSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return aicommon.RenderPromptTemplate("loop-timeline-open", aicommon.SharedTimelineOpenTemplate, materials.TimelineOpenData())
}

// renderLoopTimelineSection 是老路径的 Timeline 段渲染 (frozen + open + workspace
// 合并)。仅供 PromptPrefixAssemblyResult.Timeline 字段填充, 主路径不再消费。
//
// 关键词: renderLoopTimelineSection, 老 timeline 段, 兼容
func (pm *PromptManager) renderLoopTimelineSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return pm.executeTemplate("loop-timeline", loopTimelineSectionTemplate, materials.TimelineData())
}

func (pm *PromptManager) renderLoopDynamicSection(data map[string]any) (string, error) {
	return pm.executeTemplate("loop-dynamic", loopDynamicSectionTemplate, data)
}

// buildTaggedPromptSections 按"按稳定性分层"路径拼接 6 段:
//
//	SYSTEM (high-static) | FROZEN (frozen-block) | SEMI-1 | SEMI-2 |
//	OPEN (timeline-open) | DYNAMIC
//
// 各段之间空行分隔, 空段省略。
//
// 三 cache 边界 (P1.1):
//   - frozen-block 段: <|AI_CACHE_FROZEN_semi-dynamic|>...<|AI_CACHE_FROZEN_END_semi-dynamic|>
//     由 wrapAICacheFrozen 注入, hijacker 切到 user1, 主动打 ephemeral cc
//   - semi-dynamic-1 段: <|AI_CACHE_SEMI_semi|>...<|AI_CACHE_SEMI_END_semi|>
//     由 wrapAICacheSemi 注入 (覆盖 PROMPT_SECTION_semi-dynamic-1 整段),
//     hijacker 切到 user2, 不打 cc (string content)
//   - semi-dynamic-2 段: <|AI_CACHE_SEMI2_semi|>...<|AI_CACHE_SEMI2_END_semi|>
//     由 wrapAICacheSemi2 注入 (覆盖 PROMPT_SECTION_semi-dynamic-2 整段),
//     hijacker 切到 user3, 主动打 ephemeral cc; dashscope 视 semi-1+semi-2 为
//     合并 prefix cache (cc 锚点落 semi-2 末尾, prefix 跨过 semi-1)
//
// 内层 PROMPT_SECTION_semi-dynamic-1 / -2 标签保留 (不会与 AI_CACHE_SEMI / SEMI2
// 冲突, tagName 不同), 让 splitter 6 段切片仍能识别 semi-dynamic-1/2 段.
// 字面量必须与 aicache.semiBoundaryTagName / semi2BoundaryTagName 严格一致.
//
// 关键词: buildTaggedPromptSections, 6 段拼接, AI_CACHE_FROZEN, AI_CACHE_SEMI,
//
//	AI_CACHE_SEMI2, aicache hijacker, 5 段切分, P1.1 三 cache 边界, semi 拆两条 message
func buildTaggedPromptSections(highStatic string, frozenBlock string, semiDynamic1 string, semiDynamic2 string, timelineOpen string, dynamic string, dynamicNonce string) string {
	return aicommon.BuildTaggedPromptSectionsWithSectionNamesAndForce(
		highStatic,
		frozenBlock,
		semiDynamic1,
		promptSectionSemiDynamic1,
		true,
		semiDynamic2,
		promptSectionSemiDynamic2,
		timelineOpen,
		dynamic,
		dynamicNonce,
	)
}
