package test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
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
	manageSchema, err := json.Marshal(tools["manage_knowledge_base"].InputSchema)
	require.NoError(t, err)
	for _, field := range []string{"omni_mode", "omni_model", "omni_segment_seconds", "omni_payload_bytes"} {
		require.Contains(t, string(manageSchema), field)
	}
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
		if value["success"] == false {
			raw, err := json.Marshal(out.Result)
			require.NoError(t, err)
			var semantic map[string]any
			require.NoError(t, json.Unmarshal(raw, &semantic))
			require.Equal(t, value, semantic, "business errors must reach the structured result channel")
			status, _ := (&aitool.ToolResult{Success: true, Data: out}).GetExecutionStatus()
			require.Equal(t, aitool.ToolExecutionStatusFailed, status)
		}
		if name == "query_knowledge_base" && value["success"] != false {
			require.LessOrEqual(t, ytoken.CalcTokenCount(out.Stdout), int(value["token_limit"].(float64)), "the actual tool stdout must fit the whole-response budget")
		}
		return value
	}
	invoke("manage_knowledge_base", map[string]any{"op": "create", "name": "yak-tool-fixture", "metadata": map[string]any{"description": "原始\n描述", "tags": []string{"docs"}}})
	added := invoke("manage_knowledge_base", map[string]any{"op": "add_entry", "name": "yak-tool-fixture", "entry": map[string]any{"title": "启动说明", "content": "原始内容\n引号\" 反斜杠\\ 😀"}, "skip_index": true})
	require.False(t, added["indexed"].(bool), "skip_index must survive the tool/CLI boundary")
	queried := invoke("query_knowledge_base", map[string]any{"name": "yak-tool-fixture", "mode": "get", "entry_id": added["entry_id"], "token_limit": 512})
	require.Equal(t, false, queried["has_more"])
	require.Equal(t, float64(0), queried["next_offset"])
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
	scopeError := invoke("query_knowledge_base", map[string]any{"all": false, "token_limit": 512})
	require.Equal(t, "INVALID_ARGUMENT", scopeError["error_code"])
	require.Contains(t, scopeError["message"], "manage_knowledge_base")
	require.Contains(t, scopeError["message"], `"op":"list"`)
	listed := invoke("manage_knowledge_base", map[string]any{"op": "list", "keyword": "yak-tool-fixture"})
	require.Equal(t, float64(1), listed["total"])
	require.Equal(t, float64(1), listed["knowledge_bases"].([]any)[0].(map[string]any)["entry_count"])
	for _, tc := range []struct {
		name   string
		params map[string]any
	}{
		{"query_knowledge_base", map[string]any{"name": "yak-tool-fixture"}},
		{"manage_knowledge_base", map[string]any{"op": "build", "name": "yak-tool-fixture", "content": "must reject", "build_options": map[string]any{"omni_mode": "unknown"}}},
		{"manage_knowledge_base", map[string]any{"op": "build", "name": "yak-tool-fixture", "content": "must reject", "build_options": map[string]any{"omni_segment_seconds": -1}}},
		{"manage_knowledge_base", map[string]any{"op": "build", "name": "yak-tool-fixture", "content": "must reject", "build_options": map[string]any{"omni_payload_bytes": 100}}},
	} {
		_, err := execute(tc.name, tc.params)
		require.Error(t, err)
	}
	for _, tc := range []struct {
		name, code string
		params     map[string]any
	}{
		{"query_knowledge_base", "INVALID_ARGUMENT", map[string]any{"token_limit": 512}},
		{"query_knowledge_base", "INVALID_ARGUMENT", map[string]any{"name": "yak-tool-fixture", "all": true, "token_limit": 512}},
		{"query_knowledge_base", "INVALID_ARGUMENT", map[string]any{"name": "yak-tool-fixture", "names": []string{"yak-tool-other"}, "token_limit": 512}},
		{"query_knowledge_base", "INVALID_ARGUMENT", map[string]any{"name": "yak-tool-fixture", "mode": "get", "token_limit": 512}},
		{"query_knowledge_base", "NOT_FOUND", map[string]any{"name": "yak-tool-fixture", "mode": "get", "entry_id": "missing", "token_limit": 512}},
		{"manage_knowledge_base", "ALREADY_EXISTS", map[string]any{"op": "create", "name": "yak-tool-fixture"}},
		{"manage_knowledge_base", "NOT_FOUND", map[string]any{"op": "get", "name": "missing"}},
		{"manage_knowledge_base", "NOT_FOUND", map[string]any{"op": "build", "name": "missing", "content": "must not create"}},
		{"manage_knowledge_base", "INVALID_ARGUMENT", map[string]any{"op": "add_entry", "name": "yak-tool-fixture", "entry": map[string]any{}}},
		{"manage_knowledge_base", "INVALID_ARGUMENT", map[string]any{"op": "build", "name": "yak-tool-fixture", "content": "x", "source_file": "also supplied"}},
	} {
		value := invoke(tc.name, tc.params)
		require.Equal(t, false, value["success"], "%s %v", tc.name, tc.params)
		require.Equal(t, tc.code, value["error_code"], "%s %v", tc.name, tc.params)
		require.NotEmpty(t, value["message"])
		require.NotContains(t, value["message"], "YakVM Panic")
	}
	invoke("manage_knowledge_base", map[string]any{"op": "update", "name": "yak-tool-fixture", "metadata": map[string]any{"description": "updated description"}})
	details := invoke("manage_knowledge_base", map[string]any{"op": "get", "name": "yak-tool-fixture"})
	require.Equal(t, "updated description", details["knowledge_base"].(map[string]any)["description"])
	require.Equal(t, "updated description", details["collection"].(map[string]any)["Description"])
	oldMock := vectorstore.IsMockMode
	vectorstore.IsMockMode = true
	t.Cleanup(func() { vectorstore.IsMockMode = oldMock })
	built, err := execute("manage_knowledge_base", map[string]any{"op": "build", "name": "yak-tool-fixture", "content": "ORBIT-731 fixture recovery", "build_options": map[string]any{"mode": "chunks"}})
	require.NoError(t, err)
	require.Contains(t, built.Stdout, "embedding ready")
	require.Contains(t, built.Stdout, "persisted_entries=1")
	require.Contains(t, built.Stdout, "completed; all reported entries persisted")
	for _, title := range []string{"indexed first", "indexed second"} {
		invoke("manage_knowledge_base", map[string]any{"op": "add_entry", "name": "yak-tool-fixture", "entry": map[string]any{"title": title, "content": "ORBIT-731 recovery"}})
	}
	semantic := invoke("query_knowledge_base", map[string]any{"name": "yak-tool-fixture", "mode": "semantic", "query": "ORBIT-731", "limit": 1, "token_limit": 1024})
	require.Greater(t, semantic["total"].(float64), float64(1))
	require.Len(t, semantic["results"], 1)
	require.Equal(t, false, semantic["has_more"])
	require.Equal(t, float64(0), semantic["next_offset"])
	invoke("manage_knowledge_base", map[string]any{"op": "delete", "name": "yak-tool-fixture"})
	invoke("manage_knowledge_base", map[string]any{"op": "delete", "name": "yak-tool-other"})
	require.Equal(t, float64(0), invoke("manage_knowledge_base", map[string]any{"op": "list"})["total"])
}
