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
func (c *Controller) taskDiscovered(taskID string, attemptID uint64, refs ...string) {
	c.mu.Lock()
	a, ok := c.state.Attempts[taskID]
	if c.closed || !ok || a.ID != attemptID || a.State != Running {
		c.mu.Unlock()
		return
	}
	c.enqueueLocked("task_discovery", taskID, attemptID, "任务保存了新的关键 Evidence，请核对发现及影响。", refs, true)
	if c.discoveries == nil {
		c.discoveries = make(map[string]uint64)
	}
	c.discoveries[taskID] = c.eventRevision + 1
	c.mu.Unlock()
	c.publish()
}

// idleLocked only suspends when workers are the remaining source of progress.
// Review, failures, changed drafts and available DAG slots belong to the planner.
func (c *Controller) idleLocked() bool {
	if c.reviewing || c.state.Phase != PhaseExec || c.state.Plan == nil || len(c.workers) == 0 {
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
		if done {
			return
		}
		op.DeferAfterCallbacks(func() {
			if op.ShouldEndIteration() {
				return
			}
			after, _ := l.GetVariable("coordinator_event_revision").(uint64)
			if controller(l).Snapshot().Phase == PhasePlan {
				if len(aicommon.GetBlockingVerificationTodoItems(l.GetConfig(), task)) > 0 {
					return
				}
				if err := waitForExploration(task.GetContext(), l); err != nil {
					l.Set("coordinator_wait_error", err)
					op.EndIteration(err)
				}
				return
			}
			if planningOnly(l) {
				return
			}
			if controller(l).automatic {
				through, _ := l.GetVariable("coordinator_message_cursor").(uint64)
				d := currentDecision(l)
				controller(l).completeDecision(through, d)
				if len(aicommon.GetBlockingVerificationTodoItems(l.GetConfig(), task)) == 0 && controller(l).Snapshot().Report.Submitted && controller(l).FinalizeReport() == nil {
					op.EndIteration()
					return
				}
				if d.rejected || d.toolResult || (d.continuation && !d.yield) {
					return
				}
				if err := controller(l).waitMessages(task.GetContext(), d.timeout, func() {
					l.UserStatus("正在等待任务或用户消息", "Waiting for messages", aicommon.WithStatusCode("plan.waiting_for_messages"))
				}, false); err != nil {
					l.Set("coordinator_wait_error", err)
					op.EndIteration(err)
				}
				return
			}
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
		if callback, ok := l.GetVariable("coordinator_discovery_callback").(func(string, string) error); ok {
			id, content := a.GetString("evidence_id"), a.GetString("evidence_content")
			if id == "" {
				id = a.GetInvokeParams("next_action").GetString("evidence_id")
			}
			if content == "" {
				content = a.GetInvokeParams("next_action").GetString("evidence_content")
			}
			if err := callback(id, content); err != nil {
				op.Fail(err)
			}
		} else if l.GetConfig().GetSessionEvidenceRendered() != before {
			switch callback := l.GetVariable("coordinator_discovery_callback").(type) {
			case func(...string):
				callback(a.GetString("evidence_id"))
			case func():
				callback()
			}
		}
	}
	wrapped.FunctionCallAction = nil
	reactloops.WithOverrideLoopAction(&wrapped)(loop)
	return nil
}

// Compare this normalized finding, not the whole shared store: another worker's
// concurrent save must not turn an identical finding into a false discovery.
func (s *Session) saveTaskDiscovery(a Attempt, id, content string) error {
	op, err := aicommon.BuildSessionEvidenceUpsert(id, content)
	if err != nil {
		return err
	}
	s.evidenceMu.Lock()
	defer s.evidenceMu.Unlock()
	s.controller.mu.Lock()
	current := s.controller.state.Attempts[a.Task.ID]
	active := !s.controller.closed && current.ID == a.ID && current.State == Running
	s.controller.mu.Unlock()
	if !active {
		return nil
	}
	store := aicommon.UnmarshalEvidenceStore(s.GetSessionPromptState().GetSessionEvidence())
	for _, item := range store.Items {
		if item.ID == op.ID && item.Content == op.Content {
			return nil
		}
	}
	if _, err := reactloops.SaveSessionEvidence(s.Config, op.ID, op.Content); err != nil {
		return err
	}
	s.controller.taskDiscovered(a.Task.ID, a.ID, op.ID)
	return nil
}
