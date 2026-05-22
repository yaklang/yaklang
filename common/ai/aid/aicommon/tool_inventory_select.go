package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aitool"

// ToolInventorySelectionInput 与 prompt Tool Inventory 段使用同一套筛选参数.
type ToolInventorySelectionInput struct {
	CandidateTools        []*aitool.Tool
	ScenarioToolWhitelist []string
}

// ToolInventorySelection 是 prompt 构建 Tool Inventory 段的筛选结果.
type ToolInventorySelection struct {
	// VisibleTools 可见性过滤后的完整工具池 (ToolsCount / MoreToolsCount 口径).
	VisibleTools []*aitool.Tool
	// DisplayTools 排序 + token 预算后的展示子集 (TopTools / 能力清单 tools 口径).
	DisplayTools []*aitool.Tool
}

func (s ToolInventorySelection) MoreToolsCount() int {
	more := len(s.VisibleTools) - len(s.DisplayTools)
	if more < 0 {
		return 0
	}
	return more
}

// SelectToolInventoryTools 是 prompt 与 capability_inventory 共用的 Tool Inventory
// 筛选入口, 委托 BuildToolInventoryData 完成可见性过滤、排序与 token 预算裁剪.
func SelectToolInventoryTools(cfg *Config, input ToolInventorySelectionInput) ToolInventorySelection {
	if len(input.CandidateTools) == 0 {
		return ToolInventorySelection{}
	}

	data := BuildToolInventoryData(
		input.CandidateTools,
		resolveToolInventoryTopCount(cfg),
		input.ScenarioToolWhitelist...,
	)
	if !data.ToolInventory {
		return ToolInventorySelection{}
	}
	return ToolInventorySelection{
		VisibleTools: data.VisibleTools,
		DisplayTools: data.TopTools,
	}
}

// ResolveLoopPromptCandidateTools 与 generateLoopPrompt 使用同一候选工具解析:
// loop 的 toolsGetter 有值时用其返回, 否则 fallback 到 GetEnableTools.
func ResolveLoopPromptCandidateTools(cfg *Config, loop CapabilityInventoryLoopContext) []*aitool.Tool {
	var candidates []*aitool.Tool
	if loop != nil {
		candidates = loop.PromptCandidateTools()
	}
	return ResolvePromptCandidateTools(cfg, candidates)
}

// ResolvePromptCandidateTools 与 generateLoopPrompt + GetLoopPromptBaseMaterials
// 使用同一候选工具解析: toolsGetter 结果为空时 fallback 到 GetEnableTools.
func ResolvePromptCandidateTools(cfg *Config, tools []*aitool.Tool) []*aitool.Tool {
	if len(tools) > 0 {
		return tools
	}
	if cfg == nil || cfg.GetAiToolManager() == nil {
		return nil
	}
	enabledTools, err := cfg.GetAiToolManager().GetEnableTools()
	if err != nil {
		return nil
	}
	return enabledTools
}

// ResolvePromptToolInventory 与 GetLoopPromptBaseMaterials 中 Tool Inventory 段一致,
// 受 allowToolCall 控制是否产出展示工具.
func ResolvePromptToolInventory(
	cfg *Config,
	tools []*aitool.Tool,
	scenarioWhitelist []string,
	allowToolCall bool,
) ToolInventorySelection {
	if !allowToolCall || len(tools) == 0 {
		return ToolInventorySelection{}
	}
	return SelectToolInventoryTools(cfg, ToolInventorySelectionInput{
		CandidateTools:        tools,
		ScenarioToolWhitelist: scenarioWhitelist,
	})
}
