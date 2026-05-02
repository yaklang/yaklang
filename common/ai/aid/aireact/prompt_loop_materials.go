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

const (
	promptSectionTagName     = "PROMPT_SECTION"
	promptSectionHighStatic  = "high-static"
	promptSectionSemiDynamic = "semi-dynamic"
	promptSectionTimeline    = "timeline"
	promptSectionDynamic     = "dynamic"
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

	prompt := buildTaggedPromptSections(prefix.HighStatic, prefix.SemiDynamic, prefix.Timeline, dynamic, base.Nonce)
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
	}

	return materials
}

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
	semiDynamic, err := pm.renderLoopSemiDynamicSection(materials)
	if err != nil {
		return nil, err
	}
	timeline, err := pm.renderLoopTimelineSection(materials)
	if err != nil {
		return nil, err
	}

	sections := []*reactloops.PromptSectionObservation{
		pm.buildHighStaticObservation(materials, highStatic),
		pm.buildSemiDynamicObservation(materials, semiDynamic),
		pm.buildTimelineObservation(materials, timeline),
	}
	var filtered []*reactloops.PromptSectionObservation
	for _, section := range sections {
		if section != nil {
			filtered = append(filtered, section)
		}
	}

	return &reactloops.PromptPrefixAssemblyResult{
		Prompt:      joinPromptSections(highStatic, semiDynamic, timeline),
		HighStatic:  highStatic,
		SemiDynamic: semiDynamic,
		Timeline:    timeline,
		Sections:    filtered,
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

func (pm *PromptManager) buildSemiDynamicObservation(
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
			"section.semi_dynamic.tool_inventory",
			"Semi Dynamic / Tool Inventory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderToolInventoryBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.semi_dynamic.forge_inventory",
			"Semi Dynamic / Forge Inventory",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderForgeInventoryBlock(materials),
		),
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
	}
	section.Children = filterIncludedPromptSections(children)
	if strings.TrimSpace(rendered) != "" {
		section.Content = ""
	}
	return reactloops.FinalizePromptContainerSection(section)
}

func (pm *PromptManager) buildTimelineObservation(
	materials *reactloops.PromptPrefixMaterials,
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
			renderTimelineBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline.current_time",
			"Timeline / Current Time",
			reactloops.PromptSectionRoleRuntimeCtx,
			false,
			renderCurrentTimeBlock(materials),
		),
		reactloops.NewPromptSectionObservation(
			"section.timeline.workspace",
			"Timeline / Workspace",
			reactloops.PromptSectionRoleRuntimeCtx,
			true,
			renderWorkspaceBlock(materials),
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

func (pm *PromptManager) renderLoopTimelineSection(materials *reactloops.PromptPrefixMaterials) (string, error) {
	return pm.executeTemplate("loop-timeline", loopTimelineSectionTemplate, materials.TimelineData())
}

func (pm *PromptManager) renderLoopDynamicSection(data map[string]any) (string, error) {
	return pm.executeTemplate("loop-dynamic", loopDynamicSectionTemplate, data)
}

func buildTaggedPromptSections(highStatic string, semiDynamic string, timeline string, dynamic string, dynamicNonce string) string {
	return joinPromptSections(
		wrapPromptMessageSection(promptSectionHighStatic, highStatic, ""),
		wrapPromptMessageSection(promptSectionSemiDynamic, semiDynamic, ""),
		wrapPromptMessageSection(promptSectionTimeline, timeline, ""),
		wrapPromptMessageSection(promptSectionDynamic, dynamic, dynamicNonce),
	)
}

func wrapPromptMessageSection(sectionName string, content string, nonce string) string {
	content = strings.TrimSpace(content)
	if sectionName == promptSectionDynamic && nonce != "" {
		tagName := fmt.Sprintf("%s_%s", promptSectionTagName, sectionName)
		return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tagName, nonce, content, tagName, nonce)
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
