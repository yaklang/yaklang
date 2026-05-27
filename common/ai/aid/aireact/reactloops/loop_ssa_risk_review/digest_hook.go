package loop_ssa_risk_review

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/utils"
)

func buildRiskReviewPostIterationDigestHook() reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, _ int, _ aicommon.AIStatefulTask, _ bool, _ any, _ *reactloops.OnPostIterationOperator) {
		if loop == nil {
			return
		}
		la := loop.GetLastAction()
		if la == nil || la.ActionType != "directly_answer" {
			return
		}
		payload := strings.TrimSpace(loop.Get("directly_answer_payload"))
		if payload == "" {
			payload = strings.TrimSpace(loop.Get("tag_final_answer"))
		}
		if payload == "" {
			return
		}
		rid := strings.TrimSpace(loop.Get(sfu.LoopVarSSARiskID))
		prev := strings.TrimSpace(loop.Get(sfu.LoopVarSSARiskReviewDigest))
		var sb strings.Builder
		if prev != "" {
			sb.WriteString(prev)
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("### SSA Risk %s\n\n%s", rid, payload))
		loop.Set(sfu.LoopVarSSARiskReviewDigest, utils.ShrinkTextBlock(sb.String(), 64000))
	})
}
