package loop_syntaxflow_scan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// EmitSyntaxFlowScanProgress 在引擎内同步阶段：顶栏 loading（EmitStatus）+ 结构化事件（EVENT_TYPE_STRUCTURED, nodeId=syntaxflow_scan_progress）。
func EmitSyntaxFlowScanProgress(loop *reactloops.ReActLoop, phase, message, taskID, err string) {
	EmitSyntaxFlowScanPhase(loop, 0, "", phase, message, taskID, err, nil)
}

// EmitSyntaxFlowScanPhase 三阶段编排：step=1 编译、2 扫描、3 风险轮询/解读；stage=start|end|tick。
// extra 可带 program_name、risk_count、status 等结构化附字段。
func EmitSyntaxFlowScanPhase(loop *reactloops.ReActLoop, step int, stage, phase, message, taskID, err string, extra map[string]any) {
	if loop == nil {
		return
	}
	statusLine := message
	if statusLine == "" {
		statusLine = phase
	}
	loop.LoadingStatus("SyntaxFlow: " + statusLine)

	em := loop.GetEmitter()
	if em == nil {
		return
	}
	m := map[string]any{
		"loop":    "syntaxflow_scan",
		"phase":   phase,
		"message": message,
	}
	if step > 0 {
		m["step"] = step
	}
	if stage != "" {
		m["stage"] = stage
	}
	if taskID != "" {
		m["task_id"] = taskID
	}
	if err != "" {
		m["error"] = err
	}
	for k, v := range extra {
		m[k] = v
	}
	if _, e := em.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "syntaxflow_scan_progress", m); e != nil {
		log.Debugf("syntaxflow_scan_progress emit: %v", e)
	}
}
