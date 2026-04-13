package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	planEvidenceFieldName = "evidence"
	planEvidenceAINodeID  = "plan-evidence"
)

func outputEvidenceAction(task *AiTask) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"output_evidence",
		"Append newly verified runtime evidence into the shared EVIDENCE document. Evidence must be incremental Markdown derived from real execution or validation results, and must enumerate concrete items explicitly without vague wording like 等/其他/若干.",
		[]aitool.ToolOption{
			aitool.WithStringParam(planEvidenceFieldName,
				aitool.WithParam_Description("本轮新增的 evidence Markdown。系统会自动与历史 EVIDENCE 合并并执行 token 裁剪。必须逐项列出具体路径、接口、文件、参数或现象，严禁使用“等”“其他”“若干”。"),
			),
		},
		[]*reactloops.LoopStreamField{{
			FieldName:   planEvidenceFieldName,
			AINodeId:    planEvidenceAINodeID,
			ContentType: aicommon.TypeTextMarkdown,
		}},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			evidence, err := aicommon.NormalizeConcreteEvidenceMarkdown(action.GetString(planEvidenceFieldName))
			if err != nil {
				return utils.Wrap(err, "output_evidence")
			}
			if evidence == "" {
				return utils.Error("output_evidence: evidence content is required")
			}
			return nil
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			evidence, err := aicommon.NormalizeConcreteEvidenceMarkdown(action.GetString(planEvidenceFieldName))
			if err != nil {
				log.Warnf("task loop: output_evidence rejected vague evidence: %v", err)
				op.Continue()
				return
			}
			merged, changed := appendTaskPlanEvidence(task, evidence)
			if changed {
				log.Infof("task loop: output_evidence merged, length=%d", len(merged))
			} else {
				log.Infof("task loop: output_evidence received no new evidence")
			}
			op.Continue()
		},
	)
}
