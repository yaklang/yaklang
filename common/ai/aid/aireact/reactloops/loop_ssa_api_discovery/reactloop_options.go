package loop_ssa_api_discovery

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const defaultReActMaxIterations = 100

// ssaDiscoveryMaxIterations returns ReAct max iterations from AI config
// (frontend ReActMaxIteration; system default 100).
func ssaDiscoveryMaxIterations(r aicommon.AIInvokeRuntime) int {
	if r == nil {
		return defaultReActMaxIterations
	}
	return maxIterationsFromConfig(r.GetConfig())
}

func maxIterationsFromConfig(cfg aicommon.AICallerConfigIf) int {
	if cfg == nil {
		return defaultReActMaxIterations
	}
	n := int(cfg.GetMaxIterationCount())
	if n > 0 {
		return n
	}
	return defaultReActMaxIterations
}
