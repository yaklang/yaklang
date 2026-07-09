package subagent

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
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
func RunNestedLoop(
	invoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	scopeName string,
	loopName string,
	configure func(subLoop *reactloops.ReActLoop),
	opts ...reactloops.ReActLoopOption,
) (*reactloops.ReActLoop, error) {
	if invoker == nil {
		return nil, utils.Error("invoker is nil")
	}
	if parentTask == nil {
		return nil, utils.Error("parent task is nil")
	}

	var timeline *aicommon.Timeline
	var checkpoint int64
	if cfg := invoker.GetConfig(); cfg != nil {
		if c, ok := cfg.(*aicommon.Config); ok && c.Timeline != nil {
			timeline = c.Timeline
			checkpoint = timeline.GetMaxID()
			defer func() {
				removed := countTimelineIDsAfter(timeline, checkpoint)
				timeline.TruncateAfter(checkpoint)
				if removed > 0 {
					log.Infof("[SubAgent] nested loop %s timeline rollback: removed %d entries", scopeName, removed)
				}
			}()
		}
	}

	factory, ok := reactloops.GetLoopFactory(loopName)
	if !ok || factory == nil {
		return nil, utils.Errorf("reactloop[%s] not found", loopName)
	}

	var restoreEmitter func()
	if cfg, ok := invoker.GetConfig().(*aicommon.Config); ok && cfg != nil {
		if parentTask.GetEmitter() != nil {
			restoreEmitter = cfg.SwapEmitter(parentTask.GetEmitter())
		}
	}
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

	prevInvokerTask := invoker.GetCurrentTask()
	var prevParentLoop *reactloops.ReActLoop
	if prevInvokerTask != nil {
		if parent, ok := prevInvokerTask.GetReActLoop().(*reactloops.ReActLoop); ok {
			prevParentLoop = parent
		}
	}
	invoker.SetCurrentTask(nestedTask)
	defer invoker.SetCurrentTask(prevInvokerTask)
	defer func() {
		if prevParentLoop != nil {
			prevParentLoop.SetCurrentTask(prevInvokerTask)
		}
	}()

	if err := subLoop.ExecuteWithExistedTask(nestedTask); err != nil {
		return subLoop, utils.Wrap(err, "execute nested sub-loop")
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
