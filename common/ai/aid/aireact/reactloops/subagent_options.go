package reactloops

import (
	"github.com/yaklang/yaklang/common/schema"
)

const (
	defaultForkConcurrency = 5
	maxForkConcurrency     = 10
)

// DefaultForkOptions 返回应用于编程式 fork 子 Agent 的 loop 选项。
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

// normalizeForkConcurrency 将并发数归一化到合法范围。
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
