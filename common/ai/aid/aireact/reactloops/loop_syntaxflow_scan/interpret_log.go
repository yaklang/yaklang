package loop_syntaxflow_scan

import (
	"fmt"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// LoopVarInterpretLog 逐次解读/轮询的追加日志，供终局「完整报告」引用。
	LoopVarInterpretLog  = "sf_scan_interpret_log"
	loopVarInterpretTick = "sf_scan_interpret_tick_index"
)

// AppendSfScanInterpretLog 追加一条可持久化在 Loop 上的解读记录，并写一条节流到时间线。
func AppendSfScanInterpretLog(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, taskID, line string) {
	if loop == nil || line == "" {
		return
	}
	prev := loop.Get(LoopVarInterpretLog)
	tick := 0
	if ts := loop.Get(loopVarInterpretTick); ts != "" {
		tick, _ = strconv.Atoi(ts)
	}
	tick++
	loop.Set(loopVarInterpretTick, strconv.Itoa(tick))
	entry := fmt.Sprintf("[%s] #%d task_id=%s %s\n", time.Now().Format(time.RFC3339), tick, taskID, line)
	s := prev + entry
	const maxRune = 200000
	if len(s) > maxRune*4 {
		s = s[len(s)-maxRune*4:]
	}
	loop.Set(LoopVarInterpretLog, s)
	if r != nil {
		r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(entry, 2000))
	}
}
