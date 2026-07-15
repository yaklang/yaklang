package reactloops

import (
	"github.com/yaklang/yaklang/common/schema"
)

const (
	defaultForkConcurrency = 5
	maxForkConcurrency     = 10
)

// DefaultForkOptions returns loop options applied to programmatic forked sub-agents.
func DefaultForkOptions() []ReActLoopOption {
	return []ReActLoopOption{
		WithVar(SubAgentDepthLoopVar, 1),
		WithNoEndLoadingStatus(true),
		WithAllowPlanAndExec(false),
		WithAllowAIForge(false),
		WithActionFilter(func(action *LoopAction) bool {
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
