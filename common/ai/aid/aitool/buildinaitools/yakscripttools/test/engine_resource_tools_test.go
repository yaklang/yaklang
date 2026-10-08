package test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func resourceToolFixture(t *testing.T) (*gorm.DB, *aitool.ToolRuntimeConfig) {
	t.Helper()
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "resources.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.Payload{}, &schema.Risk{}, &schema.KnowledgeBaseInfo{}, &schema.KnowledgeBaseEntry{}, &schema.VectorStoreDocument{}, &schema.VectorStoreCollection{}).Error)
	return db, &aitool.ToolRuntimeConfig{ProjectDatabase: db, ProfileDatabase: db}
}

func invokeResourceTool(t *testing.T, name string, params map[string]any, runtime *aitool.ToolRuntimeConfig) map[string]any {
	t.Helper()
	dir := "history"
	if name == "exec_fuzztag" {
		dir = "codec"
	}
	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/" + dir + "/" + name + ".yak")
	require.NoError(t, err)
	metadata := yakscripttools.LoadYakScriptToAiTools(name, string(source))
	require.NotNil(t, metadata)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{metadata})
	require.Len(t, tools, 1)
	// Match the JSON-native values supplied by real AI tool calls, including arrays.
	paramsJSON, err := json.Marshal(params)
	require.NoError(t, err)
	var jsonParams map[string]any
	require.NoError(t, json.Unmarshal(paramsJSON, &jsonParams))
	result, err := tools[0].InvokeWithParams(jsonParams, aitool.WithRuntimeConfig(runtime))
	require.NoError(t, err)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	require.True(t, ok)
	raw, err := json.Marshal(execution.Result)
	require.NoError(t, err)
	semantic := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &semantic))
	return semantic
}

func TestEngineResourceToolsRuntimeBinding(t *testing.T) {
	db, runtime := resourceToolFixture(t)
	risk := &schema.Risk{Title: "host resource vulnerability", Severity: "critical", Solution: "fix sentinel"}
	require.NoError(t, db.Create(risk).Error)
	kb := &schema.KnowledgeBaseInfo{KnowledgeBaseName: "host knowledge", KnowledgeBaseType: "security"}
	require.NoError(t, db.Create(kb).Error)
	entry := &schema.KnowledgeBaseEntry{KnowledgeBaseID: int64(kb.ID), KnowledgeTitle: "host knowledge sentinel", KnowledgeType: "guideline", KnowledgeDetails: "original source text", HiddenIndex: "host-source"}
	require.NoError(t, db.Create(entry).Error)
	result := invokeResourceTool(t, "query_cybersecurity_risk", map[string]any{"keyword": "host resource", "severity": "critical"}, runtime)
	require.Equal(t, float64(1), result["total"])
	result = invokeResourceTool(t, "query_cybersecurity_risk", map[string]any{"risk_id": risk.ID}, runtime)
	require.Contains(t, result["hits"].([]any)[0].(map[string]any)["content"], "fix sentinel")
	result = invokeResourceTool(t, "query_knowledge", map[string]any{"query": "sentinel"}, runtime)
	require.Equal(t, float64(1), result["total"])
	result = invokeResourceTool(t, "manage_payloads", map[string]any{"operator": "add", "group": "api_cases", "folder": "web", "contents": []string{"a & b", "c+\" {{int(1-3)}}"}}, runtime)
	require.Equal(t, float64(2), result["affected"])
	result = invokeResourceTool(t, "query_payloads", map[string]any{}, runtime)
	require.Equal(t, float64(1), result["total"])
	result = invokeResourceTool(t, "query_payloads", map[string]any{"group": "api_cases"}, runtime)
	require.Equal(t, float64(2), result["total"])
	ids := []any{result["hits"].([]any)[0].(map[string]any)["payload_id"]}
	result = invokeResourceTool(t, "manage_payloads", map[string]any{"operator": "change", "group": "api_cases", "payload_ids": ids, "contents": []string{"replacement"}}, runtime)
	require.Equal(t, float64(1), result["affected"])
	result = invokeResourceTool(t, "manage_payloads", map[string]any{"operator": "delete", "group": "api_cases", "payload_ids": ids}, runtime)
	require.Equal(t, float64(1), result["affected"])
}

func TestManageKnowledgeToolLifecycle(t *testing.T) {
	_, runtime := resourceToolFixture(t)
	added := invokeResourceTool(t, "manage_knowledge", map[string]any{"operator": "add", "collection": "agent notes", "title": "权限检查", "content": "first original sentinel", "keywords": []string{"a,b", "权限"}, "questions": []string{"需要什么权限？"}}, runtime)
	require.True(t, added["created_knowledge_base"].(bool))
	require.Equal(t, "pending", added["semantic_index_status"])
	selected := map[string]any{"knowledge_base_id": added["knowledge_base_id"], "entry_uuid": added["entry_uuid"]}
	read := invokeResourceTool(t, "query_knowledge", selected, runtime)
	require.Equal(t, float64(1), read["total"])
	require.Equal(t, "first original sentinel", read["hits"].([]any)[0].(map[string]any)["content"])
	changed := invokeResourceTool(t, "manage_knowledge", map[string]any{"operator": "change", "knowledge_base_id": added["knowledge_base_id"], "entry_uuid": added["entry_uuid"], "title": "权限更新", "content": "second original sentinel"}, runtime)
	require.NotEqual(t, added["entry_uuid"], changed["entry_uuid"])
	read = invokeResourceTool(t, "query_knowledge", selected, runtime)
	require.Zero(t, read["total"])
	selected["entry_uuid"] = changed["entry_uuid"]
	read = invokeResourceTool(t, "query_knowledge", selected, runtime)
	require.Equal(t, "second original sentinel", read["hits"].([]any)[0].(map[string]any)["content"])
	invokeResourceTool(t, "manage_knowledge", map[string]any{"operator": "delete", "knowledge_base_id": added["knowledge_base_id"], "entry_id": changed["entry_id"]}, runtime)
	read = invokeResourceTool(t, "query_knowledge", selected, runtime)
	require.Zero(t, read["total"])
}

func TestPayloadToolsToHTTPRequestUsesRuntimeDatabase(t *testing.T) {
	_, runtime := resourceToolFixture(t)
	_, foreignRuntime := resourceToolFixture(t)
	invokeResourceTool(t, "manage_payloads", map[string]any{"operator": "add", "group": "bound_cases", "contents": []string{"a & b", "c+\" {{int(1-3)}}"}}, runtime)
	invokeResourceTool(t, "manage_payloads", map[string]any{"operator": "add", "group": "bound_cases", "contents": []string{"foreign sentinel"}}, foreignRuntime)
	rendered := invokeResourceTool(t, "exec_fuzztag", map[string]any{"template": "{{payload(bound_cases)}}", "format": "json", "limit": 2}, runtime)
	require.True(t, rendered["success"].(bool))
	require.Equal(t, float64(2), rendered["count"])
	server, received := fuzztagHTTPServer(t)
	config := aitool.NewToolInvokeConfig()
	aitool.WithRuntimeConfig(runtime)(config)
	params := map[string]any{"url": server.URL + "/submit", "method": "POST", "form": map[string]any{"value": "{{payload(bound_cases)}}"}, "fuzztag": true, "max-requests": 2}
	result, err := getDoHTTPRequestTool(t).ExecuteToolWithCapture(context.Background(), params, config)
	require.NoError(t, err)
	semantic := utils.InterfaceToGeneralMap(result.Result)
	cleanupHTTPArtifacts(t, semantic)
	require.Equal(t, 2, utils.InterfaceToInt(semantic["response_received_count"]))
	require.Len(t, received, 2)
	got := []string{(<-received).form, (<-received).form}
	require.ElementsMatch(t, []string{"a & b", "c+\" {{int(1-3)}}"}, got)
	params["max-requests"] = 1
	_, err = getDoHTTPRequestTool(t).ExecuteToolWithCapture(context.Background(), params, config)
	require.Error(t, err)
	require.Empty(t, received)
	params["form"] = map[string]any{"value": "{{payload(missing_group_sentinel)}}"}
	_, err = getDoHTTPRequestTool(t).ExecuteToolWithCapture(context.Background(), params, config)
	require.Error(t, err)
	require.Empty(t, received)
	params["form"] = map[string]any{"value": "{{payload(bound_cases)}}"}
	params["max-requests"] = 2
	params["form"] = map[string]any{"value": "{{base64({{payload(bound_cases)}})}}"}
	result, err = getDoHTTPRequestTool(t).ExecuteToolWithCapture(context.Background(), params, config)
	require.NoError(t, err)
	cleanupHTTPArtifacts(t, utils.InterfaceToGeneralMap(result.Result))
	require.Len(t, received, 2)
	require.ElementsMatch(t, []string{base64.StdEncoding.EncodeToString([]byte("a & b")), base64.StdEncoding.EncodeToString([]byte("c+\" {{int(1-3)}}"))}, []string{(<-received).form, (<-received).form})
	invokeResourceTool(t, "manage_payloads", map[string]any{"operator": "add", "group": "multiline_cases", "contents": []string{" x\n", " y\t"}}, runtime)
	params["form"] = map[string]any{"value": "{{payload:full(multiline_cases)}}"}
	result, err = getDoHTTPRequestTool(t).ExecuteToolWithCapture(context.Background(), params, config)
	require.NoError(t, err)
	cleanupHTTPArtifacts(t, utils.InterfaceToGeneralMap(result.Result))
	require.Len(t, received, 2)
	require.ElementsMatch(t, []string{" x\n", " y\t"}, []string{(<-received).form, (<-received).form})
	params["form"] = map[string]any{"value": "{{payload(bound_cases)}}"}
	aitool.WithRuntimeConfig(foreignRuntime)(config)
	params["max-requests"] = 2
	result, err = getDoHTTPRequestTool(t).ExecuteToolWithCapture(context.Background(), params, config)
	require.NoError(t, err)
	semantic = utils.InterfaceToGeneralMap(result.Result)
	cleanupHTTPArtifacts(t, semantic)
	require.True(t, utils.InterfaceToBoolean(semantic["response_received"]))
	require.Len(t, received, 1)
	require.Equal(t, "foreign sentinel", (<-received).form)
}
