package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aihistory"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func bindAIHistoryToEngine(engine *antlr4yak.Engine, ctx context.Context, runtime *aitool.ToolRuntimeConfig) {
	defaults := []aihistory.Option{aihistory.WithContext(ctx)}
	session := ""
	if runtime != nil {
		session = runtime.PersistentSessionID
		defaults = append(defaults, aihistory.WithDatabase(runtime.ProjectDatabase), aihistory.WithAISession(session),
			aihistory.WithLiveTimeline(session, runtime.TimelineHistorySnapshot))
	}
	exports := make(map[string]any)
	exports["CurrentSession"] = func() string { return session }
	exports["Query"] = func(query string, opts ...aihistory.Option) ([]*aihistory.Item, error) {
		return aihistory.Query(query, append(append([]aihistory.Option{}, defaults...), opts...)...)
	}
	engine.SetVars(map[string]any{"aihistory": exports})
}
