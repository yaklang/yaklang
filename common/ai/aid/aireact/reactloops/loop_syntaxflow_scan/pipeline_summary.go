package loop_syntaxflow_scan

import (
	"strings"

	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// AppendSFPipelineLine appends one line to sf_scan_pipeline_summary (compile / scan / overview ticks / 终态).
func AppendSFPipelineLine(loop *reactloops.ReActLoop, line string) {
	if loop == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	prev := strings.TrimSpace(loop.Get(sfutil.LoopVarSFPipelineSummary))
	if prev == "" {
		loop.Set(sfutil.LoopVarSFPipelineSummary, line)
		return
	}
	loop.Set(sfutil.LoopVarSFPipelineSummary, prev+"\n"+line)
}
