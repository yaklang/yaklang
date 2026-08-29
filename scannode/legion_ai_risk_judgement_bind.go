package scannode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

// validateYakRiskJudgementScopePin ensures the restart-recovery copy in the
// private runtime snapshot is exactly the same authority carried by the
// protobuf ResultContext. The protobuf context remains the only source used by
// the ResultSink; the JSON copy is stripped before the AI runtime is bound.
func validateYakRiskJudgementScopePin(
	raw json.RawMessage,
	resultContext *aiv1.AIFocusResultContext,
) error {
	runtimeScope, err := decodeYakRiskJudgementScope(raw)
	if err != nil {
		return err
	}
	var resultScope *aiv1.AIFocusRiskJudgementScope
	if resultContext != nil {
		resultScope = resultContext.GetRiskJudgementScope()
	}
	if runtimeScope == nil && resultScope == nil {
		return nil
	}
	if runtimeScope == nil || resultScope == nil {
		return fmt.Errorf("runtime and result risk_judgement_scope must both be present")
	}
	runtimeNormalized, err := normalizeLegionAIRiskJudgementScope(runtimeScope)
	if err != nil {
		return fmt.Errorf("runtime risk_judgement_scope: %w", err)
	}
	resultNormalized, err := normalizeLegionAIRiskJudgementScope(resultScope)
	if err != nil {
		return fmt.Errorf("result risk_judgement_scope: %w", err)
	}
	if runtimeNormalized.ScopeSHA256 != resultNormalized.ScopeSHA256 {
		return fmt.Errorf("runtime and result risk_judgement_scope mismatch")
	}
	return nil
}

func decodeYakRiskJudgementScope(raw json.RawMessage) (*aiv1.AIFocusRiskJudgementScope, error) {
	if !hasYakRuntimeJSONValue(raw) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var scope aiv1.AIFocusRiskJudgementScope
	if err := decoder.Decode(&scope); err != nil {
		return nil, fmt.Errorf("decode risk_judgement_scope: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("risk_judgement_scope contains trailing json values")
	}
	return &scope, nil
}

func stripYakRiskJudgementScopeRuntimeOption(raw []byte) ([]byte, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil, err
	}
	delete(options, "risk_judgement_scope")
	return json.Marshal(options)
}

func hasYakRuntimeJSONValue(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}
