package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

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

var inventoryPriorityToolNames = []string{
	"search_capabilities",
	"web_search",
	"grep",
	"read_file",
	"write_file",
	"modify_file",
	"find_file",
	"tree",
	"bash",
	"cmd",
	"encode",
	"decode",
	"auto_decode",
	"scan_port",
	"git-clone",
	"do_http_request",
	"batch_do_http_request",
	"simple_crawler",
	"cybersecurity-risk",
	"brute",
}

// IsFixedInventoryTool 判断工具是否属于 capability_inventory fixed 段.
// fixed 工具与 Tool Inventory 优先级名单 inventoryPriorityToolNames 对齐.
func IsFixedInventoryTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, fixedName := range inventoryPriorityToolNames {
		if fixedName == name {
			return true
		}
	}
	return false
}

// PrioritizeInventoryTools 按 Tool Inventory 展示优先级排序并截断候选池.
func PrioritizeInventoryTools(tools []*aitool.Tool, maxCount int, extraPriority ...string) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}

	priorityNames := inventoryPriorityToolNames
	if len(extraPriority) > 0 {
		seen := make(map[string]bool, len(extraPriority))
		merged := make([]string, 0, len(extraPriority)+len(priorityNames))
		for _, n := range extraPriority {
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			merged = append(merged, n)
		}
		for _, n := range priorityNames {
			if seen[n] {
				continue
			}
			seen[n] = true
			merged = append(merged, n)
		}
		priorityNames = merged
	}

	toolMap := make(map[string]*aitool.Tool, len(tools))
	for _, tool := range tools {
		if tool == nil || tool.Name == "" {
			continue
		}
		toolMap[tool.Name] = tool
	}

	result := make([]*aitool.Tool, 0, len(tools))
	usedNames := make(map[string]bool, len(tools))

	for _, name := range priorityNames {
		if tool, exists := toolMap[name]; exists && len(result) < maxCount {
			result = append(result, tool)
			usedNames[name] = true
		}
	}

	for _, tool := range tools {
		if len(result) >= maxCount {
			break
		}
		if tool == nil || tool.Name == "" || usedNames[tool.Name] {
			continue
		}
		result = append(result, tool)
		usedNames[tool.Name] = true
	}

	return result
}

// SelectToolInventoryTools 是 prompt 与 capability_inventory 共用的 Tool Inventory
// 筛选入口: 可见性过滤 -> 候选池排序 -> token 预算裁剪.
func SelectToolInventoryTools(cfg *Config, input ToolInventorySelectionInput) ToolInventorySelection {
	if len(input.CandidateTools) == 0 {
		return ToolInventorySelection{}
	}

	visible := FilterToolsByVisibility(input.CandidateTools, input.ScenarioToolWhitelist)
	if len(visible) == 0 {
		return ToolInventorySelection{}
	}

	topCount := ToolInventoryMinCount
	if cfg != nil && cfg.GetTopToolsCount() > 0 {
		topCount = cfg.GetTopToolsCount()
	}

	candidate := PrioritizeInventoryTools(visible, topCount, input.ScenarioToolWhitelist...)
	display := SelectToolsByTokenBudget(candidate, ToolInventoryTokenBudget, ToolInventoryMinCount)
	return ToolInventorySelection{
		VisibleTools: visible,
		DisplayTools: display,
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
