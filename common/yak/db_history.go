package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func bindDBHistoryToEngine(engine *antlr4yak.Engine, ctx context.Context, runtime *aitool.ToolRuntimeConfig) {
	var defaults []yaklib.DBHistoryOption
	if runtime != nil {
		defaults = []yaklib.DBHistoryOption{yaklib.WithDBHistoryRuntime(ctx, runtime.ProjectDatabase, runtime.ProfileDatabase)}
	} else {
		defaults = []yaklib.DBHistoryOption{yaklib.WithDBHistoryRuntime(ctx, nil, nil)}
	}
	exports := make(map[string]any, len(yaklib.DatabaseExports))
	for name, fn := range yaklib.DatabaseExports {
		exports[name] = fn
	}
	options := func(opts []yaklib.DBHistoryOption) []yaklib.DBHistoryOption {
		return append(append([]yaklib.DBHistoryOption{}, defaults...), opts...)
	}
	exports["ListYakProjects"] = func(opts ...yaklib.DBHistoryOption) ([]*yaklib.YakProject, error) {
		return yaklib.ListYakProjects(options(opts)...)
	}
	exports["QueryHTTPFlows"] = func(opts ...yaklib.DBHistoryOption) (*yaklib.HTTPHistoryPage, error) {
		return yaklib.QueryHTTPFlows(options(opts)...)
	}
	exports["QueryHTTPFlowByID"] = func(id int64, opts ...yaklib.DBHistoryOption) (*yaklib.HTTPHistoryItem, error) {
		return yaklib.QueryHTTPFlowByID(id, options(opts)...)
	}
	engine.SetVars(map[string]any{"db": exports})
}
