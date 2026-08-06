package test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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

func getCybersecurityRiskTool(t *testing.T) *aitool.Tool {
	t.Helper()
	aiTool := loadCybersecurityRiskAITool(t)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) != 1 {
		t.Fatalf("expected one converted cybersecurity-risk tool, got %d", len(tools))
	}
	return tools[0]
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

func TestCybersecurityRisk_UsesRuntimeRiskSinkInsteadOfLocalDatabase(t *testing.T) {
	tool := getCybersecurityRiskTool(t)
	runtimeID := "runtime-server-risk-" + uuid.NewString()
	var submitted *schema.Risk
	_, err := tool.InvokeWithParams(
		aitool.InvokeParams{
			"target":    "https://example.test/xss?q=admin",
			"title":     "反射型 XSS",
			"summary":   "q 参数未经编码直接进入 HTML 响应。",
			"type":      "xss",
			"severity":  "high",
			"parameter": "q",
			"payload":   "<script>alert(1)</script>",
			"request":   "GET /xss?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1\r\nHost: example.test\r\n\r\n",
			"response":  "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<script>alert(1)</script>",
		},
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RuntimeID: runtimeID,
			RiskSaveHandler: func(_ context.Context, risk *schema.Risk) error {
				copy := *risk
				submitted = &copy
				return nil
			},
		}),
	)
	if err != nil {
		t.Fatalf("InvokeWithParams() error = %v", err)
	}
	if submitted == nil {
		t.Fatal("expected runtime risk sink submission")
	}
	if submitted.RiskType != "xss" || submitted.Severity != "high" || submitted.Parameter != "q" {
		t.Fatalf("unexpected submitted risk: %#v", submitted)
	}
	if submitted.RuntimeId != runtimeID || submitted.QuotedRequest == "" || submitted.QuotedResponse == "" {
		t.Fatalf("missing runtime identity or evidence: %#v", submitted)
	}
	localRisks, err := yakit.GetRisksByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
	if err != nil {
		t.Fatalf("query local risk database: %v", err)
	}
	if len(localRisks) != 0 {
		t.Fatalf("platform-bound risk leaked into local SQLite: %#v", localRisks)
	}
}

func TestCybersecurityRisk_PropagatesRuntimeRiskSinkFailure(t *testing.T) {
	tool := getCybersecurityRiskTool(t)
	_, err := tool.InvokeWithParams(
		aitool.InvokeParams{
			"target":  "https://example.test/xss",
			"title":   "反射型 XSS",
			"summary": "输入未经编码直接进入 HTML 响应。",
			"type":    "xss",
		},
		aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RiskSaveHandler: func(context.Context, *schema.Risk) error {
				return errors.New("platform unavailable")
			},
		}),
	)
	if err == nil || !strings.Contains(err.Error(), "platform unavailable") {
		t.Fatalf("expected platform submission error, got %v", err)
	}
}
