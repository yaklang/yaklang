package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
	"time"
)

// EnableExecution is called by the full Session, never the plan-only adapter.
// One pump owns admission; model actions do not drive the DAG.
func (c *Controller) EnableExecution(manual bool, reviewInterval time.Duration) {
	c.mu.Lock()
	c.automatic = true
	c.manualReview = manual
	c.reviewInterval = reviewInterval
	c.mu.Unlock()
	c.recordApproval()
	c.schedule()
	for _, a := range c.Snapshot().Attempts {
		if a.State == AwaitingReview {
			c.beginTaskReview(a)
		}
	}
}

func (c *Controller) recordApproval() {
	c.mu.Lock()
	cfg, ok := c.resultConfig.(interface{ GetTimeline() *aicommon.Timeline })
	if !ok || c.state.Phase != PhaseExec || c.state.ApprovalRecorded {
		c.mu.Unlock()
		return
	}
	p := clone(c.state.Plan)
	c.state.ApprovalRecorded = true
	c.notifyLocked()
	c.mu.Unlock()
	data, _ := json.Marshal(p)
	cfg.GetTimeline().PushText(c.resultConfig.AcquireId(), "[PLAN_APPROVED_HISTORY] 已批准的内容，仅作历史事实；当前计划以 PLAN DOCUMENT/DEFINITION 为准。\n%s", string(data))
	c.publish()
}

func (c *Controller) schedule() {
	c.mu.Lock()
	if !c.automatic || c.editing || c.closed || c.ctx.Err() != nil || c.state.Phase != PhaseExec || c.state.Finished || c.state.Plan == nil || c.resultErr != nil {
		c.mu.Unlock()
		return
	}
	var ids []string
	for _, task := range c.state.Plan.Tasks {
		if len(c.workers)+len(ids) >= c.concurrency {
			break
		}
		if c.readyLocked(c.state.Attempts[task.ID]) {
			ids = append(ids, task.ID)
		}
	}
	if len(ids) == 0 {
		blocked := []string{}
		waitingReview := false
		for _, a := range c.state.Attempts {
			waitingReview = waitingReview || a.State == AwaitingReview
			if a.State == Pending || a.State == Failed || a.State == Rejected {
				blocked = append(blocked, fmt.Sprintf("%s:%d:%s", a.Task.ID, a.ID, a.State))
			}
		}
		if len(c.workers) == 0 && !waitingReview && len(blocked) > 0 {
			c.enqueueLocked("scheduler_blocked", "", 0, "任务图尚未解决，需要修复、重试、调整或明确取消。", blocked, true)
		}
		c.mu.Unlock()
		c.publish()
		return
	}
	started, contexts := c.startLocked(ids)
	c.mu.Unlock()
	c.launch(started, contexts)
}

// Human review runs after worker cleanup with the Session context. It owns no
// execution slot and cannot be cancelled by the completed worker's context.
func (c *Controller) beginTaskReview(a Attempt) {
	c.mu.Lock()
	if !c.automatic || !c.manualReview || c.closed || c.state.Attempts[a.Task.ID].ID != a.ID || c.state.Attempts[a.Task.ID].State != AwaitingReview {
		c.mu.Unlock()
		return
	}
	key := fmt.Sprintf("%s:%d", a.Task.ID, a.ID)
	if c.reviewJobs[key] != nil {
		c.mu.Unlock()
		return
	}
	host, ok := c.host.(interface {
		ReviewExecutionTask(context.Context, Attempt) error
	})
	if !ok {
		c.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(c.ctx)
	if c.reviewJobs == nil {
		c.reviewJobs = make(map[string]context.CancelFunc)
	}
	c.reviewJobs[key] = cancel
	c.owned.Add(1)
	c.mu.Unlock()
	go func() {
		defer c.owned.Done()
		defer cancel()
		err := host.ReviewExecutionTask(ctx, a)
		c.mu.Lock()
		delete(c.reviewJobs, key)
		c.notifyLocked()
		if err != nil && ctx.Err() == nil && !c.closed {
			c.enqueueLocked("user_message", a.Task.ID, a.ID, "任务审核需要协调员处理："+err.Error(), nil, true)
		}
		c.mu.Unlock()
		c.publish()
	}()
}

func (c *Controller) archiveLocked(a Attempt) {
	if a.ID == 0 {
		return
	}
	if c.state.History == nil {
		c.state.History = make(map[string][]Attempt)
	}
	list := c.state.History[a.Task.ID]
	for i, previous := range list {
		if previous.ID == a.ID {
			list[i] = clone(a)
			c.state.History[a.Task.ID] = list
			return
		}
	}
	c.state.History[a.Task.ID] = append(list, clone(a))
}

func (c *Controller) applyReview(id string, attemptID uint64, decision, reason string, human bool) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("review requires an evidence-backed reason")
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	if err := c.checkExecLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.automatic && c.manualReview && !human {
		c.mu.Unlock()
		return fmt.Errorf("manual task review is owned by the task's user endpoint")
	}
	a, ok := c.state.Attempts[id]
	if !ok || a.ID != attemptID || (a.State != AwaitingReview && !(decision == "cancel" && (a.State == Failed || a.State == Rejected))) {
		c.mu.Unlock()
		return fmt.Errorf("review requires the current settled attempt")
	}
	switch decision {
	case "accept":
		a.State = Accepted
	case "reject", "deepen":
		a.State = Rejected
	case "cancel":
		a.State = Cancelled
	default:
		c.mu.Unlock()
		return fmt.Errorf("review decision must be accept/reject/deepen/cancel")
	}
	a.ReviewReason = reason
	c.state.Attempts[id] = a
	c.archiveLocked(a)
	c.queueResultLocked(a)
	c.enqueueLocked("task_reviewed", id, attemptID, reason, nil, a.State == Rejected)
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	c.schedule()
	return nil
}
