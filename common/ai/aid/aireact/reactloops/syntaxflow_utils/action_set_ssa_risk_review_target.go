package syntaxflow_utils

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// WithSetSSARiskReviewTargetAction registers set_ssa_risk_review_target: switch the focused SSA risk id mid-session without new attachments.
func WithSetSSARiskReviewTargetAction(_ aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"set_ssa_risk_review_target",
		"Set the active SSA risk primary key for this ssa_risk_review loop (loop var ssa_risk_id). After changing, use the ssa-risk tool with the new risk_id before drawing conclusions.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id", aitool.WithParam_Description("SSA Risk database id (positive integer)."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetInt("risk_id") <= 0 {
				return utils.Error("risk_id must be a positive integer")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			id := int64(action.GetInt("risk_id"))
			loop.Set(LoopVarSSARiskID, fmt.Sprintf("%d", id))
			invoker := loop.GetInvoker()
			msg := fmt.Sprintf("目标 SSA Risk ID 已切换为 %d。请先使用 ssa-risk 工具拉取该条（risk_id=%d, get_full_code 视需要设为 true）。", id, id)
			invoker.AddToTimeline("ssa_risk_review", msg)
			operator.Feedback(msg)
			operator.Continue()
		},
	)
}
