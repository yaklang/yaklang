package coordinator

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func (c *Controller) eventCursor() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.eventRevision
}

// contextSnapshot binds the wait cursor to the same published state used by
// this decision. A notification arriving during inference cannot be swallowed.
func (c *Controller) contextSnapshot() (Snapshot, uint64) {
	for {
		c.publish()
		c.mu.Lock()
		if c.closed || c.state.Revision <= c.published {
			s, revision := clone(c.state), c.eventRevision
			c.mu.Unlock()
			return s, revision
		}
		c.mu.Unlock()
	}
}

// taskDiscovered runs after session evidence is saved. Old attempts and closed
// sessions cannot wake the current planner. It is not a user intervention.
func (c *Controller) taskDiscovered(taskID string, attemptID uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.state.Attempts[taskID]
	if !c.closed && ok && a.ID == attemptID && a.State == Running {
		c.signalLocked()
		if c.discoveries == nil {
			c.discoveries = make(map[string]uint64)
		}
		c.discoveries[taskID] = c.eventRevision
	}
}

// idleLocked only suspends when workers are the remaining source of progress.
// Review, failures, changed drafts and available DAG slots belong to the planner.
func (c *Controller) idleLocked() bool {
	if c.reviewing || c.state.Approved == nil || c.state.DraftVersion != c.state.ApprovedVersion || len(c.workers) == 0 {
		return false
	}
	for _, a := range c.state.Attempts {
		switch a.State {
		case AwaitingReview, Failed, Rejected, Cancelled:
			return false
		case Pending:
			if len(c.workers) < c.concurrency && c.readyLocked(a) {
				return false
			}
		}
	}
	return true
}

// waitWhenIdle has no polling timer and performs no AI call. Events are already
// durable when their notification is published; the cursor also catches events
// that occurred before this wait began. Caller cancellation always releases it.
func (c *Controller) waitWhenIdle(ctx context.Context, after uint64, onWait func()) error {
	for {
		c.mu.Lock()
		if err := c.checkLocked(); err != nil {
			c.mu.Unlock()
			return err
		}
		if c.eventRevision != after || !c.idleLocked() {
			c.mu.Unlock()
			c.publish()
			return nil
		}
		changed := c.changed
		c.mu.Unlock()
		if onWait != nil {
			onWait()
			onWait = nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-changed:
		}
	}
}

func configureAutomaticWait(loop *reactloops.ReActLoop) {
	reactloops.WithOnPostIteraction(func(l *reactloops.ReActLoop, _ int, task aicommon.AIStatefulTask, done bool, _ any, op *reactloops.OnPostIterationOperator) {
		if done || planningOnly(l) || len(aicommon.GetBlockingVerificationTodoItems(l.GetConfig(), task)) > 0 {
			return
		}
		op.DeferAfterCallbacks(func() {
			if op.ShouldEndIteration() {
				return
			}
			after, _ := l.GetVariable("coordinator_event_revision").(uint64)
			if err := controller(l).waitWhenIdle(task.GetContext(), after, func() {
				l.UserStatus("正在等待子任务的新发现或状态变化", "Waiting for task discoveries or state changes", aicommon.WithStatusCode("plan.waiting_for_tasks"))
			}); err != nil {
				l.Set("coordinator_wait_error", err)
				op.EndIteration(err)
			}
		})
	})(loop)
}

// Only successful evidence changes notify; streams and ordinary tool activity
// stay on their existing UI channels without waking the planner.
func configureWorkerDiscovery(loop *reactloops.ReActLoop) error {
	action, err := loop.GetActionHandler("save_evidence")
	if err != nil {
		return err
	}
	wrapped := *action
	wrapped.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		before := l.GetConfig().GetSessionEvidenceRendered()
		action.ActionHandler(l, a, op)
		if done, _ := op.IsTerminated(); done {
			return
		}
		if callback, ok := l.GetVariable("coordinator_discovery_callback").(func()); ok && l.GetConfig().GetSessionEvidenceRendered() != before {
			callback()
		}
	}
	wrapped.FunctionCallAction = nil
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	return nil
}
