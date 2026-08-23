package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const saveEvidenceOperationLoopVar = "save_evidence_operation"

// SaveSessionEvidence is the shared handler-level write path used by both the
// core save_evidence action and compatibility actions in higher-level loops.
// It validates, applies, and reads the Session Evidence Store back before
// reporting success.
func SaveSessionEvidence(cfg aicommon.AICallerConfigIf, evidenceID, content string) (aicommon.EvidenceOperation, error) {
	if cfg == nil {
		return aicommon.EvidenceOperation{}, utils.Error("session configuration is unavailable")
	}
	evidenceOp, err := aicommon.BuildSessionEvidenceUpsert(evidenceID, content)
	if err != nil {
		return aicommon.EvidenceOperation{}, err
	}
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{evidenceOp})

	rendered := cfg.GetSessionEvidenceRendered()
	idMarker := fmt.Sprintf("[id: %s]", evidenceOp.ID)
	if !strings.Contains(rendered, idMarker) || !strings.Contains(rendered, evidenceOp.Content) {
		return aicommon.EvidenceOperation{}, utils.Errorf("Session Evidence Store did not contain id=%q after apply", evidenceOp.ID)
	}
	return evidenceOp, nil
}

// loopAction_SaveEvidence is a core action rather than a registry extension.
// Every production ReActLoop therefore exposes the same evidence contract,
// including default loops, plan-execution task loops, and infrastructure loops.
var loopAction_SaveEvidence = &LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE,
	Description: "Persist a concise, reusable finding directly into the shared Session Evidence Store, then continue the task. " +
		"Use it for confirmed facts, discriminating negative results, or state changes that later work should rely on. " +
		"Saving is idempotent: reuse evidence_id to update the same finding; when omitted, the system derives a stable ID from the content.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"evidence_id",
			aitool.WithParam_Description("Optional stable semantic ID (1-128 characters: letters, digits, dot, underscore, colon, or hyphen). Reuse it to update the same finding."),
		),
		aitool.WithStringParam(
			"verification_payload",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Concise reusable evidence: what was tested or observed, how it was confirmed, the concrete result, and why it matters. This content is written directly to Session Evidence."),
		),
	},
	StreamFields: []*LoopStreamField{
		{FieldName: "verification_payload", AINodeId: "verification_payload"},
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
		if loop == nil || action == nil {
			return utils.Error("save_evidence requires a loop and parsed action")
		}
		payload := strings.TrimSpace(action.GetString("verification_payload"))
		if payload == "" {
			payload = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("verification_payload"))
		}
		evidenceID := strings.TrimSpace(action.GetString("evidence_id"))
		if evidenceID == "" {
			evidenceID = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("evidence_id"))
		}
		evidenceOp, err := aicommon.BuildSessionEvidenceUpsert(evidenceID, payload)
		if err != nil {
			return utils.Wrap(err, "save_evidence validation failed")
		}
		loop.Set(saveEvidenceOperationLoopVar, evidenceOp)
		return nil
	},
	ActionHandler: func(loop *ReActLoop, _ *aicommon.Action, operator *LoopActionHandlerOperator) {
		rawOp := loop.GetVariable(saveEvidenceOperationLoopVar)
		evidenceOp, ok := rawOp.(aicommon.EvidenceOperation)
		if !ok {
			operator.Fail(utils.Error("save_evidence internal error: validated evidence operation is unavailable"))
			return
		}

		evidenceOp, err := SaveSessionEvidence(loop.GetConfig(), evidenceOp.ID, evidenceOp.Content)
		if err != nil {
			operator.Fail(utils.Wrap(err, "save_evidence failed"))
			return
		}

		idMarker := fmt.Sprintf("[id: %s]", evidenceOp.ID)
		loop.GetInvoker().AddToTimeline("session_evidence_saved", fmt.Sprintf("%s\n%s", idMarker, evidenceOp.Content))
		operator.Feedback(fmt.Sprintf("Session evidence saved as %s; continue with the remaining task.", idMarker))
		operator.Continue()
	},
}
