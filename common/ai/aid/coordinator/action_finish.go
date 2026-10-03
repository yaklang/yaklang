package coordinator

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func configureCoordinatorFinish(loop *reactloops.ReActLoop) error {
	finish, err := loop.GetActionHandler("finish")
	if err != nil {
		return err
	}
	wrapped := *finish
	original := finish.ActionHandler
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		if !planningOnly(l) && controller(l).Snapshot().Phase == PhaseExec {
			op.Feedback("EXEC 由宿主在 submit_report 后检查收尾，主模型不能 finish。")
			op.Continue()
			return
		}
		check := controller(l).CanFinish
		if planningOnly(l) {
			check = controller(l).CanFinishPlanning
		}
		if err := check(); err != nil {
			c := controller(l)
			c.mu.Lock()
			idle := !planningOnly(l) && c.idleLocked()
			c.mu.Unlock()
			if idle {
				// The post-iteration hook waits locally; this is not a rejected
				// action or a new Evidence record on every waiting turn.
				op.Feedback("子任务仍在执行；系统自动等待通知，之后继续检查。")
				op.Continue()
				return
			}
			continueAfterFinishRejection(l, a, op, err)
			return
		}
		gate := reactloops.NewActionHandlerOperator(op.GetTask())
		original(l, a, gate)
		if done, err := gate.IsTerminated(); !done {
			continueAfterFinishRejection(l, a, op, fmt.Errorf("%s", gate.GetFeedback().String()))
			return
		} else if err != nil {
			if recordActionOutcome(l, a, op, "finish", nil, err) {
				op.Fail(err)
			}
			return
		}
		observed, _ := l.GetVariable("coordinator_observed_user_revision").(uint64)
		finalize := controller(l).Finalize
		if planningOnly(l) {
			finalize = controller(l).FinalizePlanning
		}
		if err := finalize(observed); err != nil {
			continueAfterFinishRejection(l, a, op, err)
			return
		}
		if recordActionOutcome(l, a, op, "finish", nil, nil) {
			op.Exit()
		}
	}
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	return nil
}

func configureWorkerFinish(loop *reactloops.ReActLoop) error {
	finish, err := loop.GetActionHandler("finish")
	if err != nil {
		return err
	}
	wrapped := *finish
	original := finish.ActionHandler
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		if _, ok := l.GetVariable("coordinator_task_result").(Result); !ok {
			continueAfterFinishRejection(l, a, op, fmt.Errorf("submit_task_result is required before finishing task %s", op.GetTask().GetId()))
			return
		}
		gate := reactloops.NewActionHandlerOperator(op.GetTask())
		original(l, a, gate)
		if done, err := gate.IsTerminated(); !done {
			continueAfterFinishRejection(l, a, op, fmt.Errorf("%s", gate.GetFeedback().String()))
		} else if err != nil {
			if recordActionOutcome(l, a, op, "finish", nil, err) {
				op.Fail(err)
			}
		} else if recordActionOutcome(l, a, op, "finish", nil, nil) {
			op.Exit()
		}
	}
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	return nil
}

func continueAfterFinishRejection(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator, err error) {
	if recordActionOutcome(l, a, op, "finish", nil, err) {
		op.Continue()
	}
}
