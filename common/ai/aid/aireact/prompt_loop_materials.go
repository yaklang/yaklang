package aireact

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

//go:embed prompts/loop/high_static_section.txt
var loopHighStaticSectionTemplate string

//go:embed prompts/loop/semi_dynamic_section.txt
var loopSemiDynamicSectionTemplate string

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
	materials.Timeline = pm.timelineDumpForPrompt()

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

	highStaticData := pm.buildLoopPromptSectionData(base, input)
	semiDynamicData := pm.buildLoopPromptSectionData(base, input)
	timelineData := pm.buildLoopPromptSectionData(base, input)
	dynamicData := pm.buildLoopPromptSectionData(base, input)

	highStatic, err := pm.executeTemplate("loop-high-static", loopHighStaticSectionTemplate, highStaticData)
	if err != nil {
		return nil, err
	}
	semiDynamic, err := pm.executeTemplate("loop-semi-dynamic", loopSemiDynamicSectionTemplate, semiDynamicData)
	if err != nil {
		return nil, err
	}
	timeline, err := pm.executeTemplate("loop-timeline", loopTimelineSectionTemplate, timelineData)
	if err != nil {
		return nil, err
	}
	dynamic, err := pm.executeTemplate("loop-dynamic", loopDynamicSectionTemplate, dynamicData)
	if err != nil {
		return nil, err
	}

	sections := pm.buildLoopPromptObservations(base, input, highStatic, semiDynamic, timeline, dynamic)

	prompt := joinPromptSections(highStatic, semiDynamic, timeline, dynamic)
	return &reactloops.LoopPromptAssemblyResult{
		Prompt:   prompt,
		Sections: sections,
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

func (pm *PromptManager) buildLoopPromptObservations(
	base *reactloops.LoopPromptBaseMaterials,
	input *reactloops.LoopPromptAssemblyInput,
	highStatic string,
	semiDynamic string,
	timeline string,
	dynamic string,
) []*reactloops.PromptSectionObservation {
	sections := []*reactloops.PromptSectionObservation{
		pm.buildHighStaticObservation(base, input, highStatic),
		pm.buildSemiDynamicObservation(base, input, semiDynamic),
		pm.buildTimelineObservation(base, timeline),
		pm.buildDynamicObservation(base, input, dynamic),
	}
	var result []*reactloops.PromptSectionObservation
	for _, section := range sections {
		if section != nil {
			result = append(result, section)
		}
	}
	return result
}

func (pm *PromptManager) buildHighStaticObservation(
	base *reactloops.LoopPromptBaseMaterials,
	input *reactloops.LoopPromptAssemblyInput,
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
			pm.renderHighStaticPreamble(base, input),
		),
		reactloops.NewPromptSectionObservation(
			"section.high_static.task_instruction",
			"Highly Static / Task Instruction",
			reactloops.PromptSectionRoleSystemPrompt,
			false,
			renderStaticTaggedBlock("PERSISTENT", input.TaskInstruction),
		),
		reactloops.NewPromptSectionObservation(
			"section.high_static.output_example",
			"Highly Static / Output Example",
			reactloops.PromptSectionRoleSystemPrompt,
			false,
			renderStaticTaggedBlock("OUTPUT_EXAMPLE", input.OutputExample),
		),
		reactloops.NewPromptSectionObservation(
			"section.high_static.schema",
			"Highly Static / Schema",
			reactloops.PromptSectionRoleSystemPrompt,
			false,
			renderSchemaBlock(input.Schema),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

func (pm *PromptManager) buildSemiDynamicObservation(
	base *reactloops.LoopPromptBaseMaterials,
	input *reactloops.LoopPromptAssemblyInput,
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
			input.SkillsContext,
		),
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic.tool_inventory",
			"Semi Dynamic / Tool Inventory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderToolInventoryBlock(base),
		),
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic.forge_inventory",
			"Semi Dynamic / Forge Inventory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderForgeInventoryBlock(base),
		),
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

func (pm *PromptManager) buildTimelineObservation(
	base *reactloops.LoopPromptBaseMaterials,
	rendered string,
) *reactloops.PromptSectionObservation {
	section := reactloops.NewPromptContainerSection(
		"section.timeline",
		"Timeline & Workspace",
		reactloops.PromptSectionRoleRuntimeCtx,
	)
	children := []*reactloops.PromptSectionObservation{
		reactloops.NewPromptSectionObservation(
			"section.timeline.timeline",
			"Timeline / Timeline Memory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderTimelineBlock(base),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline.current_time",
			"Timeline / Current Time",
			reactloops.PromptSectionRoleRuntimeCtx,
			false,
			renderCurrentTimeBlock(base),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline.workspace",
			"Timeline / Workspace",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderWorkspaceBlock(base),
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
		reactloops.NewPromptSectionObservation(
			"section.dynamic.user_history",
			"Pure Dynamic / User History",
			reactloops.PromptSectionRoleUserInput,
			false,
			base.UserHistory,
		),
		reactloops.NewPromptSectionObservation(
			"section.dynamic.extra_capabilities",
			"Pure Dynamic / Extra Capabilities",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderTaggedBlock("EXTRA_CAPABILITIES", input.Nonce, input.ExtraCapabilities),
		),
		reactloops.NewPromptSectionObservation(
			"section.dynamic.session_evidence",
			"Pure Dynamic / Session Evidence",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			input.SessionEvidence,
		),
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

func (pm *PromptManager) renderHighStaticPreamble(base *reactloops.LoopPromptBaseMaterials, input *reactloops.LoopPromptAssemblyInput) string {
	data := pm.buildLoopPromptSectionData(base, &reactloops.LoopPromptAssemblyInput{Nonce: input.Nonce})
	data["TaskInstruction"] = ""
	data["OutputExample"] = ""
	data["Schema"] = ""
	rendered, err := pm.executeTemplate("loop-high-static-preamble", loopHighStaticSectionTemplate, data)
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

func renderWorkspaceBlock(base *reactloops.LoopPromptBaseMaterials) string {
	if base == nil {
		return ""
	}
	var lines []string
	if strings.TrimSpace(base.OSArch) == "" && strings.TrimSpace(base.WorkingDir) == "" && strings.TrimSpace(base.WorkingDirGlance) == "" {
		return ""
	}
	lines = append(lines, "# Workspace Context")
	if base.OSArch != "" {
		lines = append(lines, "OS/Arch: "+base.OSArch)
	}
	if base.WorkingDir != "" {
		lines = append(lines, "working dir: "+base.WorkingDir)
	}
	if base.WorkingDirGlance != "" {
		lines = append(lines, "working dir glance: "+base.WorkingDirGlance)
	}
	return strings.Join(lines, "\n")
}

func renderToolInventoryBlock(base *reactloops.LoopPromptBaseMaterials) string {
	if base == nil || !base.AllowToolCall || base.ToolsCount <= 0 || len(base.TopTools) == 0 {
		return ""
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("# Tool Inventory\nYou have access to %d built-in tools. Top %d tools:", base.ToolsCount, base.TopToolsCount))
	for _, tool := range base.TopTools {
		if tool == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("* `%s`: %s", tool.Name, tool.Description))
	}
	if base.HasMoreTools {
		lines = append(lines, "... use `search_capabilities` to discover more tools and related capabilities.")
	}
	return strings.Join(lines, "\n")
}

func renderForgeInventoryBlock(base *reactloops.LoopPromptBaseMaterials) string {
	if base == nil || !base.ShowForgeInventory || strings.TrimSpace(base.AIForgeList) == "" {
		return ""
	}
	return "# AI Blueprint Inventory\n以下是当前可直接调用的 AI 蓝图列表：\n" + base.AIForgeList
}

func renderTimelineBlock(base *reactloops.LoopPromptBaseMaterials) string {
	if base == nil || strings.TrimSpace(base.Timeline) == "" {
		return ""
	}
	return "# Timeline Memory\n" + base.Timeline
}

func renderCurrentTimeBlock(base *reactloops.LoopPromptBaseMaterials) string {
	if base == nil || strings.TrimSpace(base.CurrentTime) == "" {
		return ""
	}
	return "# Current Time\n" + base.CurrentTime
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
