// Package coordinator owns the native PLAN runtime, scheduler and Yakit
// adapter.
package coordinator

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

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
	Task         Task      `json:"task"`
	ID           uint64    `json:"attempt_id"`
	State        State     `json:"state"`
	Result       Result    `json:"result"`
	ReviewReason string    `json:"review_reason,omitempty"`
	Plan         *Plan     `json:"-"`
	Predecessors []Attempt `json:"-"`
	PriorResults []Attempt `json:"-"`
}

// Snapshot is also the on-disk contract. Runtime contexts, callbacks and worker
// goroutines are never serialized. Interrupted workers require an explicit retry.
type Phase string

const (
	PhasePlan Phase = "PLAN"
	PhaseExec Phase = "EXEC"
)

type Snapshot struct {
	Schema           int                  `json:"schema"`
	Revision         uint64               `json:"revision"`
	Phase            Phase                `json:"phase"`
	Plan             *Plan                `json:"plan,omitempty"`
	ReviewPending    bool                 `json:"review_pending,omitempty"`
	Attempts         map[string]Attempt   `json:"attempts"`
	NextAttempt      uint64               `json:"next_attempt"`
	UserRevision     uint64               `json:"user_revision"`
	Finished         bool                 `json:"finished"`
	History          map[string][]Attempt `json:"history,omitempty"`
	Inbox            []Message            `json:"inbox,omitempty"`
	NextMessage      uint64               `json:"next_message,omitempty"`
	DeliveredThrough uint64               `json:"delivered_through,omitempty"`
	CheckedThrough   uint64               `json:"checked_through,omitempty"`
	ApprovalRecorded bool                 `json:"approval_recorded,omitempty"`
	Report           ReportDraft          `json:"report"`
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
	mu               sync.Mutex
	owned            sync.WaitGroup
	editMu           sync.Mutex
	patchDir         string
	explorationCheck func() error
	publishMu        sync.Mutex
	published        uint64
	ctx              context.Context
	cancel           context.CancelFunc
	host             Host
	state            Snapshot
	workers          map[string]context.CancelFunc
	changed          chan struct{}
	concurrency      int
	reviewing        bool
	closed           bool
	automatic        bool
	manualReview     bool
	editing          bool
	reviewJobs       map[string]context.CancelFunc
	reviewAt         map[string]time.Time
	reviewInterval   time.Duration
	// Notification cursor is runtime-only; it never claims the model read a result.
	eventRevision uint64
	discoveries   map[string]uint64
	// A fixed deadline from the first ordinary notification; later arrivals
	// cannot postpone delivery. Runtime-only, so restoring old snapshots is safe.
	messageBatchDelay time.Duration
	messageBatchDue   time.Time
	// Owned Timeline sink; initialized by NewLoop, never propagated to workers.
	resultConfig aicommon.AICallerConfigIf
	resultErr    error
	// Guard unchanged observations against reinsertion after Evidence eviction.
	// Accessed under publishMu; stores hashes only, never duplicate result bodies.
	resultHashes   map[string][32]byte
	pendingResults map[string]taskResultRecord
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
		workers: make(map[string]context.CancelFunc), changed: make(chan struct{}), messageBatchDelay: 5 * time.Second,
		state: Snapshot{Schema: 2, Phase: PhasePlan, Attempts: make(map[string]Attempt)}}
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
}

func (c *Controller) signalLocked() {
	c.eventRevision++
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
	results := clone(c.pendingResults)
	c.mu.Unlock()
	if s.Revision <= c.published {
		return
	}
	if c.resultConfig != nil {
		err := persistTaskResults(c.resultConfig, results, c.resultHashes)
		c.mu.Lock()
		c.resultErr = err
		if err == nil {
			c.clearPublishedResultsLocked(results)
		}
		c.mu.Unlock()
	}
	if c.host != nil {
		c.host.Changed(s)
		if host, ok := c.host.(interface{ StateError() error }); ok {
			if err := host.StateError(); err != nil {
				c.mu.Lock()
				c.resultErr = err
				c.mu.Unlock()
			}
		}
	}
	// Publish facts and the compatible UI snapshot before waking any waiter.
	c.mu.Lock()
	c.published = s.Revision
	c.signalLocked()
	c.mu.Unlock()
}

func (c *Controller) Snapshot() Snapshot { c.mu.Lock(); defer c.mu.Unlock(); return clone(c.state) }

func (c *Controller) PromptStatus() string {
	return c.Snapshot().PromptStatus()
}

func (s Snapshot) PromptStatus() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PLAN STATUS\n阶段：%s；已有计划：%t；等待审核：%t\n", s.Phase, s.Plan != nil, s.ReviewPending)
	if s.Phase == PhasePlan {
		b.WriteString("业务任务尚未获批，不可执行。\n")
		return b.String()
	}
	counts := map[State]int{}
	for _, a := range s.Attempts {
		counts[a.State]++
	}
	fmt.Fprintf(&b, "运行/退出中：%d；待审核：%d；未开始：%d；已审核：%d；失败/拒绝：%d\n", counts[Running]+counts[Cancelling], counts[AwaitingReview], counts[Pending], counts[Accepted], counts[Failed]+counts[Rejected])
	critical := 0
	for _, m := range s.Inbox {
		if m.NeedsDecision && m.Sequence > s.CheckedThrough {
			critical++
		}
	}
	fmt.Fprintf(&b, "待检查关键消息：%d；报告草稿：%t；报告已提交：%t\n", critical, s.Report.Path != "", s.Report.Submitted)
	for group, heading := range []string{"当前执行 / 待验收", "PLAN 未开始任务", "其他任务状态"} {
		written := false
		for _, t := range s.Plan.Tasks {
			a := s.Attempts[t.ID]
			active := a.State == Running || a.State == Cancelling || a.State == AwaitingReview
			if (group == 0 && !active) || (group == 1 && a.State != Pending) || (group == 2 && (active || a.State == Pending)) {
				continue
			}
			if !written {
				fmt.Fprintf(&b, "## %s\n", heading)
				written = true
			}
			fmt.Fprintf(&b, "- %s %q [%s]: %s; attempt=%d\n", t.Index, t.Name, t.ID, a.State, a.ID)
			if a.State == Pending {
				var blocked []string
				for _, dep := range t.DependsOn {
					if s.Attempts[dep].State != Accepted {
						blocked = append(blocked, dep)
					}
				}
				if len(blocked) == 0 {
					b.WriteString("  Dispatch: ready\n")
				} else {
					fmt.Fprintf(&b, "  Dispatch: blocked; waiting for accepted prerequisites [%s]\n", strings.Join(blocked, ", "))
				}
			}
		}
	}
	return b.String()
}

func (c *Controller) checkLocked() error {
	if c.closed {
		return fmt.Errorf("coordinator is closed")
	}
	if c.resultErr != nil {
		return fmt.Errorf("task Timeline result publication failed: %w", c.resultErr)
	}
	return c.ctx.Err()
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
	if c.state.Phase != PhaseExec || c.state.Plan == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("submit and approve a plan first")
	}
	if len(ids) == 0 {
		for _, t := range c.state.Plan.Tasks {
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
		a.State = Running
		a.Result = Result{}
		a.ReviewReason = ""
		ctx, cancel := context.WithCancel(c.ctx)
		c.workers[id] = cancel
		c.state.Attempts[id] = a
		if c.reviewAt == nil {
			c.reviewAt = make(map[string]time.Time)
		}
		c.reviewAt[id] = time.Now().Add(c.reviewInterval)
		frozen := clone(a)
		frozen.Plan = clone(c.state.Plan)
		for _, dep := range a.Task.DependsOn {
			frozen.Predecessors = append(frozen.Predecessors, clone(c.state.Attempts[dep]))
		}
		// A split group retains its preliminary attempts as background, never as
		// accepted prerequisites. Index ancestry comes from the validated tree.
		for _, records := range c.state.History {
			for _, previous := range records {
				if strings.HasPrefix(a.Task.Index, previous.Task.Index+"-") && previous.Result.Summary != "" {
					frozen.PriorResults = append(frozen.PriorResults, clone(previous))
				}
			}
		}
		sort.Slice(frozen.PriorResults, func(i, j int) bool { return frozen.PriorResults[i].ID < frozen.PriorResults[j].ID })
		started = append(started, frozen)
		contexts = append(contexts, ctx)
		c.owned.Add(1)
	}
	c.state.Finished = false
	c.notifyLocked()
	return started, contexts
}

func (c *Controller) launch(started []Attempt, contexts []context.Context) {
	c.publish()
	c.mu.Lock()
	publicationErr := c.resultErr
	c.mu.Unlock()
	for i, a := range started {
		if publicationErr != nil {
			// Admission could not be durably published. Do not execute tools for
			// a task whose dispatch cannot be recovered.
			c.mu.Lock()
			if cancel := c.workers[a.Task.ID]; cancel != nil {
				cancel()
				delete(c.workers, a.Task.ID)
			}
			current := c.state.Attempts[a.Task.ID]
			current.State = Failed
			current.Result.Error = publicationErr.Error()
			c.state.Attempts[a.Task.ID] = current
			c.archiveLocked(current)
			c.notifyLocked()
			c.signalLocked()
			c.mu.Unlock()
			c.owned.Done()
			continue
		}
		go c.execute(contexts[i], a)
	}
}

func (c *Controller) execute(ctx context.Context, a Attempt) {
	defer c.owned.Done()
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
	if c.closed || !ok || current.ID != a.ID {
		if ok && current.ID == a.ID {
			delete(c.workers, a.Task.ID)
			c.signalLocked()
		}
		c.mu.Unlock()
		return
	}
	delete(c.workers, a.Task.ID)
	current.Result = result
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
	c.archiveLocked(current)
	c.queueResultLocked(current)
	refs := append(append([]string{}, current.Result.EvidenceIDs...), current.Result.Artifacts...)
	if c.resultConfig != nil {
		id, _ := taskResultEvidence(c.resultConfig, taskResultRecords([]Attempt{current})[0])
		refs = append(refs, id)
	}
	c.enqueueLocked("task_settled", a.Task.ID, a.ID, string(current.State)+": "+current.Result.Summary+" "+current.Result.Error, refs, !c.manualReview || current.State == Failed)
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	c.schedule()
	if current.State == AwaitingReview {
		c.beginTaskReview(current)
	}
}

func (c *Controller) InspectTasks(ids []string) ([]Attempt, error) {
	c.mu.Lock()
	out, err := c.inspectLocked(ids)
	c.mu.Unlock()
	if err == nil {
		c.publish()
	}
	return out, err
}
func (c *Controller) inspectLocked(ids []string) ([]Attempt, error) {
	if err := c.checkExecLocked(); err != nil {
		return nil, err
	}
	if len(ids) == 0 && c.state.Plan != nil {
		for _, t := range c.state.Plan.Tasks {
			ids = append(ids, t.ID)
		}
	}
	for _, id := range ids {
		if _, ok := c.state.Attempts[id]; !ok {
			return nil, fmt.Errorf("unknown task %q", id)
		}
	}
	out := make([]Attempt, 0, len(ids))
	for _, id := range ids {
		a := c.state.Attempts[id]
		out = append(out, clone(a))
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
	initial, err := c.inspectLocked(ids)
	userRevision := c.state.UserRevision
	discoveryRevision := c.eventRevision
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
		tasks, err := c.inspectLocked(ids)
		interrupted := c.state.UserRevision != userRevision
		discovered := false
		for id, attempt := range bound {
			if c.discoveries[id] > discoveryRevision {
				discovered = true
			}
			if c.state.Attempts[id].ID != attempt {
				interrupted = true
			}
		}
		if err == nil && (interrupted || discovered) {
			tasks, _ = c.inspectLocked(ids)
			c.mu.Unlock()
			c.publish()
			reason := "changed"
			if discovered && !interrupted {
				reason = "discovery"
			}
			return WaitResult{reason, tasks}, nil
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
		hasResult := false
		for _, a := range tasks {
			if a.State == Running || a.State == Cancelling || a.State == Pending {
				settled = false
			}
			if a.ID > 0 && a.State != Running && a.State != Cancelling {
				hasResult = true
			}
		}
		if (hasResult && mode == "any") || settled {
			reason := "all_settled"
			if hasResult && mode == "any" {
				reason = "new_result"
			}
			tasks, _ = c.inspectLocked(ids)
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
				// Re-evaluate scoped discoveries, controls and settlement above.
				continue
			}
			// Any plan/control/user change yields ownership back to the planner.
			tasks, err = c.InspectTasks(ids)
			return WaitResult{"changed", tasks}, err
		}
	}
}

func (c *Controller) ReviewTask(id string, attemptID uint64, decision, reason string) error {
	return c.applyReview(id, attemptID, decision, reason, false)
}

func (c *Controller) RetryTask(id string, attemptID uint64, reason string) ([]Attempt, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("retry requires a reason")
	}
	c.mu.Lock()
	if err := c.checkExecLocked(); err != nil {
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
	if !c.automatic && len(c.workers) >= c.concurrency {
		c.mu.Unlock()
		return nil, fmt.Errorf("no free worker slot for retry")
	}
	for _, dep := range a.Task.DependsOn {
		if !c.automatic && c.state.Attempts[dep].State != Accepted {
			c.mu.Unlock()
			return nil, fmt.Errorf("retry requires accepted dependencies")
		}
	}
	for tid := range affected {
		t := c.state.Attempts[tid]
		// Invalidating an input must not revoke an explicit scope cancellation.
		// Only naming that cancelled task as the retry target can reopen it.
		if tid != id && t.State == Cancelled {
			continue
		}
		c.archiveLocked(t)
		t.State = Pending
		t.ID = 0
		t.Result = Result{}
		t.ReviewReason = reason
		c.state.Attempts[tid] = t
	}
	if c.automatic {
		c.state.Report.Submitted = false
		c.notifyLocked()
		c.mu.Unlock()
		c.publish()
		c.schedule()
		return []Attempt{c.Snapshot().Attempts[id]}, nil
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
	if err := c.checkExecLocked(); err != nil {
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
	// Explicit cancellation also resolves dependent goals, never silently accepting them.
	if c.automatic {
		selected := map[string]bool{}
		for _, id := range ids {
			selected[id] = true
		}
		for changed := true; changed; {
			changed = false
			for id, a := range c.state.Attempts {
				for _, dep := range a.Task.DependsOn {
					if selected[dep] && !selected[id] {
						selected[id] = true
						ids = append(ids, id)
						changed = true
					}
				}
			}
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
		c.archiveLocked(a)
		c.state.Attempts[id] = a
		c.queueResultLocked(a)
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
	c.enqueueLocked("user_message", "", 0, "用户输入或审核反馈已保存到 Timeline，请核对最新要求。", nil, true)
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
	if c.state.Plan == nil || (!c.state.ReviewPending && c.state.Phase != PhaseExec) || (c.reviewing && !c.state.ReviewPending) || len(c.workers) > 0 {
		return fmt.Errorf("submit the current plan before completing planning")
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
	if c.reviewing || c.state.Phase != PhaseExec || c.state.Plan == nil || len(c.workers) > 0 {
		return fmt.Errorf("plan approval or active workers remain")
	}
	for id, a := range c.state.Attempts {
		if a.State != Accepted && !(a.State == Cancelled && strings.TrimSpace(a.ReviewReason) != "") {
			return fmt.Errorf("task %q has not been reviewed or explicitly resolved (%s)", id, a.State)
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
	c.signalLocked()
	c.mu.Unlock()
	c.cancel()
}

func (c *Controller) Restore(s Snapshot) error {
	s = clone(s)
	if s.Schema != 2 || (s.Phase != PhasePlan && s.Phase != PhaseExec) || s.Plan == nil {
		return fmt.Errorf("invalid coordinator snapshot")
	}
	if s.CheckedThrough > s.DeliveredThrough || s.DeliveredThrough > s.NextMessage {
		return fmt.Errorf("invalid inbox delivery cursors")
	}
	seenMessages := make(map[uint64]bool)
	for _, message := range s.Inbox {
		if message.ID == "" || message.Sequence <= s.CheckedThrough || message.Sequence > s.NextMessage || seenMessages[message.Sequence] {
			return fmt.Errorf("invalid pending inbox message")
		}
		seenMessages[message.Sequence] = true
	}
	if err := validateDocument(s.Plan); err != nil {
		return err
	}
	if s.Phase == PhasePlan && (len(s.Attempts) > 0 || s.Finished) {
		return fmt.Errorf("PLAN snapshot contains executable state")
	}
	if s.Phase == PhaseExec && s.ReviewPending {
		return fmt.Errorf("EXEC snapshot cannot await plan approval")
	}
	var tasks []Task
	if s.Phase == PhaseExec {
		tasks = s.Plan.Tasks
	}
	for _, t := range tasks {
		a, ok := s.Attempts[t.ID]
		if !ok || !reflect.DeepEqual(t, a.Task) || a.ID > s.NextAttempt || (a.State != Pending && a.State != Cancelled && a.ID == 0) {
			return fmt.Errorf("invalid attempt snapshot for %q", t.ID)
		}
		if s.Finished && a.State != Accepted && a.State != Cancelled {
			return fmt.Errorf("finished snapshot contains unfinished work")
		}
		switch a.State {
		case Pending, Running, AwaitingReview, Accepted, Rejected, Failed, Cancelling, Cancelled:
		default:
			return fmt.Errorf("unknown attempt state %q", a.State)
		}
		if a.State == Running || a.State == Cancelling {
			a.State = Failed
			a.Result.Error = "execution interrupted; read Timeline and retry explicitly"
			s.Attempts[t.ID] = a
		}
	}
	if len(s.Attempts) != len(tasks) {
		return fmt.Errorf("snapshot contains tasks outside current EXEC plan")
	}
	c.mu.Lock()
	if err := c.checkLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	if c.state.Plan != nil || len(c.workers) > 0 {
		c.mu.Unlock()
		return fmt.Errorf("restore requires an empty controller")
	}
	c.state = s
	for _, a := range s.Attempts {
		c.archiveLocked(a)
		c.queueResultLocked(a)
		if a.State == Failed {
			c.enqueueLocked("task_settled", a.Task.ID, a.ID, a.Result.Error, a.Result.EvidenceIDs, true)
		}
	}
	c.notifyLocked()
	c.mu.Unlock()
	c.publish()
	return nil
}
