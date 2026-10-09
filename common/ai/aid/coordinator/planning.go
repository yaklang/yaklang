package coordinator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Receipt contains navigation only. Current bodies live in the two PLAN partitions.
type PlanEditReceipt struct {
	Status        string   `json:"status"`
	Components    []string `json:"components,omitempty"`
	TaskIDs       []string `json:"task_ids,omitempty"`
	PatchArtifact string   `json:"patch_artifact,omitempty"`
}

func validateDocument(p *Plan) error {
	if p == nil || strings.TrimSpace(p.Document) == "" {
		return fmt.Errorf("plan document must be nonempty")
	}
	derived, err := ParsePlan(string(p.Tree), p.Document, nil)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(derived.Tasks, p.Tasks) {
		return fmt.Errorf("plan DAG does not match its definition tree")
	}
	return validate(p)
}

func (c *Controller) checkPlanLocked(create bool) error {
	if err := c.checkLocked(); err != nil {
		return err
	}
	if c.state.Phase != PhasePlan && (create || c.state.Phase != PhaseExec) {
		return fmt.Errorf("plan editing is only allowed in PLAN")
	}
	if c.reviewing || c.state.ReviewPending {
		return fmt.Errorf("plan is locked for user review")
	}
	if create && c.state.Plan != nil {
		return fmt.Errorf("plan already exists; use modify_plan")
	}
	if !create && c.state.Plan == nil {
		return fmt.Errorf("create a plan first")
	}
	return nil
}

func (c *Controller) checkExecLocked() error {
	if err := c.checkLocked(); err != nil {
		return err
	}
	if c.state.Phase != PhaseExec {
		return fmt.Errorf("this operation requires EXEC; approve the plan first")
	}
	return nil
}

func (c *Controller) CreatePlan(ctx context.Context, data, document string) (PlanEditReceipt, error) {
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	err := c.checkPlanLocked(true)
	c.mu.Unlock()
	if err != nil {
		return PlanEditReceipt{}, err
	}
	if c.host == nil {
		return PlanEditReceipt{}, fmt.Errorf("coordinator host is missing")
	}
	p, err := c.host.Prepare(ctx, data, document)
	if err != nil {
		return PlanEditReceipt{}, err
	}
	if err = validateDocument(p); err != nil {
		return PlanEditReceipt{}, err
	}
	return c.commitEdit(ctx, p, true, PlanEditReceipt{Status: "updated", Components: []string{"document", "tasks"}})
}

// One serial edit transaction computes and validates a copy before publishing.
func (c *Controller) ModifyPlan(ctx context.Context, params map[string]any) (PlanEditReceipt, error) {
	if c.Snapshot().Phase == PhaseExec {
		return c.modifyExecutingPlan(ctx, params)
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	err := c.checkPlanLocked(false)
	p := clone(c.state.Plan)
	dir := c.patchDir
	c.mu.Unlock()
	if err != nil {
		return PlanEditReceipt{}, err
	}
	candidate, receipt, err := applyPlanEdit(p, params, dir)
	if err != nil {
		return receipt, err
	}
	return c.commitEdit(ctx, candidate, false, receipt)
}

func (c *Controller) commitEdit(ctx context.Context, p *Plan, create bool, receipt PlanEditReceipt) (PlanEditReceipt, error) {
	c.mu.Lock()
	if err := c.checkPlanLocked(create); err != nil {
		c.mu.Unlock()
		return receipt, err
	}
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return receipt, err
	}
	if reflect.DeepEqual(c.state.Plan, p) {
		receipt.Status = "unchanged"
		receipt.Components = nil
		receipt.TaskIDs = nil
		c.mu.Unlock()
		return receipt, nil
	}
	candidate := clone(c.state)
	candidate.Plan = clone(p)
	candidate.Finished = false
	candidate.Revision++
	if err := c.commitPlanLocked(candidate); err != nil {
		c.mu.Unlock()
		receipt.Status = "rejected"
		return receipt, err
	}
	c.state = candidate
	c.mu.Unlock()
	c.publish()
	return receipt, nil
}

func (c *Controller) SubmitPlan(ctx context.Context) error {
	if c.Snapshot().Phase != PhasePlan {
		return fmt.Errorf("submit_plan requires PLAN")
	}
	// Do not wait behind an edit and silently approve its new result.
	if !c.editMu.TryLock() {
		return fmt.Errorf("plan edit is still in progress")
	}
	c.mu.Lock()
	err := c.checkPlanLocked(false)
	if err == nil {
		err = validateDocument(c.state.Plan)
	}
	if err == nil && c.explorationCheck != nil {
		err = c.explorationCheck()
	}
	if err != nil {
		c.mu.Unlock()
		c.editMu.Unlock()
		return err
	}
	if c.host == nil {
		c.mu.Unlock()
		c.editMu.Unlock()
		return fmt.Errorf("coordinator host is missing")
	}
	pending := clone(c.state)
	pending.ReviewPending = true
	pending.Revision++
	if err := c.commitPlanLocked(pending); err != nil {
		c.mu.Unlock()
		c.editMu.Unlock()
		return err
	}
	c.reviewing = true
	c.state = pending
	p := clone(c.state.Plan)
	c.mu.Unlock()
	c.editMu.Unlock()
	c.publish()
	approved, err := c.host.Approve(ctx, p)
	detached := errors.Is(err, ErrDetachedPlanPublished)
	if detached {
		err = nil
	} else if err == nil {
		err = validateDocument(approved)
	}
	c.mu.Lock()
	// Detached publication has already persisted the pending review and sent
	// its card. Cancellation during the subsequent Timeline archive only stops
	// planning; it must not revoke that independently recoverable approval.
	if err == nil && !detached {
		err = c.checkLocked()
	}
	c.reviewing = false
	committed := false
	if err == nil && !detached {
		candidate := c.approvedSnapshotLocked(approved)
		candidate.Revision++
		if err = c.commitPlanLocked(candidate); err == nil {
			c.state = candidate
			committed = true
		} else {
			c.state.ReviewPending = false
		}
	} else if !detached || err != nil {
		c.state.ReviewPending = false
	}
	if !committed {
		c.notifyLocked()
	}
	c.mu.Unlock()
	c.publish()
	if err == nil && !detached {
		c.mu.Lock()
		automatic := c.automatic
		c.mu.Unlock()
		if automatic {
			c.recordApproval()
		}
		c.schedule()
	}
	return err
}

// Only the approval boundary transitions to EXEC. There is no executing-plan replacement.
func (c *Controller) approvedSnapshotLocked(p *Plan) Snapshot {
	s := clone(c.state)
	s.Plan = clone(p)
	s.Phase = PhaseExec
	s.ReviewPending = false
	s.Finished = false
	s.Attempts = make(map[string]Attempt, len(p.Tasks))
	for _, t := range p.Tasks {
		s.Attempts[t.ID] = Attempt{Task: clone(t), State: Pending}
	}
	return s
}

// Optional persistence boundary runs before the candidate becomes visible.
// A failed commit leaves the current plan, partitions and success journal intact.
func (c *Controller) commitPlanLocked(s Snapshot) error {
	if host, ok := c.host.(interface{ CommitPlan(Snapshot) error }); ok {
		return host.CommitPlan(s)
	}
	return nil
}

// LoadApproved is the existing approved/detached boundary, never a model action.
func (c *Controller) LoadApproved(p *Plan) error {
	if err := validateDocument(p); err != nil {
		return err
	}
	c.editMu.Lock()
	defer c.editMu.Unlock()
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.state.Plan != nil {
		c.mu.Unlock()
		return fmt.Errorf("plan already loaded")
	}
	candidate := c.approvedSnapshotLocked(p)
	candidate.Revision++
	if err := c.commitPlanLocked(candidate); err != nil {
		c.mu.Unlock()
		return err
	}
	c.state = candidate
	c.mu.Unlock()
	c.publish()
	c.schedule()
	return nil
}
