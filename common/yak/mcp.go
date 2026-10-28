package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
}
