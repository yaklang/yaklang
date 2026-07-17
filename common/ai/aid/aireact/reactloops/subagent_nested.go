package reactloops

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// nestedSubTask is a short-lived inner task for nested loops (e.g. fast_context).
// It keeps parent TaskId/UUID for UI aggregation while using an isolated context
// so the inner loop finishing does not cancel the parent scan.
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

// RunNestedLoop executes a registered sub-loop under the parent's TaskId without
// creating a new UI card. Timeline entries created during the run are rolled back.
//
// It is the no-registry counterpart of runNestedInPlace: it shares the parent
// timeline (rolling back entries afterwards), forwards the parent task emitter,
// swaps the invoker's current task to a nested sub-task for the run, and leaves a
// rollback checkpoint. It differs from runNestedInPlace only in that it does not
// register a SubAgentHandle (callers that need the stall-heartbeat bypass should
// use RunNestedJobWithProgress / RunNestedJobsConcurrentlyWithProgress instead).
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
