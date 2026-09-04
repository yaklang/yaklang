package reactloops

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// BuildSubAgentInvokerForTest 是仅供测试使用的导出桥，等价于 buildSubAgentInvoker
// （TimelineHandle 为 Fork 模式）。生产代码不得调用。
func BuildSubAgentInvokerForTest(
	parentCfg *aicommon.Config,
	fork *aicommon.TimelineFork,
	taskCtx context.Context,
	taskEmitter *aicommon.Emitter,
) (aicommon.AITaskInvokeRuntime, error) {
	handle := &TimelineHandle{mode: SubAgentTimelineFork, fork: fork, branch: fork.Branch}
	return buildSubAgentInvoker(parentCfg, handle, taskCtx, taskEmitter)
}
