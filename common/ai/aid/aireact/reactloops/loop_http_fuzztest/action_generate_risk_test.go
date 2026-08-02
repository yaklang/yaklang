package loop_http_fuzztest

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
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
