package loop_ssa_risk_review

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		id, ok := sfu.SSARiskID(task, loop)
		if !ok {
			log.Warnf("[ssa_risk_review] could not resolve risk_id from loop vars or attachments")
			r.AddToTimeline("ssa_risk_review",
				"未解析到 SSA risk_id。请附加 irify_ssa_risk/risk_id（十进制），或设置 Loop 变量 ssa_risk_id。")
			loop.Set("ssa_risk_id", "")
			op.Continue()
			return
		}
		loop.Set("ssa_risk_id", fmt.Sprintf("%d", id))
		r.AddToTimeline("ssa_risk_review", fmt.Sprintf("目标 SSA Risk ID: %d。请先使用 ssa-risk 工具拉取该条风险（risk_id=%d, get_full_code 视需要设为 true），再输出解读与建议处置。", id, id))
		op.Continue()
	}
}
