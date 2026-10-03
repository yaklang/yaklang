package coordinator

import (
	"context"
	"fmt"
	"reflect"
)

// changed closure is computed in both old and candidate DAGs. Only affected
// attempts are stopped; unrelated running tasks keep their original context.
func affectedTasks(old, next *Plan) map[string]bool {
	before, after := map[string]Task{}, map[string]Task{}
	for _, t := range old.Tasks {
		before[t.ID] = t
	}
	for _, t := range next.Tasks {
		after[t.ID] = t
	}
	affected := map[string]bool{}
	equal := func(a, b Task) bool { a.Index = ""; b.Index = ""; return reflect.DeepEqual(a, b) }
	for id, t := range before {
		if u, ok := after[id]; !ok || !equal(t, u) {
			affected[id] = true
		}
	}
	for id, t := range after {
		if u, ok := before[id]; !ok || !equal(t, u) {
			affected[id] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, p := range []*Plan{old, next} {
			for _, t := range p.Tasks {
				for _, dep := range t.DependsOn {
					if affected[dep] && !affected[t.ID] {
						affected[t.ID] = true
						changed = true
					}
				}
			}
		}
	}
	return affected
}

func (c *Controller) modifyExecutingPlan(ctx context.Context, params map[string]any) (PlanEditReceipt, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	if err := c.checkExecLocked(); err != nil {
		c.mu.Unlock()
		return PlanEditReceipt{}, err
	}
	old := clone(c.state.Plan)
	c.mu.Unlock()
	next, receipt, err := applyPlanEdit(old, params, c.patchDir)
	if err != nil || receipt.Status == "unchanged" {
		return receipt, err
	}
	affected := affectedTasks(old, next)
	c.mu.Lock()
	c.editing = true
	var cancels []context.CancelFunc
	for id := range affected {
		if cancel := c.workers[id]; cancel != nil {
			a := c.state.Attempts[id]
			a.State = Cancelling
			a.ReviewReason = "当前任务定义或必要依赖变更，停止本次冻结输入。"
			c.state.Attempts[id] = a
			cancels = append(cancels, cancel)
		}
		if a, ok := c.state.Attempts[id]; ok {
			if cancel := c.reviewJobs[fmt.Sprintf("%s:%d", id, a.ID)]; cancel != nil {
				cancels = append(cancels, cancel)
			}
		}
	}
	c.notifyLocked()
	c.mu.Unlock()
	committed := false
	defer func() {
		c.mu.Lock()
		if !committed {
			for id := range affected {
				a := c.state.Attempts[id]
				if a.State == Cancelled && a.ReviewReason == "当前任务定义或必要依赖变更，停止本次冻结输入。" {
					a.State = Failed
					a.Result.Error = "任务已停止，但计划修改未提交；保留原定义，需要明确重试。"
					c.state.Attempts[id] = a
					c.archiveLocked(a)
					c.queueResultLocked(a)
					c.enqueueLocked("task_settled", id, a.ID, a.Result.Error, nil, true)
				}
			}
		}
		c.editing = false
		c.notifyLocked()
		c.mu.Unlock()
		c.publish()
		c.schedule()
	}()
	for _, cancel := range cancels {
		cancel()
	}
	c.publish()
	for {
		c.mu.Lock()
		active := false
		for id := range affected {
			active = active || c.workers[id] != nil
		}
		changed := c.changed
		c.mu.Unlock()
		if !active {
			break
		}
		select {
		case <-ctx.Done():
			return receipt, ctx.Err()
		case <-c.ctx.Done():
			return receipt, c.ctx.Err()
		case <-changed:
		}
	}
	// Serialize persistence publication, but never hold the scheduler mutex for
	// slow storage. Worker settlement can still update the unaffected branches.
	c.publishMu.Lock()
	c.mu.Lock()
	if err := c.checkExecLocked(); err != nil {
		c.mu.Unlock()
		c.publishMu.Unlock()
		return receipt, err
	}
	candidate := clone(c.state)
	candidate.Plan = next
	candidate.Finished = false
	candidate.Report.Submitted = false
	candidate.Attempts = make(map[string]Attempt, len(next.Tasks))
	for _, task := range next.Tasks {
		a, ok := c.state.Attempts[task.ID]
		if !ok || affected[task.ID] {
			c.archiveLocked(a)
			a = Attempt{Task: task, State: Pending}
		} else {
			a.Task = task
		}
		candidate.Attempts[task.ID] = a
	}
	for id, a := range c.state.Attempts {
		if affected[id] {
			c.archiveLocked(a)
		}
	}
	candidate.History = clone(c.state.History)
	candidate.Revision++
	c.mu.Unlock()
	err = c.commitPlanLocked(candidate)
	c.mu.Lock()
	if err == nil {
		// Merge controls and settlements received during I/O; do not discard them.
		for id, a := range c.state.Attempts {
			if !affected[id] {
				if _, ok := candidate.Attempts[id]; ok {
					a.Task = candidate.Attempts[id].Task
					candidate.Attempts[id] = a
				}
			}
		}
		for id, records := range c.state.History {
			if !affected[id] {
				candidate.History[id] = clone(records)
			}
		}
		candidate.Inbox = clone(c.state.Inbox)
		candidate.NextMessage = c.state.NextMessage
		candidate.DeliveredThrough = c.state.DeliveredThrough
		candidate.CheckedThrough = c.state.CheckedThrough
		candidate.UserRevision = c.state.UserRevision
		candidate.Revision = c.state.Revision + 1
		c.state = candidate
		committed = true
		c.enqueueLocked("plan_adjusted", "", 0, "当前任务图已原子调整，无需重新审批；受影响尝试及结果保留为历史。", receipt.TaskIDs, false)
	}
	c.mu.Unlock()
	c.publishMu.Unlock()
	if err != nil {
		return receipt, err
	}
	c.publish()
	return receipt, nil
}
