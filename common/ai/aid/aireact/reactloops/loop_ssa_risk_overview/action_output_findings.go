package loop_ssa_risk_overview

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

var outputOverviewFindingsAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"output_overview_findings",
		"Record intermediate SSA risk triage notes (severity clusters, rule families, priorities). "+
			"Merged on loop only — NOT streamed to the chat UI (avoids duplicate ssa-overview-findings cards). "+
			"Call once per distinct risk cluster; do not re-submit the same risk_id set with rephrased text.",
		[]aitool.ToolOption{
			aitool.WithStringParam(overviewFindingsFieldName,
				aitool.WithParam_Description("Markdown findings (## headings). Merged with prior findings; duplicate lines and repeated risk_id clusters are dropped."),
				aitool.WithParam_Required(true),
			),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if normalizeFindings(action.GetString(overviewFindingsFieldName)) == "" {
				return utils.Error("output_overview_findings: findings content is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			incoming := normalizeFindings(action.GetString(overviewFindingsFieldName))
			merged, changed := appendOverviewFindings(loop, incoming)
			taskID := ""
			if task := loop.GetCurrentTask(); task != nil {
				taskID = task.GetId()
			}
			if !changed {
				msg := "output_overview_findings: no new content (duplicate lines or same risk_id cluster already recorded)."
				if emitter := loop.GetEmitter(); emitter != nil {
					emitter.EmitThoughtStream(taskID, msg)
				}
				op.Feedback(msg)
				op.Continue()
				return
			}
			log.Infof("ssa_risk_overview: output_overview_findings merged, length=%d", len(merged))
			if emitter := loop.GetEmitter(); emitter != nil {
				emitter.EmitThoughtStream(taskID, "Recorded overview findings (%d chars, merged total %d)", len(incoming), len(merged))
			}
			recordMetaAction(loop, "output_overview_findings",
				"recorded triage findings",
				utils.ShrinkTextBlock(incoming, 200))
			r.AddToTimeline("ssa_risk_overview_findings",
				utils.ShrinkTextBlock(incoming, 1500))
			op.Feedback(fmt.Sprintf("[output_overview_findings] merged into loop findings (%d runes total).", len([]rune(merged))))
			op.Continue()
		},
	)
}
