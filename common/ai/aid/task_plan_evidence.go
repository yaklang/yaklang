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
		"Append key newly verified runtime evidence into the shared EVIDENCE document. Evidence is optional in normal verification, but this action is for deliberate evidence delivery when you have reusable findings worth preserving.",
		[]aitool.ToolOption{
			aitool.WithStringParam(planEvidenceFieldName,
				aitool.WithParam_Description("本轮新增的 evidence Markdown。系统会自动与历史 EVIDENCE 合并并执行 token 裁剪。建议优先写关键新增发现，可使用 `## 新增待测试列表`、`## 某一个事实发现` 等小节；每条至少写清楚主体是谁、发现了什么。"),
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
		func(_ *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			evidence := aicommon.NormalizeConcreteEvidenceMarkdown(action.GetString(planEvidenceFieldName))
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
