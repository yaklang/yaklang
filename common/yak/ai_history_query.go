package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
)

func bindAIHistoryToEngine(engine *antlr4yak.Engine, ctx context.Context, runtime *aitool.ToolRuntimeConfig) {
	defaults := []yaklib.AIQueryOption{yaklib.WithAIQueryContext(ctx)}
	sessionID := ""
	if runtime != nil {
		sessionID = runtime.PersistentSessionID
		defaults = append(defaults, yaklib.WithAIQueryDatabases(runtime.ProjectDatabase, runtime.ProfileDatabase), yaklib.WithAISession(sessionID), yaklib.WithAITimelineSnapshot(sessionID, runtime.TimelineSnapshot))
	}
	exports := make(map[string]any)
	if existing, ok := engine.GetVar("yakit"); ok {
		if values, ok := existing.(map[string]any); ok {
			for name, value := range values {
				exports[name] = value
			}
		}
	}
	if len(exports) == 0 {
		for name, value := range yaklib.YakitExports {
			exports[name] = value
		}
	}
	options := func(opts []yaklib.AIQueryOption) []yaklib.AIQueryOption {
		return append(append([]yaklib.AIQueryOption{}, defaults...), opts...)
	}
	exports["CurrentAISession"] = func() string { return sessionID }
	exports["GrepAITimelineHistory"] = func(query string, opts ...yaklib.AIQueryOption) (map[string]any, error) {

		return yaklib.GrepAITimelineHistory(query, options(opts)...)
	}
	exports["QueryYakProjects"] = func(query string, opts ...yaklib.AIQueryOption) (map[string]any, error) {
		return yaklib.QueryYakProjects(query, options(opts)...)
	}
	exports["QueryHTTPHistory"] = func(filter string, opts ...yaklib.AIQueryOption) (map[string]any, error) {
		return yaklib.QueryHTTPHistory(filter, options(opts)...)
	}
	for name, query := range map[string]func(string, ...yaklib.AIQueryOption) (map[string]any, error){
		"QueryCybersecurityRisk": yaklib.QueryCybersecurityRisk,
		"QueryKnowledge":         yaklib.QueryKnowledge,
		"QueryPayloads":          yaklib.QueryPayloads,
		"ManagePayloads":         yaklib.ManagePayloads,
		"ManageKnowledge":        yaklib.ManageKnowledge,
	} {
		query := query
		exports[name] = func(filter string, opts ...yaklib.AIQueryOption) (map[string]any, error) {
			return query(filter, options(opts)...)
		}
	}
	engine.SetVars(map[string]any{"yakit": exports})
	profileDB := consts.GetGormProfileDatabase()
	if runtime != nil && runtime.ProfileDatabase != nil {
		profileDB = runtime.ProfileDatabase
	}
	fuzzExports := make(map[string]any)
	if existing, ok := engine.GetVar("fuzz"); ok {
		if values, ok := existing.(map[string]any); ok {
			for name, value := range values {
				fuzzExports[name] = value
			}
		}
	}
	if len(fuzzExports) == 0 {
		for name, value := range yaklib.FuzzExports {
			fuzzExports[name] = value
		}
	}
	fuzzExports["RenderHTTPTemplate"] = func(input string, vars map[string]interface{}, limit int, renderCtx context.Context) ([]string, error) {
		return mutate.RenderHTTPTemplate(input, vars, limit, mutate.WithPayloadDatabaseContext(renderCtx, profileDB))
	}
	fuzzExports["RenderHTTPFields"] = func(fields []string, vars map[string]interface{}, limit int, renderCtx context.Context) ([][]string, error) {
		return mutate.RenderHTTPFields(fields, vars, limit, mutate.WithPayloadDatabaseContext(renderCtx, profileDB))
	}
	fuzzExports["Render"] = func(input interface{}, opts ...mutate.FuzzConfigOpt) ([]string, error) {
		return mutate.FuzzTagExec(input, append([]mutate.FuzzConfigOpt{mutate.Fuzz_WithPayloadDatabase(profileDB)}, opts...)...)
	}
	engine.SetVars(map[string]any{"fuzz": fuzzExports})
}
