package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// getPrioritizedTools returns a prioritized list of tools, with search tools first.
//
// extraPriority 是可变参数 (常见用例: yak focus mode 通过 __SCENARIO_TOOLS__
// dunder 把指定的 scenario 工具拉回 inventory 时, 同步在这里把这些名字
// 置顶, 让它们出现在最前). extraPriority 的优先级高于内置 priorityNames,
// 即 whitelist 命中的工具会被排在 search_capabilities 等之前.
//
// 关键词: getPrioritizedTools, extraPriority, scenario whitelist top,
//        focus mode pull back ordering
func (r *ReAct) getPrioritizedTools(tools []*aitool.Tool, maxCount int, extraPriority ...string) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}

	// Priority tool names - core tools displayed in prompt
	// search_capabilities should be first as it's the discovery mechanism for all other tools/forges/skills
	priorityNames := []string{
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

	// extraPriority 拼到内置优先名单的"前面", 让 focus mode 拉回的 scenario
	// 工具占据第一梯队. 同时去重, 避免外部传入与内置 priorityNames 同名时
	// 出现重复.
	if len(extraPriority) > 0 {
		seen := make(map[string]bool, len(extraPriority))
		merged := make([]string, 0, len(extraPriority)+len(priorityNames))
		for _, n := range extraPriority {
			if n == "" {
				continue
			}
			if seen[n] {
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

	// Create map for quick lookup
	toolMap := make(map[string]*aitool.Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	var result []*aitool.Tool
	usedNames := make(map[string]bool)

	// Add priority tools first
	for _, name := range priorityNames {
		if tool, exists := toolMap[name]; exists && len(result) < maxCount {
			result = append(result, tool)
			usedNames[name] = true
		}
	}

	// Add remaining tools if we haven't reached maxCount
	for _, tool := range tools {
		if len(result) >= maxCount {
			break
		}
		if !usedNames[tool.Name] {
			result = append(result, tool)
		}
	}

	return result
}

func NewTestReAct(opts ...aicommon.ConfigOption) (*ReAct, error) {
	basicOption := []aicommon.ConfigOption{
		aicommon.WithMemoryTriage(aimem.NewMockMemoryTriage()),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisablePerception(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
		aicommon.WithPeriodicVerificationInterval(0),
	}
	basicOption = append(basicOption, opts...)
	ins, err := NewReAct(
		basicOption...,
	)
	if err != nil {
		return nil, err
	}
	ins.memoryTriage.SetInvoker(ins)
	ins.config.SetConfig("test_yaklang_aikb_rag", true)
	return ins, nil
}
