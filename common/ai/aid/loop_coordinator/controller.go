// Package loop_coordinator owns the native PLAN runtime, scheduler and Yakit
// adapter. It does not depend on the legacy aid.Coordinator implementation.
package loop_coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
)

const Name = "coordinator"

// ErrDetachedPlanPublished delegates approval to the existing detached panel.
// Submission does not grant permission to start workers.
var ErrDetachedPlanPublished = errors.New("detached plan published for user approval")

type State string

const (
	Pending        State = "pending"
	Running        State = "running"
	AwaitingReview State = "awaiting_review"
	Accepted       State = "accepted"
	Rejected       State = "rejected"
	Failed         State = "failed"
	Cancelling     State = "cancelling"
	Cancelled      State = "cancelled"
)

// Task is a frozen execution brief. DependsOn contains logical task IDs resolved
// by the host's DAG validator, rather than the UI's semantic identifiers.
type Task struct {
	ID        string   `json:"task_id"`
	Index     string   `json:"index"`
	Name      string   `json:"name"`
	Goal      string   `json:"goal"`
	DependsOn []string `json:"depends_on,omitempty"`
}

// Plan keeps the original tree for the existing UI and a validated executable
// DAG for the scheduler. Neither field contains worker status.
type Plan struct {
	Document string          `json:"document"`
	Tree     json.RawMessage `json:"root_task"`
	Tasks    []Task          `json:"tasks"`
}

type Result struct {
	Summary     string   `json:"summary"`
	Artifacts   []string `json:"artifacts,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type Attempt struct {
	Task         Task   `json:"task"`
	ID           uint64 `json:"attempt_id"`
	PlanVersion  uint64 `json:"plan_version"`
	State        State  `json:"state"`
	Result       Result `json:"result"`
	Seen         bool   `json:"seen"`
	ReviewReason string `json:"review_reason,omitempty"`
}

// Snapshot is also the on-disk contract. Runtime contexts, callbacks and worker
// goroutines are never serialized. Interrupted workers require an explicit retry.
type Snapshot struct {
	Schema           int                `json:"schema"`
	Revision         uint64             `json:"revision"`
	DraftVersion     uint64             `json:"draft_version"`
	ApprovedVersion  uint64             `json:"approved_version"`
	SubmittedVersion uint64             `json:"submitted_version,omitempty"`
	Draft            *Plan              `json:"draft,omitempty"`
	Approved         *Plan              `json:"approved,omitempty"`
	Attempts         map[string]Attempt `json:"attempts"`
	NextAttempt      uint64             `json:"next_attempt"`
	UserRevision     uint64             `json:"user_revision"`
	Finished         bool               `json:"finished"`
}

// Host implements the existing PLAN construction, approval, pe_task execution,
// persistence and event contracts. Changed receives immutable snapshots in
// revision order, outside the state lock. Execute must honor cancellation.
type Host interface {
	Prepare(context.Context, string, string) (*Plan, error)
	Approve(context.Context, *Plan) (*Plan, error)
	Execute(context.Context, Attempt) (Result, error)
	Changed(Snapshot)
}

type Controller struct {
	mu          sync.Mutex
	publishMu   sync.Mutex
	published   uint64
	ctx         context.Context
	cancel      context.CancelFunc
	host        Host
	state       Snapshot
	workers     map[string]context.CancelFunc
	changed     chan struct{}
	concurrency int
	reviewing   bool
	closed      bool
}

func New(ctx context.Context, host Host, concurrency int) *Controller {
	if ctx == nil {
		ctx = context.Background()
	}
	if concurrency < 1 {
		concurrency = 1
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Controller{ctx: ctx, cancel: cancel, host: host, concurrency: concurrency,
		workers: make(map[string]context.CancelFunc), changed: make(chan struct{}),
		state: Snapshot{Schema: 1, Attempts: make(map[string]Attempt)}}
}

func clone[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }

func validate(p *Plan) error {
	if p == nil || len(p.Tasks) == 0 {
		return fmt.Errorf("plan has no executable tasks")
	}
	tasks := make(map[string]Task)
	for _, t := range p.Tasks {
		if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Goal) == "" {
			return fmt.Errorf("task requires task_id, name and goal")
		}
		if _, exists := tasks[t.ID]; exists {
			return fmt.Errorf("duplicate task_id %q", t.ID)
		}
		tasks[t.ID] = t
	}
	visiting, done := make(map[string]bool), make(map[string]bool)
	var visit func(string) error
	visit = func(id string) error {
		if done[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("dependency cycle at %q", id)
		}
		t, ok := tasks[id]
		if !ok {
			return fmt.Errorf("unknown dependency %q", id)
		}
		visiting[id] = true
		for _, dep := range t.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		delete(visiting, id)
		done[id] = true
		return nil
	}
	for id := range tasks {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) notifyLocked() {
	c.state.Revision++
	close(c.changed)
	c.changed = make(chan struct{})
}

func (c *Controller) publish() {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	s := clone(c.state)
	c.mu.Unlock()
	if s.Revision <= c.published {
		return
	}
	if c.host != nil {
		c.host.Changed(s)
	}
	c.published = s.Revision
}

func (c *Controller) Snapshot() Snapshot { c.mu.Lock(); defer c.mu.Unlock(); return clone(c.state) }

func (c *Controller) PromptStatus() string {
	return c.Snapshot().PromptStatus()
}

func (s Snapshot) PromptStatus() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PLAN STATUS\nDraft version: %d; approved version: %d\n", s.DraftVersion, s.ApprovedVersion)
	if s.SubmittedVersion > 0 && s.Approved == nil {
		fmt.Fprintf(&b, "Detached submitted version: %d; awaiting the user's execution request.\n", s.SubmittedVersion)
	}
	if s.Approved == nil {
		b.WriteString("PLAN awaits approval; no tasks may execute.\n")
		return b.String()
	}
	for group, heading := range []string{"当前执行 / 待验收", "PLAN 未开始任务", "其他任务状态"} {
		written := false
		for _, t := range s.Approved.Tasks {
			a := s.Attempts[t.ID]
			active := a.State == Running || a.State == Cancelling || a.State == AwaitingReview
			if (group == 0 && !active) || (group == 1 && a.State != Pending) || (group == 2 && (active || a.State == Pending)) {
				continue
			}
			if !written {
				fmt.Fprintf(&b, "## %s\n", heading)
				written = true
			}
			fmt.Fprintf(&b, "- %s %q [%s]: %s; attempt=%d; observed=%v\n", t.Index, t.Name, t.ID, a.State, a.ID, a.Seen)
		}
	}
	return b.String()
}

func (c *Controller) checkLocked() error {
	if c.closed {
		return fmt.Errorf("coordinator is closed")
	}
	return c.ctx.Err()
}

// CreatePlan and ModifyPlan replace only the draft. Approval is a separate
// version-checked operation; existing approved work can continue meanwhile.
func (c *Controller) CreatePlan(ctx context.Context, data, document string) (uint64, error) {
	return c.edit(ctx, data, document, 0, true)
}
func (c *Controller) ModifyPlan(ctx context.Context, version uint64, data, document string) (uint64, error) {
	return c.edit(ctx, data, document, version, false)
}
func (c *Controller) edit(ctx context.Context, data, document string, version uint64, create bool) (uint64, error) {
	if c.host == nil {
		return 0, fmt.Errorf("coordinator host is missing")
	}
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return 0, err
	}
	if c.reviewing || (create && c.state.Draft != nil) || (!create && (version != c.state.DraftVersion || c.state.Draft == nil)) {
		c.mu.Unlock()
		return 0, fmt.Errorf("draft changed or is under review; inspect current plan")
	}
	base := c.state.DraftVersion
	c.mu.Unlock()
	p, err := c.host.Prepare(ctx, data, document)
	if err != nil {
		return 0, err
	}
	if err = validate(p); err != nil {
		return 0, err
	}
	c.mu.Lock()
	if err = c.checkLocked(); err != nil {
		c.mu.Unlock()
		return 0, err
	}
	if c.reviewing || c.state.DraftVersion != base {
		c.mu.Unlock()
		return 0, fmt.Errorf("draft changed while preparing plan")
	}
	c.state.Draft = clone(p)
	c.state.DraftVersion++
	c.state.Finished = false
	v := c.state.DraftVersion
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return v, nil
}

func (c *Controller) SubmitPlan(ctx context.Context, version uint64) error {
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.reviewing || c.state.Draft == nil || c.state.DraftVersion != version {
		c.mu.Unlock()
		return fmt.Errorf("draft changed or is already under review")
	}
	if c.state.Approved == nil && c.state.SubmittedVersion == version {
		c.mu.Unlock()
		return nil
	}
	c.reviewing = true
	draft := clone(c.state.Draft)
	c.mu.Unlock()
	p, err := c.host.Approve(ctx, draft)
	detached := errors.Is(err, ErrDetachedPlanPublished)
	if detached {
		err = nil
	}
	if err == nil && !detached {
		err = validate(p)
	}
	c.mu.Lock()
	c.reviewing = false
	if err == nil {
		err = c.checkLocked()
	}
	if err == nil && c.state.DraftVersion != version {
		err = fmt.Errorf("approval is stale")
	}
	if err == nil && detached {
		c.state.SubmittedVersion = version
	} else if err == nil {
		err = c.adoptLocked(p, version)
	}
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return err
}

func (c *Controller) adoptLocked(p *Plan, version uint64) error {
	newTasks := make(map[string]Task)
	for _, t := range p.Tasks {
		newTasks[t.ID] = t
	}
	affected := make(map[string]bool)
	for id, a := range c.state.Attempts {
		if t, ok := newTasks[id]; !ok || !reflect.DeepEqual(t, a.Task) {
			affected[id] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, t := range p.Tasks {
			for _, dep := range t.DependsOn {
				if affected[dep] && !affected[t.ID] {
					affected[t.ID], changed = true, true
				}
			}
		}
	}
	for id := range c.workers {
		a := c.state.Attempts[id]
		t, ok := newTasks[id]
		if !ok || affected[id] || !reflect.DeepEqual(t, a.Task) {
			return fmt.Errorf("cancel and wait for active task %q before replacing its brief", id)
		}
	}
	retained := make(map[string]Attempt)
	for _, t := range p.Tasks {
		if a, ok := c.state.Attempts[t.ID]; ok && !affected[t.ID] && reflect.DeepEqual(t, a.Task) {
			retained[t.ID] = a
		} else {
			retained[t.ID] = Attempt{Task: clone(t), State: Pending, PlanVersion: version}
		}
	}
	// A changed input invalidates all downstream accepted results as well.
	for changed := true; changed; {
		changed = false
		for _, t := range p.Tasks {
			a := retained[t.ID]
			if a.State != Accepted {
				continue
			}
			for _, dep := range t.DependsOn {
				if retained[dep].State != Accepted {
					a.State = Pending
					a.ID = 0
					a.Result = Result{}
					a.Seen = false
					retained[t.ID] = a
					changed = true
					break
				}
			}
		}
	}
	c.state.Draft = clone(p)
	c.state.Approved = clone(p)
	c.state.ApprovedVersion = version
	c.state.Attempts = retained
	c.state.Finished = false
	return nil
}

// LoadApproved is for existing approved/detached plans. It bypasses no approval:
// callers must already own an approved plan through the legacy review endpoint.
func (c *Controller) LoadApproved(p *Plan) error {
	if err := validate(p); err != nil {
		return err
	}
	c.mu.Lock()
	if c.state.Approved != nil || c.state.Draft != nil {
		c.mu.Unlock()
		return fmt.Errorf("plan already loaded")
	}
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	c.state.DraftVersion = 1
	err := c.adoptLocked(p, 1)
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return err
}

func (c *Controller) readyLocked(a Attempt) bool {
	if a.State != Pending {
		return false
	}
	for _, dep := range a.Task.DependsOn {
		if c.state.Attempts[dep].State != Accepted {
			return false
		}
	}
	return true
}

// StartTasks is atomic admission of a selected set (or all currently ready
// tasks). It never runs the whole DAG or hides further scheduling in a worker.
func (c *Controller) StartTasks(ids []string) ([]Attempt, error) {
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if c.state.Approved == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("submit and approve a plan first")
	}
	if len(ids) == 0 {
		for _, t := range c.state.Approved.Tasks {
			if c.readyLocked(c.state.Attempts[t.ID]) {
				ids = append(ids, t.ID)
			}
		}
		available := c.concurrency - len(c.workers)
		if available < len(ids) {
			ids = ids[:available]
		}
	}
	if len(ids) == 0 {
		c.mu.Unlock()
		return nil, fmt.Errorf("no ready tasks or no free worker slots")
	}
	if len(c.workers)+len(ids) > c.concurrency {
		c.mu.Unlock()
		return nil, fmt.Errorf("worker concurrency limit %d exceeded", c.concurrency)
	}
	seen := make(map[string]bool)
	for _, id := range ids {
		a, ok := c.state.Attempts[id]
		if !ok || seen[id] || !c.readyLocked(a) {
			c.mu.Unlock()
			return nil, fmt.Errorf("task %q is unknown, duplicated, active or has unaccepted dependencies", id)
		}
		seen[id] = true
	}
	started, contexts := c.startLocked(ids)
	c.mu.Unlock()
	c.launch(started, contexts)
	return started, nil
}

func (c *Controller) startLocked(ids []string) ([]Attempt, []context.Context) {
	started := make([]Attempt, 0, len(ids))
	contexts := make([]context.Context, 0, len(ids))
	for _, id := range ids {
		a := c.state.Attempts[id]
		c.state.NextAttempt++
		a.ID = c.state.NextAttempt
		a.PlanVersion = c.state.ApprovedVersion
		a.State = Running
		a.Seen = false
		a.Result = Result{}
		a.ReviewReason = ""
		ctx, cancel := context.WithCancel(c.ctx)
		c.workers[id] = cancel
		c.state.Attempts[id] = a
		started = append(started, clone(a))
		contexts = append(contexts, ctx)
	}
	c.state.Finished = false
	c.notifyLocked()
	return started, contexts
}

func (c *Controller) launch(started []Attempt, contexts []context.Context) {
	c.publish()
	for i, a := range started {
		go c.execute(contexts[i], a)
	}
}

func (c *Controller) execute(ctx context.Context, a Attempt) {
	var result Result
	var err error
	func() {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("worker panic: %v", p)
			}
		}()
		result, err = c.host.Execute(ctx, a)
	}()
	c.mu.Lock()
	current, ok := c.state.Attempts[a.Task.ID]
	if !ok || current.ID != a.ID {
		c.mu.Unlock()
		return
	}
	delete(c.workers, a.Task.ID)
	current.Result = result
	current.Seen = false
	if current.State == Cancelling || ctx.Err() != nil {
		current.State = Cancelled
		current.Result.Error = "task cancelled"
	} else if err != nil || result.Error != "" || strings.TrimSpace(result.Summary) == "" {
		current.State = Failed
		if strings.TrimSpace(result.Summary) == "" && result.Error == "" {
			current.Result.Error = "worker returned no result summary"
		}
		if err != nil {
			current.Result.Error = err.Error()
		}
	} else {
		current.State = AwaitingReview
	}
	c.state.Attempts[a.Task.ID] = current
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
}

func (c *Controller) InspectTasks(ids []string) ([]Attempt, error) {
	c.mu.Lock()
	out, err := c.inspectLocked(ids, true)
	c.mu.Unlock()
	if err == nil {
		c.publish()
	}
	return out, err
}
func (c *Controller) inspectLocked(ids []string, mark bool) ([]Attempt, error) {
	if err := c.checkLocked(); err != nil {
		return nil, err
	}
	if len(ids) == 0 && c.state.Approved != nil {
		for _, t := range c.state.Approved.Tasks {
			ids = append(ids, t.ID)
		}
	}
	for _, id := range ids {
		if _, ok := c.state.Attempts[id]; !ok {
			return nil, fmt.Errorf("unknown task %q", id)
		}
	}
	out := make([]Attempt, 0, len(ids))
	updated := false
	for _, id := range ids {
		a := c.state.Attempts[id]
		if mark && a.ID > 0 && a.State != Running && a.State != Cancelling && !a.Seen {
			a.Seen = true
			c.state.Attempts[id] = a
			updated = true
		}
		out = append(out, clone(a))
	}
	if updated {
		c.notifyLocked()
	}
	return out, nil
}

type WaitResult struct {
	Reason string    `json:"reason"`
	Tasks  []Attempt `json:"tasks"`
}

// WaitTasks waits on host notifications, not AI polling. Timeout never cancels
// work. User input calls Wake so the planner can reconsider immediately.
func (c *Controller) WaitTasks(ctx context.Context, ids []string, timeout time.Duration) (WaitResult, error) {
	return c.WaitTasksMode(ctx, ids, timeout, "any")
}

func (c *Controller) WaitTasksMode(ctx context.Context, ids []string, timeout time.Duration, mode string) (WaitResult, error) {
	if mode == "" {
		mode = "any"
	}
	if mode != "any" && mode != "all" {
		return WaitResult{}, fmt.Errorf("wait mode must be any or all")
	}
	c.mu.Lock()
	initial, err := c.inspectLocked(ids, false)
	userRevision, planVersion := c.state.UserRevision, c.state.ApprovedVersion
	bound := make(map[string]uint64)
	for _, a := range initial {
		bound[a.Task.ID] = a.ID
		if mode == "all" && a.State == Pending {
			err = fmt.Errorf("all wait requires dispatched tasks; %q is still pending", a.Task.ID)
		}
	}
	c.mu.Unlock()
	if err != nil {
		return WaitResult{}, err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		tasks, err := c.inspectLocked(ids, false)
		interrupted := c.state.UserRevision != userRevision || c.state.ApprovedVersion != planVersion
		for id, attempt := range bound {
			if c.state.Attempts[id].ID != attempt {
				interrupted = true
			}
		}
		if err == nil && interrupted {
			tasks, _ = c.inspectLocked(ids, true)
			c.mu.Unlock()
			c.publish()
			return WaitResult{"changed", tasks}, nil
		}
		changed := c.changed
		if err != nil {
			c.mu.Unlock()
			return WaitResult{}, err
		}
		if len(tasks) == 0 {
			c.mu.Unlock()
			return WaitResult{}, fmt.Errorf("no approved tasks to wait for")
		}
		settled := true
		unseen := false
		for _, a := range tasks {
			if a.State == Running || a.State == Cancelling || a.State == Pending {
				settled = false
			}
			if a.ID > 0 && !a.Seen && a.State != Running && a.State != Cancelling {
				unseen = true
			}
		}
		if (unseen && mode == "any") || settled {
			reason := "all_settled"
			if unseen && mode == "any" {
				reason = "new_result"
			}
			tasks, _ = c.inspectLocked(ids, true)
			c.mu.Unlock()
			c.publish()
			return WaitResult{reason, tasks}, nil
		}
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return WaitResult{}, ctx.Err()
		case <-c.ctx.Done():
			return WaitResult{}, c.ctx.Err()
		case <-timer.C:
			tasks, err = c.InspectTasks(ids)
			return WaitResult{"timeout", tasks}, err
		case <-changed:
			if mode == "all" {
				c.mu.Lock()
				interrupted := c.state.UserRevision != userRevision || c.state.ApprovedVersion != planVersion
				for id, attempt := range bound {
					if c.state.Attempts[id].ID != attempt {
						interrupted = true
					}
				}
				c.mu.Unlock()
				if !interrupted {
					continue
				}
			}
			// Any plan/control/user change yields ownership back to the planner.
			tasks, err = c.InspectTasks(ids)
			return WaitResult{"changed", tasks}, err
		}
	}
}

func (c *Controller) ReviewTask(id string, attemptID uint64, decision, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("review requires an evidence-backed reason")
	}
	if decision != "accept" && decision != "reject" {
		return fmt.Errorf("review decision must be accept or reject")
	}
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	a, ok := c.state.Attempts[id]
	if !ok || a.ID != attemptID || a.State != AwaitingReview || !a.Seen {
		c.mu.Unlock()
		return fmt.Errorf("inspect the current awaiting_review attempt before reviewing it")
	}
	if decision == "accept" {
		a.State = Accepted
	} else {
		a.State = Rejected
	}
	a.ReviewReason = reason
	c.state.Attempts[id] = a
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return nil
}

func (c *Controller) RetryTask(id string, attemptID uint64, reason string) ([]Attempt, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("retry requires a reason")
	}
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	a, ok := c.state.Attempts[id]
	if !ok || a.ID != attemptID || (a.State != Failed && a.State != Rejected && a.State != Cancelled && a.State != Accepted) {
		c.mu.Unlock()
		return nil, fmt.Errorf("only a settled current attempt can be retried")
	}
	// Running downstream tasks must be stopped before invalidating their inputs.
	affected := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for tid, t := range c.state.Attempts {
			for _, dep := range t.Task.DependsOn {
				if affected[dep] && !affected[tid] {
					affected[tid] = true
					changed = true
				}
			}
		}
	}
	for tid := range affected {
		if _, active := c.workers[tid]; active {
			c.mu.Unlock()
			return nil, fmt.Errorf("cancel and wait for dependent task %q before retry", tid)
		}
	}
	if len(c.workers) >= c.concurrency {
		c.mu.Unlock()
		return nil, fmt.Errorf("no free worker slot for retry")
	}
	for _, dep := range a.Task.DependsOn {
		if c.state.Attempts[dep].State != Accepted {
			c.mu.Unlock()
			return nil, fmt.Errorf("retry requires accepted dependencies")
		}
	}
	for tid := range affected {
		t := c.state.Attempts[tid]
		t.State = Pending
		t.ID = 0
		t.Result = Result{}
		t.Seen = false
		t.ReviewReason = reason
		c.state.Attempts[tid] = t
	}
	started, contexts := c.startLocked([]string{id})
	c.mu.Unlock()
	c.launch(started, contexts)
	return started, nil
}

func (c *Controller) CancelTasks(ids []string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("cancel requires a reason")
	}
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if len(ids) == 0 {
		for id := range c.state.Attempts {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	for _, id := range ids {
		if _, ok := c.state.Attempts[id]; !ok {
			c.mu.Unlock()
			return fmt.Errorf("unknown task %q", id)
		}
	}
	cancels := make([]context.CancelFunc, 0)
	for _, id := range ids {
		a := c.state.Attempts[id]
		if cancel, ok := c.workers[id]; ok {
			a.State = Cancelling
			cancels = append(cancels, cancel)
		} else if a.State != Accepted {
			a.State = Cancelled
		}
		a.ReviewReason = reason
		c.state.Attempts[id] = a
	}
	c.notifyLocked()
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	c.publish()
	return nil
}

func (c *Controller) Wake() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.state.UserRevision++
	c.state.Finished = false
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
}

// Finalize records completion after the ordinary loop's TODO gate admits exit.
// Inputs arriving since prompt construction must be considered before closing.
func (c *Controller) Finalize(observedUserRevision uint64) error {
	c.mu.Lock()
	if err := c.canFinishLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.state.UserRevision != observedUserRevision {
		c.mu.Unlock()
		return fmt.Errorf("new user input arrived; inspect Timeline before finishing")
	}
	c.state.Finished = true
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return nil
}

func (c *Controller) CanFinish() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.canFinishLocked()
}

func (c *Controller) CanFinishPlanning() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.canFinishPlanningLocked()
}

func (c *Controller) canFinishPlanningLocked() error {
	if err := c.checkLocked(); err != nil {
		return err
	}
	approved := c.state.Approved != nil && c.state.ApprovedVersion == c.state.DraftVersion
	detached := c.state.Approved == nil && c.state.SubmittedVersion > 0 && c.state.SubmittedVersion == c.state.DraftVersion
	if c.reviewing || (!approved && !detached) || len(c.workers) > 0 {
		return fmt.Errorf("approve the current draft before completing planning")
	}
	return nil
}

func (c *Controller) FinalizePlanning(observedUserRevision uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.canFinishPlanningLocked(); err != nil {
		return err
	}
	if c.state.UserRevision != observedUserRevision {
		return fmt.Errorf("new user input arrived; inspect Timeline before finishing")
	}
	return nil
}

func (c *Controller) canFinishLocked() error {
	if err := c.checkLocked(); err != nil {
		return err
	}
	if c.reviewing || c.state.Approved == nil || c.state.DraftVersion != c.state.ApprovedVersion || len(c.workers) > 0 {
		return fmt.Errorf("plan approval or active workers remain")
	}
	for id, a := range c.state.Attempts {
		if a.State != Accepted || !a.Seen {
			return fmt.Errorf("task %q has not been inspected and accepted (%s)", id, a.State)
		}
	}
	return nil
}

func (c *Controller) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.notifyLocked()
	c.mu.Unlock()
	c.cancel()
}

func (c *Controller) Restore(s Snapshot) error {
	s = clone(s)
	if s.Schema != 1 || s.DraftVersion == 0 || s.DraftVersion < s.ApprovedVersion || s.SubmittedVersion > s.DraftVersion || (s.Approved == nil && s.Draft == nil) {
		return fmt.Errorf("invalid coordinator snapshot")
	}
	if s.Approved != nil {
		if s.ApprovedVersion == 0 {
			return fmt.Errorf("approved snapshot is missing its version")
		}
		if err := validate(s.Approved); err != nil {
			return err
		}
	}
	if s.Approved == nil && (s.ApprovedVersion != 0 || len(s.Attempts) > 0 || s.Finished) {
		return fmt.Errorf("unapproved snapshot contains executable state")
	}
	var approvedTasks []Task
	if s.Approved != nil {
		approvedTasks = s.Approved.Tasks
	}
	if s.Draft != nil {
		if err := validate(s.Draft); err != nil {
			return err
		}
	}
	for _, t := range approvedTasks {
		a, ok := s.Attempts[t.ID]
		if !ok || !reflect.DeepEqual(t, a.Task) || a.ID > s.NextAttempt {
			return fmt.Errorf("invalid attempt snapshot for %q", t.ID)
		}
		if a.PlanVersion == 0 || a.PlanVersion > s.ApprovedVersion || (a.State != Pending && a.State != Cancelled && a.ID == 0) {
			return fmt.Errorf("invalid attempt identity/version for %q", t.ID)
		}
		if s.Finished && (a.State != Accepted || !a.Seen || s.DraftVersion != s.ApprovedVersion) {
			return fmt.Errorf("finished snapshot contains unfinished work")
		}
		switch a.State {
		case Pending, Running, AwaitingReview, Accepted, Rejected, Failed, Cancelling, Cancelled:
		default:
			return fmt.Errorf("unknown attempt state %q", a.State)
		}
		if a.State == Running || a.State == Cancelling {
			a.State = Failed
			a.Result.Error = "execution interrupted; inspect and retry explicitly"
			a.Seen = false
			s.Attempts[t.ID] = a
		}
	}
	if len(s.Attempts) != len(approvedTasks) {
		return fmt.Errorf("snapshot contains tasks outside approved plan")
	}
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.state.Approved != nil || len(c.workers) > 0 {
		c.mu.Unlock()
		return fmt.Errorf("restore requires an empty controller")
	}
	c.state = clone(s)
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return nil
}
