package scannode

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestValidateYakRiskJudgementScopePinAcceptsExactPrivateCopy(t *testing.T) {
	resultContext := validAIRiskJudgementResultContext()
	raw, err := json.Marshal(resultContext.RiskJudgementScope)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateYakRiskJudgementScopePin(raw, resultContext); err != nil {
		t.Fatalf("matching risk judgement scope rejected: %v", err)
	}
}

func TestValidateYakRiskJudgementScopePinRejectsMismatchAndUnknownFields(t *testing.T) {
	resultContext := validAIRiskJudgementResultContext()
	mismatched := *resultContext.RiskJudgementScope
	mismatched.ProjectId = "project-other"
	raw, err := json.Marshal(&mismatched)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateYakRiskJudgementScopePin(raw, resultContext); err == nil ||
		!strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatched risk judgement scope was not rejected: %v", err)
	}

	unknown := append([]byte(nil), raw[:len(raw)-1]...)
	unknown = append(unknown, []byte(`,"submit_judgement":true}`)...)
	if err := validateYakRiskJudgementScopePin(unknown, resultContext); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown nested scope field was not rejected: %v", err)
	}
}

func TestPrepareLegionCodeWorkspaceStripsPrivateRiskJudgementScope(t *testing.T) {
	resultContext := validAIRiskJudgementResultContext()
	rawScope, err := json.Marshal(resultContext.RiskJudgementScope)
	if err != nil {
		t.Fatal(err)
	}
	rawOptions, err := json.Marshal(map[string]json.RawMessage{
		"review_policy":        json.RawMessage(`"yolo"`),
		"risk_judgement_scope": rawScope,
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace, publicOptions, err := prepareLegionCodeWorkspace(
		context.Background(), rawOptions, legionCodeWorkspaceMaterializeOptions{},
	)
	if err != nil {
		t.Fatalf("prepare runtime options: %v", err)
	}
	if workspace != nil {
		t.Fatal("risk judgement runtime unexpectedly created a source workspace")
	}
	if strings.Contains(string(publicOptions), "risk_judgement_scope") ||
		!strings.Contains(string(publicOptions), `"review_policy":"yolo"`) {
		t.Fatalf("private scope was not stripped safely: %s", publicOptions)
	}
}

func TestRiskJudgementScopeCannotEnterHotpatchOrTurnContext(t *testing.T) {
	params := yakRuntimeOptions{
		ReviewPolicy:       "yolo",
		RiskJudgementScope: json.RawMessage(`{"task_run_id":"attacker"}`),
	}
	if err := validateYakAIHotpatch(aicommon.HotPatchType_AgreePolicy, params); err == nil ||
		!strings.Contains(err.Error(), "cannot change risk_judgement_scope") {
		t.Fatalf("hotpatch risk judgement scope was not rejected: %v", err)
	}
	if err := validateLegionCodeWorkspaceContextPin(
		[]byte(`{"risk_judgement_scope":{"task_run_id":"attacker"}}`),
		[]byte(`{}`),
	); err == nil || !strings.Contains(err.Error(), "private bind-only") {
		t.Fatalf("turn context risk judgement scope was not rejected: %v", err)
	}
}
