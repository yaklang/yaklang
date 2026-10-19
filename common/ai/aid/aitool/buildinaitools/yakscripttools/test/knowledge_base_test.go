package test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
)

func TestKnowledgeBaseYakTools(t *testing.T) {
	profile, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "profile.db"))
	require.NoError(t, err)
	defer profile.Close()
	tools := map[string]*aitool.Tool{}
	for _, name := range []string{"manage_knowledge_base", "query_knowledge_base"} {
		source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/knowledge/" + name + ".yak")
		require.NoError(t, err)
		metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
		require.NotNil(t, metadata)
		converted := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
		require.Len(t, converted, 1)
		tools[name] = converted[0]
	}
	schemaJSON, err := json.Marshal(tools["query_knowledge_base"].InputSchema)
	require.NoError(t, err)
	require.Contains(t, string(schemaJSON), `"required":["token_limit"]`)
	execute := func(name string, params map[string]any) (*aitool.ToolExecutionResult, error) {
		// ReAct parameters cross a JSON boundary, including nested arrays.
		raw, err := json.Marshal(params)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &params))
		result, err := tools[name].InvokeWithParams(aitool.InvokeParams(params), aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{ProfileDatabase: profile}))
		if result == nil || result.Data == nil {
			return nil, err
		}
		execution, ok := result.Data.(*aitool.ToolExecutionResult)
		require.True(t, ok)
		return execution, err
	}
	invoke := func(name string, params map[string]any) map[string]any {
		t.Helper()
		out, err := execute(name, params)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal([]byte(out.Stdout), &value), "stdout=%s", out.Stdout)
		if name == "query_knowledge_base" {
			require.LessOrEqual(t, ytoken.CalcTokenCount(out.Stdout), int(value["token_limit"].(float64)), "the actual tool stdout must fit the whole-response budget")
		}
		return value
	}
	invoke("manage_knowledge_base", map[string]any{"op": "create", "name": "yak-tool-fixture", "metadata": map[string]any{"description": "原始\n描述", "tags": []string{"docs"}}})
	added := invoke("manage_knowledge_base", map[string]any{"op": "add_entry", "name": "yak-tool-fixture", "entry": map[string]any{"title": "启动说明", "content": "原始内容\n引号\" 反斜杠\\ 😀"}, "skip_index": true})
	require.False(t, added["indexed"].(bool), "skip_index must survive the tool/CLI boundary")
	queried := invoke("query_knowledge_base", map[string]any{"name": "yak-tool-fixture", "mode": "get", "entry_id": added["entry_id"], "token_limit": 512})
	hits := queried["results"].([]any)
	require.Len(t, hits, 1)
	require.Equal(t, "原始内容\n引号\" 反斜杠\\ 😀", hits[0].(map[string]any)["content"])
	for _, scope := range []map[string]any{{"all": true}, {"names": []string{"yak-tool-fixture"}}} {
		scope["token_limit"] = 512
		scope["per_entry_token_limit"] = 256
		page := invoke("query_knowledge_base", scope)
		require.Equal(t, float64(1), page["total"])
		require.Equal(t, "yak-tool-fixture", page["results"].([]any)[0].(map[string]any)["knowledge_base"])
	}
	invoke("manage_knowledge_base", map[string]any{"op": "create", "name": "yak-tool-other"})
	invoke("manage_knowledge_base", map[string]any{"op": "add_entry", "name": "yak-tool-other", "entry": map[string]any{"title": "other", "content": "SECOND_SCOPE_MARKER"}, "skip_index": true})
	multi := invoke("query_knowledge_base", map[string]any{"names": []string{"yak-tool-fixture", "yak-tool-other"}, "token_limit": 1024, "limit": 1})
	require.Equal(t, float64(2), multi["total"])
	require.Len(t, multi["results"], 1, "multiple names must survive CLI conversion and share one result limit")
	_, scopeError := execute("query_knowledge_base", map[string]any{"all": false, "token_limit": 512})
	require.ErrorContains(t, scopeError, "manage_knowledge_base")
	require.ErrorContains(t, scopeError, `"op":"list"`)
	listed := invoke("manage_knowledge_base", map[string]any{"op": "list", "keyword": "yak-tool-fixture"})
	require.Equal(t, float64(1), listed["total"])
	for _, tc := range []struct {
		name   string
		params map[string]any
	}{
		{"query_knowledge_base", map[string]any{"name": "yak-tool-fixture"}},
		{"query_knowledge_base", map[string]any{"token_limit": 512}},
		{"manage_knowledge_base", map[string]any{"op": "create", "name": "yak-tool-fixture"}},
		{"manage_knowledge_base", map[string]any{"op": "build", "name": "missing", "content": "must not create"}},
	} {
		_, err := execute(tc.name, tc.params)
		require.Error(t, err)
	}
	invoke("manage_knowledge_base", map[string]any{"op": "delete", "name": "yak-tool-fixture"})
	invoke("manage_knowledge_base", map[string]any{"op": "delete", "name": "yak-tool-other"})
	require.Equal(t, float64(0), invoke("manage_knowledge_base", map[string]any{"op": "list"})["total"])
}
