package scannode

import (
	"encoding/json"
	"strings"
	"testing"
)

func validFocusExecutionContractJSON(t *testing.T) string {
	t.Helper()
	raw, err := json.Marshal(legionFocusExecutionContract{
		SchemaVersion: legionFocusExecutionContractSchemaV1,
		Stages: []legionFocusExecutionStage{
			{Key: "source_prepare"},
			{Key: "report_generation"},
		},
		Capabilities: []string{
			"result.report.v1",
			"source.read",
			"task.stage",
		},
		Results: []legionFocusExecutionResultContract{
			{Key: "report", Capability: "result.report.v1", Kind: "ai_code_audit_v1", Required: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestParseLegionFocusExecutionContractBuildsClosedAllowlists(t *testing.T) {
	contract, err := parseLegionFocusExecutionContract(validFocusExecutionContractJSON(t))
	if err != nil {
		t.Fatalf("parse execution contract: %v", err)
	}
	if !contract.allowsStage("source_prepare") || contract.allowsStage("model_selected") {
		t.Fatalf("unexpected stage allowlist: %#v", contract.stageSet)
	}
	if !contract.allowsCapability("source.read") || contract.allowsCapability("http.request") {
		t.Fatalf("unexpected capability allowlist: %#v", contract.capabilitySet)
	}
	result, ok := contract.resultForCapability("result.report.v1")
	if !ok || result.Kind != "ai_code_audit_v1" || !result.Required {
		t.Fatalf("unexpected result mapping: %#v ok=%v", result, ok)
	}
}

func TestParseLegionFocusExecutionContractFailsClosed(t *testing.T) {
	valid := validFocusExecutionContractJSON(t)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "schema", raw: strings.Replace(valid, legionFocusExecutionContractSchemaV1, "legion.focus-execution/v2", 1), want: "schema_version"},
		{name: "unknown result capability", raw: strings.Replace(valid, `"capability":"result.report.v1"`, `"capability":"result.finding.v1"`, 1), want: "undeclared capability"},
		{name: "non canonical", raw: strings.Replace(valid, `{"schema_version"`, `{ "schema_version"`, 1), want: "canonical JSON"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseLegionFocusExecutionContract(test.raw); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parse error = %v, want %q", err, test.want)
			}
		})
	}
}
