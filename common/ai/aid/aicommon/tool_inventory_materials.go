package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// inventoryPriorityToolNames 是 Tool Inventory 展示优先级名单, 与 capability_inventory fixed 段对齐.
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

type ToolInventoryData struct {
	ToolInventory  bool
	ToolsCount     int
	TopToolsCount  int
	TopTools       []*aitool.Tool
	VisibleTools   []*aitool.Tool
	HasMoreTools   bool
	MoreToolsCount int
}

// IsFixedInventoryTool 判断工具是否属于 capability_inventory fixed 段.
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

func resolveToolInventoryTopCount(cfg *Config) int {
	if cfg != nil && cfg.GetTopToolsCount() > 0 {
		return cfg.GetTopToolsCount()
	}
	return ToolInventoryMinCount
}

func PrioritizeToolsForInventory(tools []*aitool.Tool, maxCount int, extraPriority ...string) []*aitool.Tool {
	if len(tools) == 0 || maxCount <= 0 {
		return nil
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

	result := make([]*aitool.Tool, 0, min(maxCount, len(tools)))
	usedNames := make(map[string]bool, len(tools))
	for _, name := range priorityNames {
		if len(result) >= maxCount {
			break
		}
		if tool, exists := toolMap[name]; exists {
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

func BuildToolInventoryData(tools []*aitool.Tool, topToolsCount int, scenarioWhitelist ...string) ToolInventoryData {
	tools = FilterToolsByVisibility(tools, scenarioWhitelist)
	if len(tools) == 0 {
		return ToolInventoryData{}
	}

	candidate := PrioritizeToolsForInventory(tools, topToolsCount, scenarioWhitelist...)
	display := SelectToolsByTokenBudget(candidate, ToolInventoryTokenBudget, ToolInventoryMinCount)
	more := len(tools) - len(display)
	if more < 0 {
		more = 0
	}

	return ToolInventoryData{
		ToolInventory:  true,
		ToolsCount:     len(tools),
		TopToolsCount:  len(display),
		TopTools:       display,
		VisibleTools:   tools,
		HasMoreTools:   len(tools) > len(display),
		MoreToolsCount: more,
	}
}

func BuildToolInventoryDataFromConfig(config *Config, scenarioWhitelist ...string) (ToolInventoryData, error) {
	if config == nil {
		return ToolInventoryData{}, nil
	}
	toolMgr := config.GetAiToolManager()
	if toolMgr == nil {
		return ToolInventoryData{}, nil
	}
	tools, err := toolMgr.GetEnableTools()
	if err != nil {
		return ToolInventoryData{}, err
	}
	return BuildToolInventoryData(tools, resolveToolInventoryTopCount(config), scenarioWhitelist...), nil
}

func ApplyToolInventoryData(materials *PromptMaterials, data ToolInventoryData) {
	if materials == nil {
		return
	}
	materials.ToolInventory = data.ToolInventory
	materials.ToolsCount = data.ToolsCount
	materials.TopToolsCount = data.TopToolsCount
	materials.TopTools = data.TopTools
	materials.HasMoreTools = data.HasMoreTools
	materials.MoreToolsCount = data.MoreToolsCount
}

func PopulateToolInventoryFromConfig(materials *PromptMaterials, config *Config, scenarioWhitelist ...string) error {
	data, err := BuildToolInventoryDataFromConfig(config, scenarioWhitelist...)
	if err != nil {
		return err
	}
	ApplyToolInventoryData(materials, data)
	return nil
}
