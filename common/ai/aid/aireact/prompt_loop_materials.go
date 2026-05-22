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

	// Timeline frozen/open 与 Session Artifacts frozen/open 必须共享同一轮
	// FrozenTimeUnix；midterm recall 是 turn 级 open prefix，在统一切分之后追加。
	// 关键词: BuildPromptFrozenOpenMaterials, artifacts frozen time, midterm open
	frozenOpen := aicommon.BuildPromptFrozenOpenMaterials(pm.react.config, nonce)
	frozenOpen.TimelineOpen = prependMidtermTimelinePrefixForPrompt(pm.react, frozenOpen.TimelineOpen)
	materials.PromptFrozenOpenMaterials = frozenOpen

	allowPlanAndExec := pm.react.config.GetEnablePlanAndExec() && pm.react.GetCurrentPlanExecutionTask() == nil
	allowToolCall := true
	hasLoadCapability := false

	tools = aicommon.ResolvePromptCandidateTools(pm.react.config, tools)

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

	var scenarioWL []string
	if currentLoop := pm.react.GetCurrentLoop(); currentLoop != nil {
		scenarioWL = currentLoop.GetScenarioToolWhitelist()
	}
	selection := aicommon.ResolvePromptToolInventory(pm.react.config, tools, scenarioWL, allowToolCall)
	if len(selection.VisibleTools) == 0 {
		return materials, nil
	}
	materials.ToolsCount = len(selection.VisibleTools)
	materials.TopTools = selection.DisplayTools
	materials.TopToolsCount = len(selection.DisplayTools)
	materials.HasMoreTools = selection.MoreToolsCount() > 0
	materials.MoreToolsCount = selection.MoreToolsCount()

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
		materials.MoreToolsCount = base.MoreToolsCount
		materials.ForgeInventory = base.ShowForgeInventory && strings.TrimSpace(base.AIForgeList) != ""
		materials.AIForgeList = base.AIForgeList

		aicommon.ApplyPromptFrozenOpenMaterials(materials, base.PromptFrozenOpenMaterials)
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
		if strings.TrimSpace(materials.SessionEvidenceOpen) == "" {
			materials.SessionEvidenceOpen = input.SessionEvidence
		}
		// 全局 TODO 块: 与 SessionEvidence 平行透传, 物理位置在 timeline-open 段
		// section.timeline_open.todo_list (在 session_evidence 之后), 让 loop
		// prompt 任何一次 iteration 都能看到当前 TODO 全貌.
		// 关键词: TodoSnapshot 透传, timeline-open, SessionEvidence 之后
		materials.TodoSnapshot = input.TodoSnapshot
		// CACHE_TOOL_CALL 块从 dynamic/REFLECTION 迁到 semi-dynamic 段
		// 关键词: RecentToolsCache 透传, semi-dynamic 段
		materials.RecentToolsCache = input.RecentToolsCache
		// PE-TASK PLAN 产物 (PARENT_TASK + CURRENT_TASK + INSTRUCTION) 通过
		// FrozenUserContext 字段透传, 渲染时位于 timeline-open 段最末尾
		// (UserHistory 之后), 落在所有 cache 边界之外。早期版本曾尝试
		// frozen-block / semi-dynamic, 但子任务切换会让 PlanContext 内容
		// 抖动, 破坏上游缓存命中, 现采用"放弃自身缓存, 保护上游缓存"策略。
		// 关键词: FrozenUserContext 透传, PLAN_CONTEXT, timeline-open 末尾,
		//        缓存边界外
		materials.FrozenUserContext = input.FrozenUserContext
		materials.FrozenPartitions = append(materials.FrozenPartitions, input.FrozenPartitions...)
	}
	if base != nil {
		// UserHistory 来自 LoopPromptBaseMaterials (config.FormatUserInputHistoryAITag)
		materials.UserHistory = base.UserHistory
	}

	materials.FrozenPartitions = aicommon.NormalizeFrozenBlockPartitions(materials.FrozenPartitions)
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
		Sections:     filtered,
	}, nil
}

func (pm *PromptManager) buildLoopPromptSectionData(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) map[string]any {
	data := map[string]any{
		"Nonce":                   "",
		"UserQuery":               "",
		"TaskInstruction":         "",
		"OutputExample":           "",
		"Schema":                  "",
		"SkillsContext":           "",
		"ExtraCapabilities":       "",
		"SessionEvidence":         "",
		"TodoSnapshot":            "",
		"ReactiveData":            "",
		"InjectedMemory":          "",
		"AllowPlanAndExec":        false,
		"AllowToolCall":           false,
		"HasLoadCapability":       false,
		"ShowForgeInventory":      false,
		"CurrentTime":             "",
		"OSArch":                  "",
		"WorkingDir":              "",
		"WorkingDirGlance":        "",
		"SessionArtifactsListing": "",
		"SessionArtifactsFrozen":  "",
		"SessionArtifactsOpen":    "",
		"Workspace":               false,
		"AutoContext":             "",
		"UserHistory":             "",
		"ToolsCount":              0,
		"TopToolsCount":           0,
		"TopTools":                []*aitool.Tool{},
		"HasMoreTools":            false,
		"ToolInventory":           false,
		"AIForgeList":             "",
		"ForgeInventory":          false,
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
		data["SessionArtifactsFrozen"] = base.SessionArtifactsFrozen
		data["SessionArtifactsOpen"] = base.SessionArtifactsOpen
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
		data["TodoSnapshot"] = input.TodoSnapshot
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
	}
	var partitions []aicommon.FrozenBlockPartition
	if materials != nil {
		partitions = aicommon.NormalizeFrozenBlockPartitions(materials.FrozenPartitions)
	}
	for _, partition := range partitions {
		label := partition.Title
		if strings.TrimSpace(label) == "" {
			label = partition.ID
		}
		children = append(children, reactloops.NewPromptSectionObservation(
			"section.frozen_block.partition."+partition.ID,
			label,
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderFrozenPartitionBlock(partition),
		))
	}
	children = append(children,
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.session_artifacts_frozen",
			"Session Artifacts (Frozen)",
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderSessionArtifactsFrozenBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.session_evidence_frozen",
			"Session Evidence (Frozen)",
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderSessionEvidenceFrozenBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.timeline_frozen",
			"Timeline (Frozen Prefix)",
			reactloops.PromptSectionRoleFrozenBlock,
			true,
			renderTimelineFrozenBlock(materials),
		),
	)
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
// 是因为 PlanContext 内容会随 PE-TASK 子任务切换抖动 (CURRENT_TASK 内容变化)。
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
// PlanContext 已彻底迁出本段 (历史曾位于 semi-dynamic 段, 但子任务切换会让
// 内容抖动并破坏 AI_CACHE_SEMI 命中; 现已迁到 timeline-open 段末尾,
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
// Timeline 末桶 (+ midterm 检索结果) + SessionEvidence + TodoSnapshot + Workspace +
// SessionArtifactsOpen + UserHistory + Current Time + PlanContext (末尾)。
//
// 段内排序原则 (P1-C3 调整):
//  1. Timeline (Open Tail) 在最前: 时间线最末桶是模型理解"刚发生了什么"的
//     首要信息源, 顶到段首让 LLM 第一时间看到。
//  2. Session Evidence 紧跟其后: SESSION_ARTIFACTS 是 Config 级持久化观测
//     (跨 turn 累积的工件证据), 与 Timeline 末桶共同构成"会话级实证"语料,
//     物理上贴近 Timeline 让两者形成连续语义块。
//  3. TodoSnapshot 紧跟 SessionEvidence, 暴露全局待办状态。
//  4. Workspace 居中: OS/Arch + working dir + glance 是相对静态的环境标识,
//     既不属于"刚发生", 也不属于"用户视角", 居中过渡。
//  5. Session Artifacts (Open) 在 Workspace 之后: 最近 task group 与 root files
//     仍属易变尾段, 不污染 frozen prefix。
//  6. User History 在 Artifacts 之后: PREV_USER_INPUT 是用户历史输入轨迹,
//     与 Current Time 一起构成"时序前缀", 紧贴当前时间。
//  7. Current Time 紧跟 User History: 当前时间是最末稳定的时序锚点, 放在
//     User History 之后形成"历史输入 -> 现在"的时间递进, 同时与下方
//     PlanContext (任务规划) 形成"时间 -> 任务"的语义衔接。
//  8. Plan Context 末尾: PE-TASK PLAN 产物本质易变 (子任务切换),
//     放最末让其落在所有 cache
//     边界外, 不污染上游 system / frozen / semi 三段缓存命中率。
//
// timeline-open 整段位于 system / frozen / semi 三段缓存之外, 是 prompt 的
// "易变尾段", 段内子块顺序不影响上游 prefix cache, 仅影响 LLM 理解顺序。
//
// 关键词: buildTimelineOpenObservation, Timeline 末桶, SessionEvidence,
//
//	Workspace, SessionArtifactsOpen, UserHistory, Current Time, PlanContext 末尾,
//	段内排序原则, P1-C3 顺序调整, 缓存边界外
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
	//
	// 子节点排列顺序: timeline_open -> session_evidence -> todo_list -> workspace ->
	// session_artifacts_open -> user_history -> current_time -> plan_context. 该顺序与 timeline_open_section.txt
	// 模板渲染顺序严格一致, 让"上下文成分"面板看到的层级与实际 prompt 字节
	// 流顺序保持同步.
	// 关键词: P1-C3 子节点顺序, observation 与模板对齐
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.timeline_open",
			"Timeline (Open Tail)",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			renderTimelineOpenBlock(materials),
		),
		// P1-C3: SessionEvidence 紧跟 Timeline (Open Tail), 与时间线末桶
		// 形成"会话级实证"连续块.
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.session_evidence",
			"Session Evidence",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			materials.SessionEvidence,
		),
		// 全局 TODO 块: 紧跟 SessionEvidence, 让 loop prompt 始终能看到当前
		// TODO 列表; 数据来源是 SessionPromptState.VerificationTodoStore,
		// 由 VerifyUserSatisfaction 通过 ApplyVerificationTodoOps 增量写入.
		// 段位仍属 timeline-open, 落在所有 cache 边界外, 不污染上游 prefix cache.
		// 关键词: section.timeline_open.todo_list, 全局 TODO, SessionEvidence 之后
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.todo_list",
			"Todo List",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			materials.TodoSnapshot,
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.workspace",
			"Workspace",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			renderWorkspaceBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.session_artifacts_open",
			"Session Artifacts (Open)",
			reactloops.PromptSectionRoleTimelineOpen,
			true,
			renderSessionArtifactsOpenBlock(materials),
		),
		// P1-C3: UserHistory 在 Workspace 之后, 与下方 Current Time 共同
		// 构成"用户输入历史 -> 现在"的时序前缀.
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.user_history",
			"User History",
			reactloops.PromptSectionRoleTimelineOpen,
			false,
			materials.UserHistory,
		),
		// P1-C3: Current Time 紧跟 User History, 充当时序末端锚点;
		// 同时与下方 PlanContext (任务规划) 形成"现在 -> 任务"语义衔接.
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.current_time",
			"Current Time",
			reactloops.PromptSectionRoleTimelineOpen,
			false,
			renderCurrentTimeBlock(materials),
		),
		// PlanContext (PE-TASK PLAN 产物) 末尾注入: 该字段仅 PE-TASK 子任务
		// 非空, 内容随子任务切换抖动, 不适合放任何 cache 边界内。
		// 放 timeline-open 段最末让其落在所有
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

// renderWorkspaceBlock 渲染 timeline-open 段中 Workspace 子块.
//
// SessionArtifacts 已迁出 Workspace，作为 SessionArtifactsFrozen /
// SessionArtifactsOpen 一级块渲染。Workspace 只保留 OS / working dir / glance。
//
// 关键词: renderWorkspaceBlock, Workspace 不再内嵌 Session Artifacts
func renderWorkspaceBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil {
		return ""
	}
	hasEnv := strings.TrimSpace(materials.OSArch) != "" ||
		strings.TrimSpace(materials.WorkingDir) != "" ||
		strings.TrimSpace(materials.WorkingDirGlance) != ""
	if !materials.Workspace || !hasEnv {
		return ""
	}
	var lines []string
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

func renderFrozenPartitionBlock(partition aicommon.FrozenBlockPartition) string {
	partition.Content = strings.TrimSpace(partition.Content)
	if partition.Content == "" {
		return ""
	}
	partition.ID = aicommon.NormalizeFrozenPartitionID(partition.ID)
	partition.Title = strings.TrimSpace(partition.Title)
	if partition.Title == "" {
		partition.Title = partition.ID
	}
	partition.Nonce = strings.TrimSpace(partition.Nonce)
	if partition.Nonce == "" {
		partition.Nonce = aicommon.StablePromptNonce("frozen-partition", partition.ID, partition.Content)
	}
	return fmt.Sprintf("# %s\n<|FROZEN_PARTITION_%s_%s|>\n%s\n<|FROZEN_PARTITION_END_%s_%s|>",
		partition.Title,
		partition.ID,
		partition.Nonce,
		partition.Content,
		partition.ID,
		partition.Nonce,
	)
}

func renderSessionArtifactsFrozenBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.SessionArtifactsFrozen) == "" {
		return ""
	}
	return "# Session Artifacts (Frozen)\n" + materials.SessionArtifactsFrozen
}

func renderSessionArtifactsOpenBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.SessionArtifactsOpen) == "" {
		return ""
	}
	return "# Session Artifacts (Open)\n" + materials.SessionArtifactsOpen
}

func renderSessionEvidenceFrozenBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || strings.TrimSpace(materials.SessionEvidenceFrozen) == "" {
		return ""
	}
	return "# Session Evidence (Frozen)\n" + materials.SessionEvidenceFrozen
}

// renderToolInventoryBlock 是给 observation 树 (UI / 调试) 用的镜像渲染, 必须
// 与 frozen_block_section.txt 模板保持字节级一致, 否则面板里看到的与 LLM 真正
// 收到的会错位. 任何模板改动都要同步本函数, 反之亦然.
// 关键词: renderToolInventoryBlock, observation 镜像, frozen_block_section 对齐
func renderToolInventoryBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || !materials.ToolInventory || materials.ToolsCount <= 0 || len(materials.TopTools) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines,
		"# Tool Inventory",
		fmt.Sprintf("You have access to %d built-in tools. Below are %d prioritized entries selected within a token budget:", materials.ToolsCount, materials.TopToolsCount),
		"",
		"## Call Mode (single tool vs tool_compose)",
		"",
		"- 单工具入口: 一次提一个工具 + 参数, 返回后再决策下一步; 探索 / 上游不确定 / 需要逐步收紧时的默认形态.",
		"- 工具编排入口 (tool_compose): 一次提交 >=2 节点的 DAG, 节点间显式串行依赖或天然并行, 由 caller 拓扑跑完后再观察整体结果.",
		"- 选择原则 (与 high-static 段实验准则保持一致):",
		"  - 默认单步. 凡是\"调一步看一眼再定下一步\"的链路一律走单工具, 不要把猜测拼成 DAG.",
		"  - 节点 <2 / 节点之间无依赖且互相独立 / 任务目标尚不明确 / 不可逆动作 -> 走单工具.",
		"  - 已经知道要调哪些工具 + 上游产物喂下游 (硬数据依赖) 或同质多目标 (多 URL / 多文件 / 多参数并行) -> 走 tool_compose, 单次 DAG <=5 节点, 超出拆多轮.",
		"  - DAG 限定在当前 CURRENT-TASK 内, 不跨子任务串接; 探索阶段一律单步, 仅 EXEC 阶段才合法.",
		"",
		"## Prioritized Tools",
	)
	for _, tool := range materials.TopTools {
		if tool == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("* `%s`: %s", tool.Name, tool.Description))
	}
	if materials.HasMoreTools {
		lines = append(lines,
			"",
			fmt.Sprintf("> 还有 %d 个工具未列入上方清单. 不在列表中的工具 / AI 蓝图 / 技能 / Focus 模式, 通过 `search_capabilities` 按关键字检索后再加载使用.", materials.MoreToolsCount),
		)
	}
	return strings.Join(lines, "\n")
}

func renderForgeInventoryBlock(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil || !materials.ForgeInventory || strings.TrimSpace(materials.AIForgeList) == "" {
		return ""
	}
	return "# AI Blueprint Inventory\n以下是当前可直接调用的 AI 蓝图列表：\n" + materials.AIForgeList
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
