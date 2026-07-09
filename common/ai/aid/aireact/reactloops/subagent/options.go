package subagent

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	// DepthLoopVar marks forked sub-loop instances so nested dispatch is blocked.
	DepthLoopVar = "sub_agent_depth"

	defaultForkConcurrency = 5
	maxForkConcurrency     = 10
)

// DefaultForkOptions returns loop options applied to programmatic forked sub-agents.
func DefaultForkOptions() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		reactloops.WithVar(DepthLoopVar, 1),
		reactloops.WithNoEndLoadingStatus(true),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS
		}),
	}
}

func normalizeForkConcurrency(concurrency, jobCount int) int {
	if concurrency <= 0 {
		concurrency = defaultForkConcurrency
		if jobCount < concurrency {
			concurrency = jobCount
		}
	}
	if concurrency > maxForkConcurrency {
		concurrency = maxForkConcurrency
	}
	if concurrency > jobCount {
		concurrency = jobCount
	}
	return concurrency
}
