package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const cybersecurityRiskToolName = "cybersecurity-risk"

func loadCybersecurityRiskAITool(t *testing.T) *schema.AIYakTool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/risk/cybersecurity-risk.yak")
	if err != nil {
		t.Fatalf("failed to read cybersecurity-risk.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(cybersecurityRiskToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse cybersecurity-risk.yak metadata")
	}
	return aiTool
}

func getCybersecurityRiskToolSchema(t *testing.T) map[string]any {
	t.Helper()
	aiTool := loadCybersecurityRiskAITool(t)
	var schemaObj map[string]any
	if err := json.Unmarshal([]byte(aiTool.Params), &schemaObj); err != nil {
		t.Fatalf("failed to unmarshal aiTool.Params: %v\nparams=%s", err, aiTool.Params)
	}
	return schemaObj
}

func TestCybersecurityRisk_MetadataUsesCompactDisclosure(t *testing.T) {
	aiTool := loadCybersecurityRiskAITool(t)

	assert.Assert(t, strings.Contains(aiTool.Usage, "`summary` is mandatory"), "usage should require summary")
	assert.Assert(t, strings.Contains(aiTool.Usage, "title-only"), "usage should forbid title-only risks")
	assert.Assert(t, strings.Contains(aiTool.Usage, "中文标题 / English title"), "usage should document bilingual compact title format")
	assert.Assert(t, strings.Contains(aiTool.Usage, "request  -> <|TOOL_PARAM_request_{NONCE}|>"), "usage should document inline request AITAG")
	assert.Assert(t, strings.Contains(aiTool.Usage, "response -> <|TOOL_PARAM_response_{NONCE}|>"), "usage should document inline response AITAG")
	assert.Assert(t, strings.Contains(aiTool.Usage, "request-file"), "usage should document request-file")
	assert.Assert(t, strings.Contains(aiTool.Usage, "Do not use JSON/object-style complex parameters."), "usage should explicitly avoid object-style params")
}

func TestCybersecurityRisk_SchemaUsesCompactFields(t *testing.T) {
	schemaObj := getCybersecurityRiskToolSchema(t)
	properties, ok := schemaObj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties missing or invalid: %#v", schemaObj["properties"])
	}

	_, ok = properties["summary"]
	assert.Assert(t, ok, "schema should expose summary")
	_, ok = properties["parameter"]
	assert.Assert(t, ok, "schema should expose parameter")
	_, ok = properties["payload"]
	assert.Assert(t, ok, "schema should expose payload")
	_, ok = properties["request"]
	assert.Assert(t, ok, "schema should expose request")
	_, ok = properties["response"]
	assert.Assert(t, ok, "schema should expose response")
	_, ok = properties["evidence"]
	assert.Assert(t, ok, "schema should expose compact evidence field")
	_, ok = properties["request-file"]
	assert.Assert(t, ok, "schema should expose request-file")
	_, ok = properties["response-file"]
	assert.Assert(t, ok, "schema should expose response-file")

	_, ok = properties["title-en"]
	assert.Assert(t, !ok, "schema should not expose title-en")
	_, ok = properties["title-zh"]
	assert.Assert(t, !ok, "schema should not expose title-zh")
	_, ok = properties["finding"]
	assert.Assert(t, !ok, "schema should not expose nested finding object")
	_, ok = properties["http-request"]
	assert.Assert(t, !ok, "http-request should not be a top-level disclosed field")
	_, ok = properties["http-response"]
	assert.Assert(t, !ok, "http-response should not be a top-level disclosed field")
	_, ok = properties["desc"]
	assert.Assert(t, !ok, "desc should not be a top-level disclosed field")
}
