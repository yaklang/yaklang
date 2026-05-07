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
	promptSectionTagName     = "PROMPT_SECTION"
	promptSectionHighStatic  = "high-static"
	promptSectionSemiDynamic = "semi-dynamic"
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

//go:embed prompts/loop/high_static_section.txt
var loopHighStaticSectionTemplate string

//go:embed prompts/loop/semi_dynamic_section.txt
var loopSemiDynamicSectionTemplate string

//go:embed prompts/loop/timeline_section.txt
var loopTimelineSectionTemplate string

//go:embed prompts/loop/frozen_block_section.txt
var loopFrozenBlockSectionTemplate string

//go:embed prompts/loop/timeline_open_section.txt
var loopTimelineOpenSectionTemplate string

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

func (r *ReAct) NewPromptPrefixMaterials(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) *reactloops.PromptPrefixMaterials {
	if r == nil || r.promptManager == nil {
		return nil
	}
	return r.promptManager.NewPromptPrefixMaterials(base, input)
}

func (r *ReAct) AssemblePromptPrefix(materials *reactloops.PromptPrefixMaterials) (*reactloops.PromptPrefixAssemblyResult, error) {
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

	prefixMaterials := pm.NewPromptPrefixMaterials(base, input)
	prefix, err := pm.AssemblePromptPrefix(prefixMaterials)
	if err != nil {
		return nil, err
	}
	dynamicData := pm.buildLoopPromptSectionData(base, input)
	dynamic, err := pm.renderLoopDynamicSection(dynamicData)
	if err != nil {
		return nil, err
	}

	sections := append([]*reactloops.PromptSectionObservation{}, prefix.Sections...)
	if dynamicSection := pm.buildDynamicObservation(base, input, dynamic); dynamicSection != nil {
		sections = append(sections, dynamicSection)
	}

	prompt := buildTaggedPromptSections(prefix.HighStatic, prefix.FrozenBlock, prefix.SemiDynamic, prefix.TimelineOpen, dynamic, base.Nonce)
	return &reactloops.LoopPromptAssemblyResult{
		Prompt:   prompt,
		Sections: sections,
	}, nil
}

func (pm *PromptManager) NewPromptPrefixMaterials(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) *reactloops.PromptPrefixMaterials {
	materials := &reactloops.PromptPrefixMaterials{}

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
		// PE-TASK 缓存优化: PLAN 产物 (PARENT_TASK + CURRENT_TASK + INSTRUCTION)
		// 通过 FrozenUserContext 字段从 dynamic 段迁到 frozen-block.
		// 关键词: FrozenUserContext 透传, PLAN_CONTEXT, frozen-block
		materials.FrozenUserContext = input.FrozenUserContext
	}
	if base != nil {
		// UserHistory 来自 LoopPromptBaseMaterials (config.FormatUserInputHistoryAITag)
		materials.UserHistory = base.UserHistory
	}

	return materials
}

// AssemblePromptPrefix 按"稳定性分层"路径输出 4 段: HighStatic | FrozenBlock |
// SemiDynamic (Skills + Schema 残留) | TimelineOpen。Prompt 字段是 4 段拼接结果,
// 调用方拼上 Dynamic 段后形成完整 prompt。
//
// FrozenBlock 段 (Tool Inventory + Forge Inventory + Timeline-frozen) 整体字节稳定,
// 由 buildTaggedPromptSections 用 <|AI_CACHE_FROZEN_semi-dynamic|>...
// <|AI_CACHE_FROZEN_END_semi-dynamic|> 标签包裹, 供 aicache hijacker
// splitByFrozenBoundary 精准切片 user1 (frozen prefix) / user2 (open tail)。
//
// 兼容字段 Timeline: 等于老 timeline 段 (frozen + open + workspace + current time)
// 的合并渲染, 仅供老 caller / 测试断言使用; 新路径不读取它。
//
// 关键词: AssemblePromptPrefix, 4 段拼接, 按稳定性分层, AI_CACHE_FROZEN
func (pm *PromptManager) AssemblePromptPrefix(materials *reactloops.PromptPrefixMaterials) (*reactloops.PromptPrefixAssemblyResult, error) {
	if pm == nil {
		return nil, fmt.Errorf("prompt manager is nil")
	}
	if materials == nil {
		return nil, fmt.Errorf("prompt prefix materials is nil")
	}

	highStatic, err := pm.renderLoopHighStaticSection(materials)
	if err != nil {
		return nil, err
	}
	frozenBlock, err := pm.renderLoopFrozenBlockSection(materials)
	if err != nil {
		return nil, err
	}
	semiDynamic, err := pm.renderLoopSemiDynamicSection(materials)
	if err != nil {
		return nil, err
	}
	timelineOpen, err := pm.renderLoopTimelineOpenSection(materials)
	if err != nil {
		return nil, err
	}

	// 老 Timeline 段渲染保留, 仅写入 PromptPrefixAssemblyResult.Timeline 供观测/兼容;
	// 不进入新路径的 Prompt 拼接。
	legacyTimeline, err := pm.renderLoopTimelineSection(materials)
	if err != nil {
		return nil, err
	}

	sections := []*reactloops.PromptSectionObservation{
		pm.buildHighStaticObservation(materials, highStatic),
		pm.buildFrozenBlockObservation(materials, frozenBlock),
		pm.buildSemiDynamicResidualObservation(materials, semiDynamic),
		pm.buildTimelineOpenObservation(materials, timelineOpen),
	}
	var filtered []*reactloops.PromptSectionObservation
	for _, section := range sections {
		if section != nil {
			filtered = append(filtered, section)
		}
	}

	return &reactloops.PromptPrefixAssemblyResult{
		Prompt:       joinPromptSections(highStatic, frozenBlock, semiDynamic, timelineOpen),
		HighStatic:   highStatic,
		FrozenBlock:  frozenBlock,
		SemiDynamic:  semiDynamic,
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
		reactloops.PromptSectionRoleSystemPrompt,
	)
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.high_static.static_preamble",
			"Highly Static / Traits & Agent Systems",
			reactloops.PromptSectionRoleSystemPrompt,
			false,
			pm.renderHighStaticPreamble(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.high_static.task_instruction",
			"Highly Static / Task Instruction",
			reactloops.PromptSectionRoleSystemPrompt,
			false,
			renderStaticTaggedBlock("PERSISTENT", materials.TaskInstruction),
		),
		reactloops.NewPromptSectionObservation(
			"section.high_static.output_example",
			"Highly Static / Output Example",
			reactloops.PromptSectionRoleSystemPrompt,
			false,
			renderStaticTaggedBlock("OUTPUT_EXAMPLE", materials.OutputExample),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

// buildFrozenBlockObservation 给"AI_CACHE_FROZEN 块"做观测树:
// Tool Inventory + Forge Inventory + (PE-TASK only) Plan Context +
// Timeline-frozen (reducer + 非末 interval)。
//
// Plan Context 段位置在 Tool/Forge 之后、Timeline-frozen 之前: Tool/Forge 是
// 整个 root 任务生命周期都不变的"系统级"内容, 应当排在最前; Plan Context
// 在同一 plan 周期内字节稳定, 排在中间; Timeline-frozen 随时间轴增长可能
// 间歇性扩展 (新一段 reducer 块产生时), 排在最后, 让前两段的前缀缓存更稳定。
//
// 关键词: buildFrozenBlockObservation, Tool/Forge/PlanContext/Timeline-frozen,
//        AI_CACHE_FROZEN, PE-TASK frozen 注入位置
func (pm *PromptManager) buildFrozenBlockObservation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.frozen_block",
		"Frozen Block",
		reactloops.PromptSectionRoleRuntimeCtx,
	)
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.tool_inventory",
			"Frozen Block / Tool Inventory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderToolInventoryBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.forge_inventory",
			"Frozen Block / Forge Inventory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderForgeInventoryBlock(materials),
		),
		// PE-TASK 缓存优化: PLAN 产物 (PARENT_TASK + CURRENT_TASK +
		// INSTRUCTION + 父链 FACTS/DOCUMENT) 从 dynamic 段迁到 frozen-block,
		// 用 plan-scoped stable nonce 包装, 跨同一 plan 周期字节稳定。
		// 关键词: section.frozen_block.plan_context, PLAN_CONTEXT 段
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.plan_context",
			"Frozen Block / Plan Context (PE-TASK PLAN Output)",
			reactloops.PromptSectionRoleUserInput,
			true,
			renderPlanContextBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.frozen_block.timeline_frozen",
			"Frozen Block / Timeline (Frozen Prefix)",
			reactloops.PromptSectionRoleRuntimeCtx,
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
// 需要给 frozen-block splitter 一个稳定的边界标记, 与内层标签命名空间互不冲突。
//
// 关键词: renderPlanContextBlock, PLAN_CONTEXT, plan-scoped nonce,
//        frozen-block 边界
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

// buildSemiDynamicResidualObservation 给"PROMPT_SECTION_semi-dynamic 残留段"做观测树:
// Skills Context + Schema + Cache Tool Call. Tool/Forge 已迁出到 FrozenBlock,
// CACHE_TOOL_CALL 从 dynamic/REFLECTION 迁入此段 (用稳定 nonce 渲染, 字节稳定).
//
// 关键词: buildSemiDynamicResidualObservation, Skills Context, Schema, Cache Tool Call
func (pm *PromptManager) buildSemiDynamicResidualObservation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.semi_dynamic",
		"Semi Dynamic",
		reactloops.PromptSectionRoleRuntimeCtx,
	)
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic.skills_context",
			"Semi Dynamic / Skills Context",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			materials.SkillsContext,
		),
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic.schema",
			"Semi Dynamic / Schema",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderSchemaBlock(materials.Schema),
		),
		// CACHE_TOOL_CALL 块从 dynamic/REFLECTION 迁到此处, 用稳定 nonce 渲染.
		// 关键词: Semi Dynamic / Cache Tool Call, RecentToolsCache 观测节点
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic.cache_tool_call",
			"Semi Dynamic / Cache Tool Call",
			reactloops.PromptSectionRoleRuntimeCtx,
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

// buildTimelineOpenObservation 给"PROMPT_SECTION_timeline-open 段"做观测树:
// Timeline 末桶 (+ midterm 检索结果) + Current Time + Workspace。
//
// 关键词: buildTimelineOpenObservation, Timeline 末桶, Current Time, Workspace
func (pm *PromptManager) buildTimelineOpenObservation(
	materials *reactloops.PromptPrefixMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.timeline_open",
		"Timeline Open & Workspace",
		reactloops.PromptSectionRoleRuntimeCtx,
	)
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.timeline_open",
			"Timeline Open / Timeline (Open Tail)",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderTimelineOpenBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.current_time",
			"Timeline Open / Current Time",
			reactloops.PromptSectionRoleRuntimeCtx,
			false,
			renderCurrentTimeBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.workspace",
			"Timeline Open / Workspace",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderWorkspaceBlock(materials),
		),
		// P1-C2: SessionEvidence (SESSION_ARTIFACTS) 上移到 timeline-open
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.session_evidence",
			"Timeline Open / Session Evidence",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			materials.SessionEvidence,
		),
		// P1-C2: UserHistory (PREV_USER_INPUT) 上移到 timeline-open
		reactloops.NewPromptSectionObservation(
			"section.timeline_open.user_history",
			"Timeline Open / User History",
			reactloops.PromptSectionRoleUserInput,
			false,
			materials.UserHistory,
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
		reactloops.PromptSectionRoleMixed,
	)
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.dynamic.user_query",
			"Pure Dynamic / User Query",
			reactloops.PromptSectionRoleUserInput,
			false,
			renderUserQueryBlock(input.Nonce, input.UserQuery),
		),
		reactloops.NewPromptSectionObservation(
			"section.dynamic.auto_context",
			"Pure Dynamic / Auto Context",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			base.AutoContext,
		),
		// P1-C2: user_history 已上移到 section.timeline_open.user_history,
		// 此处 dynamic 段不再渲染 PREV_USER_INPUT.
		reactloops.NewPromptSectionObservation(
			"section.dynamic.extra_capabilities",
			"Pure Dynamic / Extra Capabilities",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderTaggedBlock("EXTRA_CAPABILITIES", input.Nonce, input.ExtraCapabilities),
		),
		// P1-C2: session_evidence 已上移到 section.timeline_open.session_evidence,
		// 此处 dynamic 段不再渲染 SESSION_ARTIFACTS.
		reactloops.NewPromptSectionObservation(
			"section.dynamic.reactive_data",
			"Pure Dynamic / Reactive Data",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderTaggedBlock("REFLECTION", input.Nonce, input.ReactiveData),
		),
		reactloops.NewPromptSectionObservation(
			"section.dynamic.injected_memory",
			"Pure Dynamic / Injected Memory",
			reactloops.PromptSectionRoleRuntimeCtx,
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

func (pm *PromptManager) renderHighStaticPreamble(materials *reactloops.PromptPrefixMaterials) string {
	if materials == nil {
		return ""
	}
	preamble := materials.HighStaticData()
	preamble["TaskInstruction"] = ""
	preamble["OutputExample"] = ""
	rendered, err := pm.executeTemplate("loop-high-static-preamble", loopHighStaticSectionTemplate, preamble)
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
	return pm.executeTemplate("loop-high-static", loopHighStaticSectionTemplate, materials.HighStaticData())
}

func (pm *PromptManager) renderLoopSemiDynamicSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return pm.executeTemplate("loop-semi-dynamic", loopSemiDynamicSectionTemplate, materials.SemiDynamicData())
}

// renderLoopFrozenBlockSection 渲染"按稳定性分层"路径下的 FrozenBlock 段
// (Tool Inventory + Forge Inventory + Timeline-frozen)。模板中只引用稳定字段。
//
// 关键词: renderLoopFrozenBlockSection, frozen_block_section.txt
func (pm *PromptManager) renderLoopFrozenBlockSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return pm.executeTemplate("loop-frozen-block", loopFrozenBlockSectionTemplate, materials.FrozenBlockData())
}

// renderLoopTimelineOpenSection 渲染"按稳定性分层"路径下的 TimelineOpen 段
// (Timeline 末桶 + Current Time + Workspace)。
//
// 关键词: renderLoopTimelineOpenSection, timeline_open_section.txt
func (pm *PromptManager) renderLoopTimelineOpenSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return pm.executeTemplate("loop-timeline-open", loopTimelineOpenSectionTemplate, materials.TimelineOpenData())
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

// buildTaggedPromptSections 按"按稳定性分层"路径拼接 5 段:
//   SYSTEM (high-static) | FROZEN (frozen-block) | SEMI (semi-dynamic 残留) |
//   OPEN (timeline-open) | DYNAMIC
//
// 各段之间空行分隔, 空段省略。
//
// 双 cache 边界 (P1):
//   - frozen-block 段: <|AI_CACHE_FROZEN_semi-dynamic|>...<|AI_CACHE_FROZEN_END_semi-dynamic|>
//     由 wrapAICacheFrozen 注入, hijacker 切到 user1, 主动打 ephemeral cc
//   - semi-dynamic 段: <|AI_CACHE_SEMI_semi|>...<|AI_CACHE_SEMI_END_semi|>
//     由 wrapAICacheSemi 注入 (覆盖 PROMPT_SECTION_semi-dynamic 整段),
//     hijacker 切到 user2, 主动打 ephemeral cc
//
// 内层 PROMPT_SECTION_semi-dynamic 标签保留 (不会与 AI_CACHE_SEMI 冲突, tagName
// 不同), 让 splitter 4 段切片仍能识别 semi-dynamic 段. 字面量必须与
// aicache.semiBoundaryTagName / semiBoundaryNonce 严格一致.
//
// 关键词: buildTaggedPromptSections, 5 段拼接, AI_CACHE_FROZEN, AI_CACHE_SEMI,
//        aicache hijacker, 4 段切分, 双 cc, P1 双 cache 边界
func buildTaggedPromptSections(highStatic string, frozenBlock string, semiDynamic string, timelineOpen string, dynamic string, dynamicNonce string) string {
	return joinPromptSections(
		wrapPromptMessageSection(promptSectionHighStatic, highStatic, ""),
		wrapAICacheFrozen(frozenBlock),
		wrapAICacheSemi(wrapPromptMessageSection(promptSectionSemiDynamic, semiDynamic, "")),
		wrapPromptMessageSection(promptSectionTimelineOpen, timelineOpen, ""),
		wrapPromptMessageSection(promptSectionDynamic, dynamic, dynamicNonce),
	)
}

// wrapAICacheFrozen 用 <|AI_CACHE_FROZEN_semi-dynamic|>...
// <|AI_CACHE_FROZEN_END_semi-dynamic|> 包裹 frozen-block 内容, 与
// aicommon.TimelineFrozenBoundaryTagName / TimelineFrozenBoundaryNonce 一致,
// 复用 aicache hijacker 既有的 splitByFrozenBoundary 锚点。
//
// 内容为空时返回空串, 调用方 joinPromptSections 会自动跳过空段。
//
// 关键词: wrapAICacheFrozen, AI_CACHE_FROZEN, frozen 边界标签
func wrapAICacheFrozen(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	tagName := aicommon.TimelineFrozenBoundaryTagName
	nonce := aicommon.TimelineFrozenBoundaryNonce
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tagName, nonce, content, tagName, nonce)
}

// wrapAICacheSemi 用 <|AI_CACHE_SEMI_semi|>...<|AI_CACHE_SEMI_END_semi|> 包裹
// 已经包了 PROMPT_SECTION_semi-dynamic 标签的 semi-dynamic 段内容, 字面量与
// aicommon.SemiDynamicCacheBoundaryTagName / SemiDynamicCacheBoundaryNonce 一致,
// 让 aicache hijacker 在 frozen 边界 END 之后通过字符串 IndexOf 精准切到 user2,
// 形成 4 段消息 (system+cc, user1+cc=frozen, user2+cc=semi, user3=open+dynamic).
//
// 注意: 该函数包的是 wrapPromptMessageSection 已经产出的"PROMPT_SECTION_
// semi-dynamic 完整段". 双层 tag 嵌套是有意为之: 内层 PROMPT_SECTION 让 splitter
// 仍能识别段归属, 外层 AI_CACHE_SEMI 给 hijacker 一对字节恒定的边界.
//
// 内容为空时返回空串, 调用方 joinPromptSections 会自动跳过空段.
//
// 关键词: wrapAICacheSemi, AI_CACHE_SEMI, semi cache boundary, 4 段拆分,
//        与 PROMPT_SECTION_semi-dynamic 双层嵌套
func wrapAICacheSemi(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	tagName := aicommon.SemiDynamicCacheBoundaryTagName
	nonce := aicommon.SemiDynamicCacheBoundaryNonce
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tagName, nonce, content, tagName, nonce)
}

func wrapPromptMessageSection(sectionName string, content string, nonce string) string {
	content = strings.TrimSpace(content)
	if sectionName == promptSectionDynamic && nonce != "" {
		tagName := fmt.Sprintf("%s_%s", promptSectionTagName, sectionName)
		return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tagName, nonce, content, tagName, nonce)
	}
	// high-static 段使用 AI_CACHE_SYSTEM tagName，让 aicache 与上游识别为系统级缓存边界
	// 关键词: AI_CACHE_SYSTEM_high-static, aicache hijack
	if sectionName == promptSectionHighStatic {
		return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", aiCacheSystemTagName, sectionName, content, aiCacheSystemTagName, sectionName)
	}
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", promptSectionTagName, sectionName, content, promptSectionTagName, sectionName)
}

func joinPromptSections(parts ...string) string {
	var filtered []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "\n\n")
}
