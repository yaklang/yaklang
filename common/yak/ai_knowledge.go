package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aiknowledge"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func bindAIKnowledgeToEngine(engine *antlr4yak.Engine, ctx context.Context, runtime *aitool.ToolRuntimeConfig) {
	defaults := []aiknowledge.Option{aiknowledge.WithContext(ctx)}
	if runtime != nil && runtime.ProfileDatabase != nil {
		defaults = append(defaults, aiknowledge.WithDatabase(runtime.ProfileDatabase))
	}
	options := func(opts []aiknowledge.Option) []aiknowledge.Option {
		return append(append([]aiknowledge.Option{}, defaults...), opts...)
	}
	engine.SetVars(map[string]any{"aiknowledge": map[string]any{
		"Manage": func(request map[string]any, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.Manage(request, options(opts)...)
		},
		"Query": func(request map[string]any, opts ...aiknowledge.Option) (*aiknowledge.QueryResult, error) {
			return aiknowledge.Query(request, options(opts)...)
		},
	}})
}
