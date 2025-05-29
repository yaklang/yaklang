package mcp

import (
	"maps"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

type ToolWithHandler struct {
	tool    *mcp.Tool
	handler ToolHandlerWrapperFunc
}

var (
	globalTools    = make(map[string]*ToolWithHandler, 0)
	globalToolSets = make(map[string]*ToolSet, 0)
)

type ToolSet struct {
	Tools map[string]*ToolWithHandler
}

type ToolSetOption func(*ToolSet)
type ToolHandlerWrapperFunc func(*MCPServer) server.ToolHandlerFunc

func WithTool(tool *mcp.Tool, handler ToolHandlerWrapperFunc) ToolSetOption {
	return func(b *ToolSet) {
		b.Tools[tool.Name] = &ToolWithHandler{
			tool:    tool,
			handler: handler,
		}
	}
}

func AddGlobalToolSet(setName string, opts ...ToolSetOption) {
	b := &ToolSet{
		Tools: make(map[string]*ToolWithHandler),
	}
	for _, opt := range opts {
		opt(b)
	}

	globalToolSets[setName] = b
	maps.Copy(globalTools, b.Tools)
}

func GlobalToolSetList() []string {
	return lo.Keys(globalToolSets)
}
