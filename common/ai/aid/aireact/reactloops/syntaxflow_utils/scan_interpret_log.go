package syntaxflow_utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// LoopVarSFInterpretLog is appended by SyntaxFlow scan interpret / poll hooks for final report traceability.
	LoopVarSFInterpretLog = "sf_scan_interpret_log"
	loopVarInterpretTick  = "sf_scan_interpret_tick_index"
)

// AppendSfScanInterpretLog appends one line of interpret/poll tracing to the loop and a short timeline entry.
func AppendSfScanInterpretLog(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, taskID, line string) {
	if loop == nil || strings.TrimSpace(line) == "" {
		return
	}
	prev := loop.Get(LoopVarSFInterpretLog)
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
	loop.Set(LoopVarSFInterpretLog, s)
	if r != nil {
		r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(entry, 2000))
	}
}
