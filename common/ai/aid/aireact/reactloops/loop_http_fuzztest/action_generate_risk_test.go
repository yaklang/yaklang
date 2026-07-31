package loop_http_fuzztest

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestParseGenerateRiskDetails_JSONObject(t *testing.T) {
	details := parseGenerateRiskDetails(`{"evidence":"idor","tested_payloads":["1001","1002"]}`)
	if got := details["evidence"]; got != "idor" {
		t.Fatalf("expected evidence to be parsed, got %v", got)
	}
	if _, ok := details["tested_payloads"].([]any); !ok {
		t.Fatalf("expected tested_payloads to be parsed as array, got %#v", details["tested_payloads"])
	}
}

func TestParseGenerateRiskDetails_PlainText(t *testing.T) {
	details := parseGenerateRiskDetails("状态码与响应长度出现稳定差异")
	if got := details["summary"]; got != "状态码与响应长度出现稳定差异" {
		t.Fatalf("expected plain text details to be stored as summary, got %v", got)
	}
}

func TestIsValidGenerateRiskSeverity(t *testing.T) {
	for _, severity := range []string{"critical", "high", "warning", "medium", "low", "info", "warn", "middle"} {
		if !isValidGenerateRiskSeverity(severity) {
			t.Fatalf("expected severity %q to be valid", severity)
		}
	}
	if isValidGenerateRiskSeverity("unknown") {
		t.Fatal("expected unknown severity to be invalid")
	}
}

func TestGenerateRiskSpecFromParams_SupportsStructuredDetails(t *testing.T) {
	spec := generateRiskSpecFromParams(aitool.InvokeParams{
		"target":      "https://example.com/api/orders?id=1",
		"title":       "订单接口越权",
		"risk_type":   "privilege-escalation",
		"severity":    "high",
		"description": "切换 id 后返回其他用户订单。",
		"details": map[string]any{
			"evidence": "id=1/2 返回不同用户",
		},
		"payload": "id=2",
	})

	if spec.Title != "订单接口越权" {
		t.Fatalf("unexpected title: %s", spec.Title)
	}
	if !strings.Contains(spec.Details, `"evidence"`) {
		t.Fatalf("expected structured details to be encoded as JSON, got %s", spec.Details)
	}
}

func TestValidateGenerateRiskSpec_RequiresFields(t *testing.T) {
	err := validateGenerateRiskSpec(nil, generateRiskSpec{Title: "missing type"}, 0)
	if err == nil || !strings.Contains(err.Error(), "risk_type is required") {
		t.Fatalf("expected risk_type validation error, got %v", err)
	}
}

func TestSaveGeneratedRiskUsesInjectedResultSink(t *testing.T) {
	var submitted *schema.Risk
	sink := aicommon.ResultSinkFunc(func(
		_ context.Context,
		risk *schema.Risk,
	) (aicommon.ResultReceipt, error) {
		submitted = risk
		return aicommon.ResultReceipt{
			ResultID:  "risk-result-1",
			DedupeKey: "dedupe-1",
			BackendID: "job-1",
		}, nil
	})
	loop := newTestHTTPFuzzLoopWithResultSink(t, sink)

	resultID, summary, persistedLocally, err := saveGeneratedRisk(loop, generateRiskSpec{
		Target:       "https://example.com/api/orders?id=2",
		Title:        "订单接口越权",
		TitleVerbose: "订单接口存在越权读取",
		RiskType:     "privilege-escalation",
		Severity:     "high",
		Description:  "切换订单 ID 后返回其他用户数据。",
		Details:      `{"evidence":"id=2"}`,
		Payload:      "id=2",
	})
	if err != nil {
		t.Fatalf("save generated risk: %v", err)
	}
	if persistedLocally {
		t.Fatal("expected injected result sink to own persistence")
	}
	if resultID != "risk-result-1" {
		t.Fatalf("unexpected result id: %s", resultID)
	}
	if submitted == nil {
		t.Fatal("expected risk to be submitted")
	}
	if submitted.ID != 0 {
		t.Fatalf("expected risk not to be saved in local database, got id=%d", submitted.ID)
	}
	if submitted.RiskType != "privilege-escalation" || submitted.Severity != "high" {
		t.Fatalf("unexpected submitted risk: %#v", submitted)
	}
	if !strings.Contains(summary, "result_id=risk-result-1") {
		t.Fatalf("unexpected summary: %s", summary)
	}
}

func newTestHTTPFuzzLoopWithResultSink(
	t *testing.T,
	sink aicommon.ResultSink,
) *reactloops.ReActLoop {
	t.Helper()
	config := aicommon.NewConfig(
		context.Background(),
		aicommon.WithResultSink(sink),
	)
	return reactloops.NewMinimalReActLoop(config, nil)
}
