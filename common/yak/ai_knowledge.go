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
		"List": func(opts ...aiknowledge.Option) (map[string]any, error) { return aiknowledge.List(options(opts)...) },
		"Add": func(name string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.Add(name, options(opts)...)
		},
		"Get": func(name string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.Get(name, options(opts)...)
		},
		"Update": func(name string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.Update(name, options(opts)...)
		},
		"Delete": func(name string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.Delete(name, options(opts)...)
		},
		"RebuildIndex": func(name string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.RebuildIndex(name, options(opts)...)
		},
		"Build": func(name, content string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.Build(name, content, options(opts)...)
		},
		"BuildFromFile": func(name, path string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.BuildFromFile(name, path, options(opts)...)
		},
		"AddEntry": func(name, title, content string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.AddEntry(name, title, content, options(opts)...)
		},
		"UpdateEntry": func(name, id, title, content string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.UpdateEntry(name, id, title, content, options(opts)...)
		},
		"DeleteEntry": func(name, id string, opts ...aiknowledge.Option) (map[string]any, error) {
			return aiknowledge.DeleteEntry(name, id, options(opts)...)
		},
		"Query": func(scope any, query string, tokenLimit int, opts ...aiknowledge.Option) (*aiknowledge.QueryResult, error) {
			return aiknowledge.Query(scope, query, tokenLimit, options(opts)...)
		},
	}})
}
