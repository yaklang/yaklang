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
	op, _, err := saveSessionEvidenceWithChange(cfg, evidenceID, content)
	return op, err
}

func saveSessionEvidenceWithChange(cfg aicommon.AICallerConfigIf, evidenceID, content string) (aicommon.EvidenceOperation, bool, error) {
	if cfg == nil {
		return aicommon.EvidenceOperation{}, false, utils.Error("session configuration is unavailable")
	}
	evidenceOp, err := aicommon.BuildSessionEvidenceUpsert(evidenceID, content)
	if err != nil {
		return aicommon.EvidenceOperation{}, false, err
	}
	before := cfg.GetSessionEvidenceRendered()
	cfg.ApplySessionEvidenceOps([]aicommon.EvidenceOperation{evidenceOp})
	changed := cfg.GetSessionEvidenceRendered() != before

	rendered := cfg.GetSessionEvidenceRendered()
	idMarker := fmt.Sprintf("[id: %s]", evidenceOp.ID)
	if !strings.Contains(rendered, idMarker) || !strings.Contains(rendered, evidenceOp.Content) {
		return aicommon.EvidenceOperation{}, false, utils.Errorf("Session Evidence Store did not contain id=%q after apply", evidenceOp.ID)
	}
	return evidenceOp, changed, nil
}

// loopAction_SaveEvidence is a core action rather than a registry extension.
// Every production ReActLoop therefore exposes the same evidence contract,
// including default loops, plan-execution task loops, and infrastructure loops.
var loopAction_SaveEvidence = &LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_SAVE_EVIDENCE,
	Description: "直接保存简明、可复用的已确认事实、排除结果或状态变化，然后继续任务。" +
		"子智能体显式保存的证据也会进入父证据日志，内容变化时可提前唤醒父智能体；原始工具输出和其他私有证据不自动共享。" +
		"复用 evidence_id 可更新同一发现；省略时按内容生成稳定标识，相同内容重试不新增证据增量。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"evidence_id",
			aitool.WithParam_Description("可选的稳定语义标识，1 至 128 个字母、数字、点、下划线、冒号或连字符；更新同一发现时复用。"),
		),
		aitool.WithStringParam(
			"evidence_content",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("可复用的具体证据：验证对象与方式、实际观测、确认结果及其意义；内容会直接写入会话证据。"),
		),
	},
	StreamFields: []*LoopStreamField{
		{FieldName: "evidence_content", AINodeId: "evidence_content"},
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
		if loop == nil || action == nil {
			return utils.Error("save_evidence requires a loop and parsed action")
		}
		payload := strings.TrimSpace(action.GetString("evidence_content"))
		if payload == "" {
			payload = strings.TrimSpace(action.GetInvokeParams("next_action").GetString("evidence_content"))
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

		before := loop.GetConfig().GetSessionEvidenceRendered()
		if err := loop.GetConfig().BroadcastEvidence(evidenceOp); err != nil {
			operator.Fail(utils.Wrap(err, "save_evidence failed"))
			return
		}
		changed := loop.GetConfig().GetSessionEvidenceRendered() != before
		if changed && loop.IsSubAgent() && loop.notifyEvidence != nil {
			if err := loop.notifyEvidence(evidenceOp.ID); err != nil {
				operator.Fail(utils.Wrap(err, "notify parent of evidence discovery failed"))
				return
			}
		}

		idMarker := fmt.Sprintf("[id: %s]", evidenceOp.ID)
		loop.GetInvoker().AddToTimeline("session_evidence_saved", fmt.Sprintf("%s 已保存，证据正文已记录到时间线增量。", idMarker))
		operator.Feedback(fmt.Sprintf("会话证据 %s 已保存，请继续剩余任务。", idMarker))
		operator.Continue()
	},
}
