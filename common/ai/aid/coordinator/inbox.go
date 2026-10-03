package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"sort"
	"time"
)

// Messages reference the canonical Timeline/result store; they never carry a
// second plan or large result. Delivery and business resolution are distinct.
type Message struct {
	ID            string   `json:"id"`
	Sequence      uint64   `json:"sequence"`
	CoordinatorID string   `json:"coordinator_id"`
	Type          string   `json:"type"`
	TaskID        string   `json:"task_id,omitempty"`
	AttemptID     uint64   `json:"attempt_id,omitempty"`
	Summary       string   `json:"summary"`
	References    []string `json:"references,omitempty"`
	NeedsDecision bool     `json:"needs_decision"`
}

func (c *Controller) enqueueLocked(kind, task string, attempt uint64, summary string, refs []string, decision bool) {
	if c.closed {
		return
	}
	// Only pending reminders/progress are coalesced. Distinct discoveries and
	// terminal/user facts are never evicted or overwritten under pressure.
	if kind == "review_due" || kind == "scheduler_blocked" {
		for _, m := range c.state.Inbox {
			if m.Type == kind && m.TaskID == task && m.AttemptID == attempt && m.Sequence > c.state.CheckedThrough {
				return
			}
		}
	}
	c.state.NextMessage++
	id := "coordinator"
	if c.resultConfig != nil {
		id = c.resultConfig.GetRuntimeId()
	}
	c.state.Inbox = append(c.state.Inbox, Message{ID: fmt.Sprintf("%s:message:%d", id, c.state.NextMessage), Sequence: c.state.NextMessage, CoordinatorID: id, Type: kind, TaskID: task, AttemptID: attempt, Summary: boundedText(summary, 600), References: refs, NeedsDecision: decision})
	if decision {
		c.state.Report.Submitted = false
		c.state.Finished = false
	}
	c.notifyLocked()
}

func boundedText(s string, limit int) string {
	r := []rune(s)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

func (c *Controller) deliverMessages() (uint64, error) {
	c.publish()
	c.mu.Lock()
	var batch []Message
	for _, m := range c.state.Inbox {
		if m.Sequence > c.state.DeliveredThrough {
			batch = append(batch, m)
		}
	}
	through := c.state.NextMessage
	cfg := c.resultConfig
	c.mu.Unlock()
	if len(batch) == 0 {
		return through, nil
	}
	priority := func(kind string) int {
		switch kind {
		case "user_message":
			return 0
		case "task_settled", "scheduler_blocked":
			return 1
		case "task_discovery":
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(batch, func(i, j int) bool { return priority(batch[i].Type) < priority(batch[j].Type) })
	data, err := json.Marshal(batch)
	if err != nil {
		return 0, err
	}
	if sink, ok := cfg.(interface{ GetTimeline() *aicommon.Timeline }); ok {
		sink.GetTimeline().PushText(cfg.AcquireId(), "[COORDINATOR_INBOX_BATCH]\n%s", string(data))
	}
	c.mu.Lock()
	if through > c.state.DeliveredThrough {
		c.state.DeliveredThrough = through
		c.notifyLocked()
	}
	c.mu.Unlock()
	c.publish()
	return through, nil
}

// CheckedThrough acknowledges a completed decision boundary only. It never
// accepts a task: awaiting_review/failed/rejected still block completion.
func (c *Controller) checkedMessages(through uint64) {
	c.mu.Lock()
	if through > c.state.CheckedThrough && through <= c.state.DeliveredThrough {
		c.state.CheckedThrough = through
		kept := c.state.Inbox[:0]
		for _, m := range c.state.Inbox {
			if m.Sequence > through {
				kept = append(kept, m)
			}
		}
		c.state.Inbox = kept
		c.notifyLocked()
	}
	c.mu.Unlock()
	c.publish()
}

func (c *Controller) actionableLocked() bool {
	for _, m := range c.state.Inbox {
		if m.NeedsDecision && m.Sequence > c.state.CheckedThrough {
			return true
		}
	}
	for _, a := range c.state.Attempts {
		if a.State == Failed || a.State == Rejected || (a.State == AwaitingReview && !c.manualReview) {
			return true
		}
	}
	return c.canFinishLocked() == nil // report work, rather than model-controlled finish
}

func (c *Controller) reviewDueLocked(now time.Time) {
	if c.reviewInterval <= 0 {
		return
	}
	for id, due := range c.reviewAt {
		a := c.state.Attempts[id]
		if a.State == Running && !now.Before(due) {
			c.enqueueLocked("review_due", id, a.ID, "长任务到期检查：核对阻塞、执行深度及是否需要干预；这不是验收。", nil, true)
			c.reviewAt[id] = now.Add(c.reviewInterval)
		}
	}
}

// Explicit and automatic waits share this path. Empty timeouts grant the host
// a check opportunity without calling the model, cancelling work or exiting.
func (c *Controller) WaitMessages(ctx context.Context, timeout time.Duration, onWait func()) error {
	return c.waitMessages(ctx, timeout, onWait, true)
}

func (c *Controller) waitMessages(ctx context.Context, timeout time.Duration, onWait func(), explicit bool) error {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	for {
		c.mu.Lock()
		if err := c.checkExecLocked(); err != nil {
			c.mu.Unlock()
			return err
		}
		c.reviewDueLocked(time.Now())
		// An explicit action yields for any queued batch. Automatic idling may
		// sleep through already-handled manual acceptances, avoiding model reviews.
		if c.actionableLocked() || (explicit && len(c.state.Inbox) > 0) {
			c.mu.Unlock()
			c.publish()
			return nil
		}
		changed := c.changed
		wait := timeout
		for id, due := range c.reviewAt {
			if c.state.Attempts[id].State == Running && c.reviewInterval > 0 {
				if d := time.Until(due); d > 0 && d < wait {
					wait = d
				}
			}
		}
		c.mu.Unlock()
		if onWait != nil {
			onWait()
			onWait = nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-c.ctx.Done():
			timer.Stop()
			return c.ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}
