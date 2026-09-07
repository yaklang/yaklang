package aid

import (
	"fmt"

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

func outputEvidenceAction(_ *AiTask) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"output_evidence",
		"Append key newly verified runtime evidence into the shared EVIDENCE document. Evidence is optional in normal verification, but this action is for deliberate evidence delivery when you have reusable findings worth preserving.",
		[]aitool.ToolOption{
			aitool.WithStringParam(planEvidenceFieldName,
				aitool.WithParam_Description("新增的 evidence Markdown。系统会自动与历史 EVIDENCE 合并并执行 token 裁剪。建议优先写关键新增发现，可使用 `## 新增待测试列表`、`## 某一个事实发现` 等小节；每条至少写清楚主体是谁、发现了什么。"),
			),
		},
		[]*reactloops.LoopStreamField{{
			FieldName:   planEvidenceFieldName,
			AINodeId:    planEvidenceAINodeID,
			ContentType: aicommon.TypeTextMarkdown,
		}},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			evidence := aicommon.NormalizeConcreteEvidenceMarkdown(action.GetString(planEvidenceFieldName))
			if evidence == "" {
				return utils.Error("output_evidence: evidence content is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			evidence := aicommon.NormalizeConcreteEvidenceMarkdown(action.GetString(planEvidenceFieldName))
			evidenceOp, err := reactloops.SaveSessionEvidence(loop.GetConfig(), "", evidence)
			if err != nil {
				op.Fail(utils.Wrap(err, "output_evidence failed"))
				return
			}
			idMarker := fmt.Sprintf("[id: %s]", evidenceOp.ID)
			log.Infof("task loop: output_evidence saved to Session Evidence Store, id=%s", evidenceOp.ID)
			loop.GetInvoker().AddToTimeline("session_evidence_saved",
				fmt.Sprintf("%s\n%s", idMarker, evidenceOp.Content))
			op.Continue()
		},
	)
}
