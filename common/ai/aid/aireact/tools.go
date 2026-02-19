package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// getPrioritizedTools returns a prioritized list of tools, with search tools first
func (r *ReAct) getPrioritizedTools(tools []*aitool.Tool, maxCount int) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}

	// Priority tool names - the 14 core tools displayed in prompt
	// search_capabilities should be first as it's the discovery mechanism for all other tools/forges/skills
	priorityNames := []string{
		"search_capabilities",
		"grep",
		"read_file",
		"write_file",
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
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
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
