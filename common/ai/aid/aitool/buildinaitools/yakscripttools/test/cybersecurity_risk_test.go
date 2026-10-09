package test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	previousDNS := netx.GetDefaultOptions()
	netx.SetDefaultDNSOptions(append(previousDNS, netx.WithTemporaryHosts(map[string]string{
		"example.test": "127.0.0.1",
	}))...)
	t.Cleanup(func() { netx.SetDefaultDNSOptions(previousDNS...) })
	aiTool := loadCybersecurityRiskAITool(t)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) != 1 {
		t.Fatalf("expected one converted cybersecurity-risk tool, got %d", len(tools))
	}
	return tools[0]
}

func TestCybersecurityRisk_MetadataUsesCompactDisclosure(t *testing.T) {
	aiTool := loadCybersecurityRiskAITool(t)

	assert.Assert(t, strings.Contains(aiTool.Usage, "`target`、`title`、`summary` 必填"), "usage should require complete finding")
	assert.Assert(t, strings.Contains(aiTool.Usage, "避免只有标题或只有猜测"), "usage should forbid unsupported risks")
	assert.Assert(t, strings.Contains(aiTool.Usage, "安全风险或安全见解"), "usage should cover both risks and insights")
	assert.Assert(t, strings.Contains(aiTool.Usage, "`reproduction` 写实际复现步骤、触发路径"), "usage should teach reproducible descriptions")
	assert.Assert(t, strings.Contains(aiTool.Usage, "数据包可选"), "usage should not require HTTP packets")
	assert.Assert(t, strings.Contains(aiTool.Usage, "中文标题 / English title"), "usage should document bilingual compact title format")
	assert.Assert(t, strings.Contains(aiTool.Usage, "request-file"), "usage should document request-file")
}

func TestCybersecurityRisk_SchemaUsesCompactFields(t *testing.T) {
	schemaObj := getCybersecurityRiskToolSchema(t)
	properties, ok := schemaObj["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties missing or invalid: %#v", schemaObj["properties"])
	}

	_, ok = properties["summary"]
	assert.Assert(t, ok, "schema should expose summary")
	_, ok = properties["reproduction"]
	assert.Assert(t, ok, "schema should expose reproduction or observation method")
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
			"target":       "https://example.test/xss?q=admin",
			"title":        "反射型 XSS",
			"summary":      "q 参数未经编码直接进入 HTML 响应。",
			"reproduction": "在浏览器访问 /xss?q=%3Cscript%3Ealert(1)%3C/script%3E，确认脚本执行。",
			"type":         "xss",
			"severity":     "high",
			"parameter":    "q",
			"payload":      "<script>alert(1)</script>",
			"request":      "GET /xss?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1\r\nHost: example.test\r\n\r\n",
			"response":     "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<script>alert(1)</script>",
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
	if submitted.Host != "example.test" || submitted.IP != "127.0.0.1" || submitted.Url != "example.test/xss?q=admin" {
		t.Fatalf("normalized target lost its host, local resolution, or endpoint: %#v", submitted)
	}
	if submitted.Hash != yakit.ComputeRiskHash("example.test/xss?q=admin", "", 0, "xss", "q") {
		t.Fatal("normalizing the host changed the existing risk identity")
	}
	if submitted.RiskType != "xss" || submitted.Severity != "high" || submitted.Parameter != "q" {
		t.Fatalf("unexpected submitted risk: %#v", submitted)
	}
	if submitted.RuntimeId != runtimeID || submitted.QuotedRequest == "" || submitted.QuotedResponse == "" {
		t.Fatalf("missing runtime identity or evidence: %#v", submitted)
	}
	assert.Assert(t, strings.Contains(submitted.Description, "触发入口/观察对象：https://example.test/xss?q=admin"))
	assert.Assert(t, strings.Contains(submitted.Description, "复现/观察方法：\n在浏览器访问 /xss?q="))
	request, err := strconv.Unquote(submitted.QuotedRequest)
	assert.NilError(t, err)
	assert.Assert(t, strings.HasPrefix(request, "GET /xss?q=%3Cscript%3Ealert(1)%3C/script%3E HTTP/1.1\r\n"))
	response, err := strconv.Unquote(submitted.QuotedResponse)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(response, "<script>alert(1)</script>"))
	localRisks, err := yakit.GetRisksByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
	if err != nil {
		t.Fatalf("query local risk database: %v", err)
	}
	if len(localRisks) != 0 {
		t.Fatalf("platform-bound risk leaked into local SQLite: %#v", localRisks)
	}
}

func TestCybersecurityRisk_LocalSavePreservesInvocationClient(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.Risk{}).Error)
	previous := consts.CaptureProjectDatabaseBinding()
	consts.BindProjectDatabaseWithReader(db, nil, "")
	t.Cleanup(func() {
		consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path)
	})
	tool := getCybersecurityRiskTool(t)
	for _, riskType := range []string{"info", "xss"} {
		t.Run(riskType, func(t *testing.T) {
			runtimeID := uuid.NewString()
			params := aitool.InvokeParams{
				"target": "https://example.test/proof", "title": "测试发现", "summary": "已确认的观察结果。",
			}
			if riskType == "xss" {
				params["type"] = "xss"
				params["parameter"] = "q"
				params["payload"] = "<script>alert(1)</script>"
				params["request"] = "GET /proof?q=test HTTP/1.1\r\nHost: example.test\r\n\r\n"
				params["response"] = "HTTP/1.1 200 OK\r\n\r\ntest"
			}
			var riskOutputs []*ypb.ExecResult
			result, err := tool.InvokeWithParams(params, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
				RuntimeID:       runtimeID,
				ProjectDatabase: db,
				FeedBacker: func(output *ypb.ExecResult) error {
					if strings.Contains(string(output.GetMessage()), "json-risk") {
						riskOutputs = append(riskOutputs, output)
					}
					return nil
				},
			}))
			assert.NilError(t, err)
			status, _ := result.GetExecutionStatus()
			assert.Equal(t, status, aitool.ToolExecutionStatusSucceeded)
			records, err := yakit.GetRisksByRuntimeId(db, runtimeID)
			assert.NilError(t, err)
			assert.Equal(t, len(records), 1)
			assert.Equal(t, records[0].RiskType, riskType)
			assert.Equal(t, len(riskOutputs), 1, "saved risk must reach this invocation's feedbacker")
			assert.Equal(t, riskOutputs[0].RuntimeID, runtimeID)
			execution := result.Data.(*aitool.ToolExecutionResult)
			assert.Equal(t, utils.InterfaceToString(utils.InterfaceToGeneralMap(execution.Result)["risk_hash"]), records[0].Hash)
		})
	}
}

func TestCybersecurityRisk_InsightCanBeRecordedWithoutPackets(t *testing.T) {
	var submitted *schema.Risk
	result, err := getCybersecurityRiskTool(t).InvokeWithParams(aitool.InvokeParams{
		"target":       "example.test:443",
		"title":        "暴露的调试入口",
		"summary":      "调试入口可从公网访问，需确认其信息范围。",
		"reproduction": "在未登录会话访问 /debug/，记录页面标题和返回状态。",
		"evidence":     "返回调试页面标题。",
	}, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
		RiskSaveHandler: func(_ context.Context, risk *schema.Risk) error {
			copy := *risk
			submitted = &copy
			return nil
		},
	}))
	assert.NilError(t, err)
	if submitted == nil {
		t.Fatal("insight was not submitted")
	}
	assert.Equal(t, submitted.RiskType, "info")
	assert.Equal(t, submitted.Severity, "info")
	assert.Equal(t, submitted.QuotedRequest, "")
	assert.Equal(t, submitted.QuotedResponse, "")
	assert.Assert(t, strings.Contains(submitted.Description, "触发入口/观察对象：example.test:443"))
	assert.Assert(t, strings.Contains(submitted.Description, "复现/观察方法：\n在未登录会话访问 /debug/"))
	assert.Equal(t, submitted.Solution, "")
	status, _ := result.GetExecutionStatus()
	assert.Equal(t, status, aitool.ToolExecutionStatusSucceeded)
	execution, ok := result.Data.(*aitool.ToolExecutionResult)
	assert.Assert(t, ok)
	assert.Equal(t, utils.InterfaceToString(utils.InterfaceToGeneralMap(execution.Result)["risk_hash"]), submitted.Hash)
}

func TestCybersecurityRisk_IndependentInsightsPersistAndUpdateSeparately(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	assert.NilError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	assert.NilError(t, db.AutoMigrate(&schema.Risk{}).Error)
	tool := getCybersecurityRiskTool(t)
	var submitted []*schema.Risk
	for _, input := range []struct{ target, title, evidence string }{
		{"https://127.0.0.1/debug", "公开调试入口", "first observation"},
		{"https://127.0.0.1/debug", "缺少 HSTS", "second observation"},
		{"https://127.0.0.1/other", "公开调试入口", "different endpoint"},
		{"https://127.0.0.1/debug", "公开调试入口", "updated evidence"},
	} {
		result, err := tool.InvokeWithParams(aitool.InvokeParams{
			"target": input.target, "title": input.title, "summary": input.evidence,
			"evidence": input.evidence,
		}, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RiskSaveHandler: func(_ context.Context, record *schema.Risk) error {
				if err := yakit.CreateOrUpdateRisk(db, record.Hash, record); err != nil {
					return err
				}
				copy := *record
				submitted = append(submitted, &copy)
				return nil
			},
		}))
		assert.NilError(t, err)
		status, _ := result.GetExecutionStatus()
		assert.Equal(t, status, aitool.ToolExecutionStatusSucceeded)
	}
	assert.Equal(t, len(submitted), 4)
	assert.Assert(t, submitted[0].Hash != submitted[1].Hash, "different insights on one target must remain distinct")
	assert.Assert(t, submitted[0].Hash != submitted[2].Hash, "different endpoint paths must remain distinct")
	assert.Equal(t, submitted[0].Hash, submitted[3].Hash)
	assert.Equal(t, submitted[0].ID, submitted[3].ID)
	var rows []schema.Risk
	assert.NilError(t, db.Find(&rows).Error)
	assert.Equal(t, len(rows), 3)
	for _, row := range rows {
		if row.Hash == submitted[0].Hash {
			assert.Assert(t, strings.Contains(row.Description, "updated evidence"))
		}
	}
}

func TestCybersecurityRisk_ReadsPacketFilesOverInlineEvidence(t *testing.T) {
	requestFile := t.TempDir() + "/request.txt"
	responseFile := t.TempDir() + "/response.txt"
	wantRequest := "GET /proof?id=7 HTTP/1.1\r\nHost: example.test\r\n\r\n"
	wantResponse := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nverified-risk-evidence"
	assert.NilError(t, os.WriteFile(requestFile, []byte(wantRequest), 0600))
	assert.NilError(t, os.WriteFile(responseFile, []byte(wantResponse), 0600))

	var submitted *schema.Risk
	_, err := getCybersecurityRiskTool(t).InvokeWithParams(aitool.InvokeParams{
		"target":        "https://example.test/proof?id=7",
		"title":         "已验证的风险",
		"summary":       "测试响应包含预期证据。",
		"request":       "GET /wrong HTTP/1.1\r\nHost: wrong.test\r\n\r\n",
		"response":      "HTTP/1.1 404 Not Found\r\n\r\nwrong",
		"request-file":  requestFile,
		"response-file": responseFile,
	}, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
		RiskSaveHandler: func(_ context.Context, risk *schema.Risk) error {
			copy := *risk
			submitted = &copy
			return nil
		},
	}))
	assert.NilError(t, err)
	if submitted == nil {
		t.Fatal("risk was not submitted")
	}
	gotRequest, err := strconv.Unquote(submitted.QuotedRequest)
	assert.NilError(t, err)
	gotResponse, err := strconv.Unquote(submitted.QuotedResponse)
	assert.NilError(t, err)
	assert.Equal(t, gotRequest, wantRequest)
	assert.Equal(t, gotResponse, wantResponse)
	assert.Assert(t, strings.Contains(gotRequest, "GET /proof?id=7 HTTP/1.1"))
	assert.Assert(t, strings.Contains(gotResponse, "verified-risk-evidence"))
	detailsText, err := strconv.Unquote(submitted.Details)
	assert.NilError(t, err)
	var details map[string]any
	assert.NilError(t, json.Unmarshal([]byte(detailsText), &details))
	assert.Equal(t, details["request_source"], "file")
	assert.Equal(t, details["response_source"], "file")
}

func TestCybersecurityRisk_InvalidEvidenceReturnsFailure(t *testing.T) {
	emptyFile := t.TempDir() + "/empty.txt"
	assert.NilError(t, os.WriteFile(emptyFile, nil, 0600))
	for _, tc := range []struct{ name, field, value, message string }{
		{"empty request", "request-file", emptyFile, "request-file is empty"},
		{"empty response", "response-file", emptyFile, "response-file is empty"},
		{"missing request", "request-file", emptyFile + ".missing", "failed to read request-file"},
		{"missing response", "response-file", emptyFile + ".missing", "failed to read response-file"},
		{"blank summary", "summary", " \n\t", "summary is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			params := aitool.InvokeParams{
				"target": "https://example.test/empty", "title": "测试风险", "summary": "有验证证据。",
				"request":  "GET /inline HTTP/1.1\r\nHost: example.test\r\n\r\n",
				"response": "HTTP/1.1 200 OK\r\n\r\ninline",
			}
			params[tc.field] = tc.value
			result, err := getCybersecurityRiskTool(t).InvokeWithParams(params, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
				RiskSaveHandler: func(context.Context, *schema.Risk) error {
					called = true
					return nil
				},
			}))
			assert.Assert(t, err != nil && strings.Contains(err.Error(), tc.message), "expected actionable error, got %v", err)
			assert.Assert(t, result == nil || !result.Success, "invalid evidence must not complete successfully")
			assert.Assert(t, !called, "invalid evidence must not create a risk record or fall back to inline packets")
		})
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

func TestCybersecurityRisk_NormalizesTypeAndParameterBeforeDedupHash(t *testing.T) {
	tool := getCybersecurityRiskTool(t)
	var submitted []*schema.Risk
	for _, params := range []aitool.InvokeParams{
		{"target": "https://example.test/search", "title": "反射型 XSS", "summary": "q 参数未经编码回显。", "type": " XSS ", "parameter": " q "},
		{"target": "https://example.test/search", "title": "反射型 XSS", "summary": "q 参数未经编码回显。", "type": "xss", "parameter": "q"},
	} {
		_, err := tool.InvokeWithParams(params, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{
			RiskSaveHandler: func(_ context.Context, risk *schema.Risk) error {
				copy := *risk
				submitted = append(submitted, &copy)
				return nil
			},
		}))
		if err != nil {
			t.Fatalf("InvokeWithParams() error = %v", err)
		}
	}
	if len(submitted) != 2 {
		t.Fatalf("expected two risk submissions, got %d", len(submitted))
	}
	for _, risk := range submitted {
		if risk.RiskType != "xss" || risk.Parameter != "q" {
			t.Fatalf("risk options were not normalized: %#v", risk)
		}
	}
	if submitted[0].Hash == "" || submitted[0].Hash != submitted[1].Hash {
		t.Fatalf("equivalent risk keys produced different hashes: %q and %q", submitted[0].Hash, submitted[1].Hash)
	}
}
