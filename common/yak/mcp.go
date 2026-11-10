package yak

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aimedia"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcpclient"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// Bind registry operations to the invocation's profile rather than the process
// default. Copy the module map so concurrent scripts cannot change each other.
func bindMCPToEngine(engine *antlr4yak.Engine, ctx context.Context, runtime *aitool.ToolRuntimeConfig) {
	defaults := []mcpclient.Option{mcpclient.WithContext(ctx)}
	if runtime != nil && runtime.ProfileDatabase != nil {
		defaults = append(defaults, mcpclient.WithDatabase(runtime.ProfileDatabase))
	}
	options := func(opts []mcpclient.Option) []mcpclient.Option {
		return append(append([]mcpclient.Option{}, defaults...), opts...)
	}
	exports := map[string]any{
		"NewClient": func(transport, endpoint string, opts ...mcpclient.Option) (*mcpclient.Client, error) {
			return mcpclient.NewClient(transport, endpoint, options(opts)...)
		},
		"Connect": func(name string, opts ...mcpclient.Option) (*mcpclient.Client, error) {
			return mcpclient.Connect(name, options(opts)...)
		},
		"AddServer": func(name, transport, endpoint string, opts ...mcpclient.Option) (*mcpclient.ServerInfo, error) {
			return mcpclient.AddServer(name, transport, endpoint, options(opts)...)
		},
		"DeleteServer": func(name string, opts ...mcpclient.Option) error {
			return mcpclient.DeleteServer(name, options(opts)...)
		},
		"ListServers": func(keyword string, opts ...mcpclient.Option) (*mcpclient.ServerPage, error) {
			return mcpclient.ListServers(keyword, options(opts)...)
		},
	}
	engine.SetVars(map[string]any{"mcp": exports})
	agentExports := map[string]any{}
	if original, ok := engine.Var("aiagent").(map[string]any); ok {
		for key, value := range original {
			agentExports[key] = value
		}
	}
	agentExports["MCPResult"] = func(client *mcpclient.Client, server string, result *mcp.CallToolResult) (any, error) {
		source := aimedia.Source{Kind: "mcp", ServerName: server, ObservedAt: time.Now()}
		if runtime != nil {
			source.ToolCallID = runtime.RuntimeID
		}
		return aitool.MCPToolMediaResult(ctx, mcpResourceAdapter{client}, result, source)
	}
	engine.SetVars(map[string]any{"aiagent": agentExports})
}

type mcpResourceAdapter struct{ client *mcpclient.Client }

func (a mcpResourceAdapter) ReadResource(_ context.Context, request mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	return a.client.ReadResource(request.Params.URI)
}

func mcpResult(client *mcpclient.Client, server string, result *mcp.CallToolResult) (any, error) {
	return aitool.MCPToolMediaResult(context.Background(), mcpResourceAdapter{client}, result, aimedia.Source{Kind: "mcp", ServerName: server, ObservedAt: time.Now()})
}
