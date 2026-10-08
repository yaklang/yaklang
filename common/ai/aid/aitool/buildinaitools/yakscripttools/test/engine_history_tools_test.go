package test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
)

func TestEngineHistoryToolsRuntimeBinding(t *testing.T) {
	openDB := func(name string, models ...any) *gorm.DB {
		db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), name))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		require.NoError(t, db.AutoMigrate(models...).Error)
		return db
	}
	project := openDB("project.db", &schema.AIAgentRuntime{}, &schema.AIMemoryEntity{}, &schema.AIMemoryCollection{}, &schema.ProjectGeneralStorage{}, &schema.HTTPFlow{})
	profile := openDB("profile.db", &schema.Project{})
	require.NoError(t, profile.Create(&schema.Project{ProjectName: "host project", Type: "project", DatabasePath: "host.db"}).Error)
	require.NoError(t, project.Create(&schema.HTTPFlow{Url: "https://host.example/login", Method: "POST", StatusCode: 201}).Error)
	stored := aicommon.NewTimeline(nil, nil)
	stored.PushText(1, "old persisted text")
	raw, err := aicommon.MarshalTimeline(stored)
	require.NoError(t, err)
	require.NoError(t, project.Create(&schema.AIAgentRuntime{Uuid: "runtime-a", PersistentSession: "session-a", QuotedTimeline: strconv.Quote(raw)}).Error)
	live, err := aicommon.UnmarshalTimeline(raw)
	require.NoError(t, err)
	live.PushText(2, "live sentinel 当前历史")
	live.PushTextWithPromptProjection(3, "display text", "projection-only-answer")
	foreign := aicommon.NewTimeline(nil, nil)
	foreign.PushText(1, "foreign sentinel")
	foreignRaw, err := aicommon.MarshalTimeline(foreign)
	require.NoError(t, err)
	require.NoError(t, project.Create(&schema.AIAgentRuntime{Uuid: "runtime-b", PersistentSession: "session-b", QuotedTimeline: strconv.Quote(foreignRaw)}).Error)
	runtime := &aitool.ToolRuntimeConfig{ProjectDatabase: project, ProfileDatabase: profile, MemoryNamespace: "memory-a", PersistentSessionID: "session-a", TimelineSnapshot: func() (string, error) { return aicommon.MarshalTimeline(live) }}
	invoke := func(path, name string, params map[string]any) string {
		source, err := yakscripttools.GetEmbedFS().ReadFile(path)
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, tools, 1)
		config := aitool.NewToolInvokeConfig()
		aitool.WithRuntimeConfig(runtime)(config)
		result, err := tools[0].ExecuteToolWithCapture(context.Background(), params, config)
		require.NoError(t, err)
		out, err := json.Marshal(result.Result)
		require.NoError(t, err)
		return string(out)
	}
	result := invoke("yakscriptforai/memory/amend_memory.yak", "amend_memory", map[string]any{"operator": "add", "content": "host-bound preference sentinel"})
	require.Contains(t, result, `"namespace":"memory-a"`)
	result = invoke("yakscriptforai/memory/read_memory.yak", "read_memory", map[string]any{"query": "preference", "search_mode": "bm25"})
	require.Contains(t, result, "host-bound preference sentinel")
	result = invoke("yakscriptforai/history/grep_timeline_history.yak", "grep_timeline_history", map[string]any{"query": "sentinel", "search_mode": "regexp"})
	require.Contains(t, result, "live sentinel")
	require.NotContains(t, result, "foreign sentinel")
	require.Contains(t, result, `"session_id":"session-a"`)
	result = invoke("yakscriptforai/history/grep_timeline_history.yak", "grep_timeline_history", map[string]any{"query": "projection-only-answer", "search_mode": "literal", "view": "value_json"})
	require.Contains(t, result, "projection-only-answer")
	require.Contains(t, result, "prompt_text")
	result = invoke("yakscriptforai/history/query_yak_projects.yak", "query_yak_projects", map[string]any{})
	require.Contains(t, result, "host project")
	result = invoke("yakscriptforai/history/query_http_history.yak", "query_http_history", map[string]any{"methods": "POST", "status_code": "201"})
	require.Contains(t, result, "https://host.example/login")
}
