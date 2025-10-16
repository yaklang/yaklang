package aireact

import (
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// getPrioritizedTools returns a prioritized list of tools, with search tools first
func (r *ReAct) getPrioritizedTools(tools []*aitool.Tool, maxCount int) []*aitool.Tool {
	if len(tools) == 0 {
		return tools
	}

	// Priority tool names (tools_search should be first)
	priorityNames := []string{
		"tools_search",
		"aiforge_search",
		"now",
		"bash",
		"read_file",
		"grep",
		"find_file",
		"send_http_request_by_url",
		"whois",
		"dig",
		"scan_tcp_port",
		"encode",
		"decode",
		"auto_decode",
		"current_time",
		"echo",
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

func NewTestReAct(opts ...Option) (*ReAct, error) {
	basicOption := []Option{
		WithMemoryTriage(aimem.NewMockMemoryTriage()),
	}
	basicOption = append(basicOption, opts...)
	ins, err := NewReAct(
		basicOption...,
	)
	if err != nil {
		return nil, err
	}
	ins.memoryTriage.SetInvoker(ins)
	return ins, nil
}
