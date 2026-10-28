package aireact

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// The existing execution-owner finalizers call this once user work has exited.
// Capture only the task's last constructed prompt; never inspect UI results.
func (r *ReAct) collectTerminalTrajectory(task aicommon.AIStatefulTask) {
	defer aicommon.RecoverTrajectoryCollection()
	if r != nil && r.config != nil {
		r.config.CollectTerminalTrajectory(task)
	}
}
