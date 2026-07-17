package reactloops

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// nestedSubTask 是 nested loop（如 fast_context）使用的短生命周期内部任务。
// 它保留父任务的 TaskId/UUID 供 UI 聚合，同时使用独立的 context，使内部 loop
// 结束不会取消父扫描。
type nestedSubTask struct {
	*aicommon.AIStatefulTaskBase
	parent aicommon.AIStatefulTask
}

func newNestedSubTask(parent aicommon.AIStatefulTask, scopeName string) *nestedSubTask {
	parentCtx := parent.GetContext()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	childCtx, childCancel := context.WithCancel(parentCtx)
	internalID := fmt.Sprintf("%s@nested:%s", parent.GetId(), scopeName)
	inner := aicommon.NewSubTaskBaseWithOptions(
		parent,
		internalID,
		parent.GetUserInput(),
		aicommon.WithStatefulTaskBaseName(scopeName),
		aicommon.WithStatefulTaskBaseContextAndCancel(childCtx, childCancel),
		aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit(),
	)
	if parent.GetEmitter() != nil {
		inner.SetEmitter(parent.GetEmitter())
	}
	return &nestedSubTask{AIStatefulTaskBase: inner, parent: parent}
}

func (n *nestedSubTask) EmitterTaskBase() *aicommon.AIStatefulTaskBase {
	if n == nil {
		return nil
	}
	return n.AIStatefulTaskBase
}

func (n *nestedSubTask) GetId() string {
	if n == nil || n.parent == nil {
		return ""
	}
	return n.parent.GetId()
}

func (n *nestedSubTask) GetIndex() string {
	return n.GetId()
}

func (n *nestedSubTask) GetUUID() string {
	if n == nil || n.parent == nil {
		return ""
	}
	return n.parent.GetUUID()
}

// RunNestedLoop 在父任务的 TaskId 下执行一个已注册的子 loop，不创建新的 UI 卡片。
// 运行期间产生的 timeline 条目会在结束后回滚。
//
// 它是 runNestedInPlace 的无-registry 版本：共享父 timeline（结束后回滚条目）、
// 转发父任务 emitter、把 invoker 的 current task 切换为 nested 子任务，并保留一个
// 回滚 checkpoint。与 runNestedInPlace 的唯一区别是不注册 SubAgentHandle——需要
// stall-heartbeat 旁路的调用方应改用 RunNestedJobWithProgress /
// RunNestedJobsConcurrentlyWithProgress。
func RunNestedLoop(
	invoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	scopeName string,
	loopName string,
	configure func(subLoop *ReActLoop),
	opts ...ReActLoopOption,
) (*ReActLoop, error) {
	if invoker == nil {
		return nil, utils.Error("invoker is nil")
	}
	if parentTask == nil {
		return nil, utils.Error("parent task is nil")
	}
	factory, ok := GetLoopFactory(loopName)
	if !ok || factory == nil {
		return nil, utils.Errorf("reactloop[%s] not found", loopName)
	}

	defer timelineRollbackCheckpoint(invoker)()

	restoreEmitter := swapEmitterForRun(invoker, parentTask)
	if restoreEmitter != nil {
		defer restoreEmitter()
	}

	subLoop, err := factory(invoker, opts...)
	if err != nil {
		return nil, utils.Wrap(err, "create nested sub-loop")
	}
	if configure != nil {
		configure(subLoop)
	}

	nestedTask := newNestedSubTask(parentTask, scopeName)
	execErr := runWithCurrentTask(invoker, nestedTask, func() error {
		return subLoop.ExecuteWithExistedTask(nestedTask)
	})
	if execErr != nil {
		return subLoop, utils.Wrap(execErr, "execute nested sub-loop")
	}
	return subLoop, nil
}

// countTimelineIDsAfter 统计 timeline 中 id 大于 checkpoint 的条目数量。
func countTimelineIDsAfter(timeline *aicommon.Timeline, checkpoint int64) int {
	if timeline == nil {
		return 0
	}
	count := 0
	for _, id := range timeline.GetTimelineItemIDs() {
		if id > checkpoint {
			count++
		}
	}
	return count
}
