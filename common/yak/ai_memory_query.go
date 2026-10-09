package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aimemory"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// Bind the actual memory set ID and project DB, without a registry or callback.
func bindMemorySearchToEngine(engine *antlr4yak.Engine, ctx context.Context, runtime *aitool.ToolRuntimeConfig) {
	id := "default"
	defaults := []aimemory.Option{aimemory.WithContext(ctx)}
	if runtime != nil {
		defaults = append(defaults, aimemory.WithDatabase(runtime.ProjectDatabase))
		if runtime.MemoryNamespace != "" {
			id = runtime.MemoryNamespace
		}
	}
	defaults = append(defaults, aimemory.WithMemoryNamespace(id))
	exports := make(map[string]any, len(aimemory.Exports))
	for name, value := range aimemory.Exports {
		exports[name] = value
	}
	exports["CurrentNamespace"] = func() string { return id }
	exports["Search"] = func(query string, opts ...aimemory.Option) ([]*aimemory.Item, error) {
		return aimemory.Search(query, append(append([]aimemory.Option{}, defaults...), opts...)...)
	}
	exports["Amend"] = func(operator, memoryID, content string, tags []string, opts ...aimemory.Option) (*aimemory.Item, error) {
		return aimemory.Amend(operator, memoryID, content, tags, append(append([]aimemory.Option{}, defaults...), opts...)...)
	}
	engine.SetVars(map[string]any{"aimemory": exports})
}
