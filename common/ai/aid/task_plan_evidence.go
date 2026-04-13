package aid

import (
	"strings"

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
		"Append newly verified runtime evidence into the shared EVIDENCE document. Evidence must be incremental Markdown derived from real execution or validation results.",
		[]aitool.ToolOption{
			aitool.WithStringParam(planEvidenceFieldName,
				aitool.WithParam_Description("本轮新增的 evidence Markdown。系统会自动与历史 EVIDENCE 合并并执行 token 裁剪。"),
			),
		},
		[]*reactloops.LoopStreamField{{
			FieldName:   planEvidenceFieldName,
			AINodeId:    planEvidenceAINodeID,
			ContentType: aicommon.TypeTextMarkdown,
		}},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			evidence := strings.TrimSpace(action.GetString(planEvidenceFieldName))
			if evidence == "" {
				return utils.Error("output_evidence: evidence content is required")
			}
			return nil
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			merged, changed := appendTaskPlanEvidence(task, action.GetString(planEvidenceFieldName))
			if changed {
				log.Infof("task loop: output_evidence merged, length=%d", len(merged))
			} else {
				log.Infof("task loop: output_evidence received no new evidence")
			}
			op.Continue()
		},
	)
}
