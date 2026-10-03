package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Session owns the new coordinator's lifecycle. No legacy Coordinator, AiTask,
// plan builder, execution runtime or persistence implementation is involved.
type Session struct {
	*aicommon.Config
	invoker        aicommon.AITaskInvokeRuntime
	controller     *Controller
	query          string
	cancel         context.CancelFunc
	cleanup        func()
	parent         *aicommon.Config
	parentTaskID   string
	planningOnly   bool
	detached       bool
	startTaskID    string
	mu             sync.Mutex
	last           Snapshot
	lastTree       json.RawMessage
	opened, closed map[string]bool
	tasks          map[string]*aicommon.AIStatefulTaskBase
	stateErr       error
	closeOnce      sync.Once
}

func NewSession(ctx context.Context, query string, opts ...aicommon.ConfigOption) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	opts = append(append([]aicommon.ConfigOption{}, opts...), NativeOptions()...)
	opts = append(opts, aicommon.WithContext(ctx))
	runtime, err := aicommon.AIRuntimeInvokerGetter(ctx, opts...)
	if err != nil {
		cancel()
		return nil, err
	}
	cfg, ok := runtime.GetConfig().(*aicommon.Config)
	if !ok {
		cancel()
		return nil, fmt.Errorf("coordinator requires Config-backed runtime")
	}
	s := &Session{Config: cfg, invoker: runtime, query: query, cancel: cancel, detached: cfg.EnableDetachedPlan, opened: map[string]bool{}, closed: map[string]bool{}, tasks: map[string]*aicommon.AIStatefulTaskBase{}}
	cfg.Guardian.SetOutputEmitter(cfg.Id, cfg.EventHandler)
	cfg.Guardian.SetAICaller(cfg)
	if cfg.EnableAISearch {
		if err := s.configureToolSearch(); err != nil {
			cancel()
			return nil, err
		}
	}
	s.controller = New(ctx, s, cfg.GetPlanExecTaskConcurrency())
	return s, nil
}

// FromRuntime is the outer-session adapter. Input is journaled once by the
// parent; only completed notifications wake the coordinator. Each child owns
// its event processor and lifecycle, so no handlers overwrite the parent.
func FromRuntime(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, id string) (*Session, error) {
	parent, ok := r.GetConfig().(*aicommon.Config)
	if !ok || task == nil {
		return nil, fmt.Errorf("coordinator requires a session task")
	}
	if !parent.EnablePlanAndExec {
		return nil, fmt.Errorf("PLAN is disabled for this session")
	}
	if id == "" {
		id = uuid.NewString()
	}
	if ctx == nil {
		ctx = task.GetContext()
	}
	ctx, cancel := context.WithCancel(ctx)
	input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
	hotpatch := parent.HotPatchBroadcaster.Subscribe()
	opts := aicommon.ConvertConfigToOptions(parent)
	opts = append(opts, nativePlanOptions(parent)...)
	opts = append(opts, aicommon.WithForceManualPlanReview(parent.ForceManualPlanReview))
	opts = append(opts, aicommon.WithPlanPrompt(parent.PlanPrompt))
	opts = append(opts, aicommon.WithID(id), aicommon.WithContext(ctx), aicommon.WithEventInputChanx(input), aicommon.WithHotPatchOptionChan(hotpatch), aicommon.WithAICallbacks(parent.GetRawAICallbacks()), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
		e.CoordinatorId = id
		if e.Type == schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE {
			e.CoordinatorId = parent.Id
		}
		parent.EventHandler(e)
	}))
	s, err := NewSession(ctx, task.GetUserInput(), opts...)
	if err != nil {
		cancel()
		parent.HotPatchBroadcaster.Unsubscribe(hotpatch)
		return nil, err
	}
	s.parent, s.parentTaskID = parent, task.GetId()
	key := "coordinator-input-" + id
	parent.InputEventManager.RegisterMirrorOfAIInputEvent(key, func(e *ypb.AIInputEvent) {
		switch e.SyncType {
		case aicommon.SYNC_TYPE_USER_INTERVENTION, "queue_info", "react_cancel_task", "react_cancel_current_task":
			// The outer runtime owns queue cancellation. Its context already
			// cancels this child; replaying the root task ID here is invalid.
			return
		}
		input.SafeFeed(e)
	})
	parent.InputEventManager.RegisterAfterInputEvent(key, func(e *ypb.AIInputEvent) {
		if e.IsInteractiveMessage || e.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION {
			s.controller.Wake()
		}
	})
	s.cleanup = func() {
		parent.InputEventManager.UnregisterMirrorOfAIInputEvent(key)
		parent.InputEventManager.UnregisterAfterInputEvent(key)
		parent.HotPatchBroadcaster.Unsubscribe(hotpatch)
		cancel()
	}
	return s, nil
}

func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.controller.Close()
		s.cancel()
		if s.cleanup != nil {
			s.cleanup()
		}
	})
}
func (s *Session) PlanningOnly() bool               { return s.planningOnly || s.detached }
func (s *Session) ReportRequired() bool             { return s.GenerateReport }
func (s *Session) Run() error                       { return s.run(false) }
func (s *Session) RunPlanOnly() error               { return s.run(true) }
func (s *Session) RunExecuteApprovedPlan() error    { return s.run(false) }
func (s *Session) RunExecuteOnly() error            { return s.run(false) }
func (s *Session) SetRecoveryStartTaskID(id string) { s.startTaskID = id }
func (s *Session) Snapshot() Snapshot               { return s.controller.Snapshot() }

// SubmitInput adapts an existing RPC plan payload to this runtime's own
// approval and persistence. It never invokes a legacy planner or model loop.
func (s *Session) SubmitInput(ctx context.Context, input *aicommon.ExecutePlanInput, detached, forceManual bool) (*aicommon.ExecutePlanInput, error) {
	if input == nil {
		return nil, fmt.Errorf("execute plan input is nil")
	}
	if ctx == nil {
		ctx = s.GetContext()
	}
	s.detached, s.planningOnly = detached, true
	_ = aicommon.WithForceManualPlanReview(forceManual)(s.Config)
	version, err := s.controller.CreatePlan(ctx, input.PlanData, input.PlanDocument)
	if err != nil {
		return nil, err
	}
	if err := s.controller.SubmitPlan(ctx, version); err != nil {
		return nil, err
	}
	s.mu.Lock()
	err = s.stateErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	snapshot := s.controller.Snapshot()
	p := snapshot.Approved
	if detached {
		p = snapshot.Draft
	}
	if p == nil {
		return nil, fmt.Errorf("plan was not submitted")
	}
	return &aicommon.ExecutePlanInput{PlanPayload: input.PlanPayload, PlanData: string(p.Tree), PlanDocument: p.Document}, nil
}

func (s *Session) Prepare(ctx context.Context, data, document string) (*Plan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snapshot := s.controller.Snapshot()
	previous := snapshot.Draft
	if previous == nil {
		previous = snapshot.Approved
	}
	return ParsePlan(data, document, previous)
}

func (s *Session) BuildRootTaskFromPlanData(data, query string) (*PlanNode, error) {
	p, err := ParsePlan(data, "", nil)
	if err != nil {
		return nil, err
	}
	var root PlanNode
	err = json.Unmarshal(p.Tree, &root)
	return &root, err
}

func (s *Session) CommitApprovedPlan(root *PlanNode, document string) error {
	data, err := json.Marshal(root)
	if err != nil {
		return err
	}
	p, err := ParsePlan(string(data), document, nil)
	if err != nil {
		return err
	}
	snapshot := Snapshot{Schema: 1, DraftVersion: 1, ApprovedVersion: 1, Draft: p, Approved: p, Attempts: map[string]Attempt{}}
	for _, task := range p.Tasks {
		snapshot.Attempts[task.ID] = Attempt{Task: task, PlanVersion: 1, State: Pending}
	}
	s.detached = false
	return s.controller.Restore(snapshot)
}

func (s *Session) restore() error {
	if s.controller.Snapshot().Draft != nil {
		return nil
	}
	if s.GetDB() == nil {
		return nil
	}
	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(s.GetDB(), s.GetRuntimeId())
	if err != nil || record == nil {
		return nil
	}
	var progress struct {
		Engine string    `json:"plan_engine"`
		State  *Snapshot `json:"coordinator_state"`
	}
	if err := json.Unmarshal([]byte(record.TaskProgress), &progress); err != nil {
		return err
	}
	if progress.Engine != "" && progress.Engine != Name {
		return fmt.Errorf("stored plan belongs to coordinator_legacy; select its route")
	}
	if progress.State == nil {
		return fmt.Errorf("coordinator snapshot is missing; refusing implicit legacy recovery")
	}
	if err := resetCoordinatorRecovery(progress.State, s.startTaskID); err != nil {
		return err
	}
	if progress.State.Approved != nil {
		s.detached = false
	}
	return s.controller.Restore(*progress.State)
}

func (s *Session) run(planningOnly bool) (err error) {
	defer s.Close()
	s.planningOnly = planningOnly
	if err = s.restore(); err != nil {
		return err
	}
	if s.controller.Snapshot().Approved != nil {
		s.detached = false
	}
	s.registerControls()
	s.EmitCurrentConfigInfo()
	key := "coordinator-wake-" + s.GetRuntimeId()
	s.InputEventManager.RegisterAfterInputEvent(key, func(e *ypb.AIInputEvent) {
		if e.IsInteractiveMessage || e.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION {
			s.controller.Wake()
		}
	})
	defer s.InputEventManager.UnregisterAfterInputEvent(key)
	if s.parent != nil && !s.detached {
		payload := map[string]any{"coordinator_id": s.Id, "re-act_id": s.parent.Id, "re-act_task": s.parentTaskID, "start_task_id": s.startTaskID}
		s.parent.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, "plan", payload)
		defer func() {
			if err != nil {
				s.parent.EmitPlanExecFail(err.Error())
			}
			s.parent.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, "plan", payload)
		}()
	}
	input := s.query
	if s.parent != nil {
		// The parent owns user-input ingress. A nested loop must not journal
		// its task description (including approval/recovery labels) as input.
		input = ""
	}
	task := aicommon.NewStatefulTaskBase("coordinator-"+s.Id, input, s.GetContext(), s.GetEmitter(), true)
	task.SetUserInput(s.query)
	s.invoker.SetCurrentTask(task)
	if s.parent == nil {
		// The loop reuses this ingress receipt instead of recording the same
		// standalone Session query a second time under another timeline item.
		s.Timeline.EnsureTaskUserInput(task.GetId(), task.GetOriginUserInput(), s.AcquireId)
	}
	if err := s.preparePresetPlan(); err != nil {
		return fmt.Errorf("prepare preset plan: %w", err)
	}
	opts := append(reactloops.BasicAICommonConfigOption(s.Config), WithController(s.controller))
	if s.PlanningOnly() {
		opts = append(opts, WithPlanningOnly())
	}
	loop, err := NewLoop(s.invoker, opts...)
	if err != nil {
		return err
	}
	err = executeLoop(s.Config, s.invoker, loop, task)
	if err != nil {
		return err
	}
	if s.PlanningOnly() {
		err = s.controller.CanFinishPlanning()
	} else {
		err = s.controller.CanFinish()
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.stateErr
	s.mu.Unlock()
	return err
}

func executeLoop(cfg *aicommon.Config, invoker aicommon.AITaskInvokeRuntime, loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	invoker.SetCurrentTask(task)
	differ := aicommon.NewTimelineDiffer(cfg.Timeline)
	differ.SetBaseline()
	buffer := aicommon.NewMemoryFlushBuffer(Name, differ, nil)
	defer buffer.Close()
	reactloops.WithOnPostIteraction(func(_ *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, done bool, reason any, op *reactloops.OnPostIterationOperator) {
		op.DeferAfterCallbacks(func() {
			if cfg.MemoryTriage == nil {
				return
			}
			buffer.ProcessAsync(aicommon.MemoryFlushSignal{Iteration: iteration, Task: task, IsDone: done, Reason: reason, ShouldEndIteration: op.ShouldEndIteration()}, func(payload *aicommon.MemoryFlushPayload, err error) {
				if err == nil && payload != nil {
					if err := cfg.MemoryTriage.HandleMemory(payload.ContextualInput); err != nil {
						cfg.EmitError("memory triage: %v", err)
					}
				}
			})
		})
	})(loop)
	aicommon.BeginSessionSnapshotExecutionForTask(cfg, task, time.Now())
	reactloops.EmitSessionSnapshot(cfg, loop, task)
	err := loop.ExecuteWithExistedTask(task)
	aicommon.FinalizeSessionSnapshotExecutionForTask(cfg, task, time.Now())
	reactloops.EmitSessionSnapshot(cfg, loop, task)
	return err
}

func (s *Session) Execute(ctx context.Context, a Attempt) (result Result, retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	opts := aicommon.ConvertConfigToOptions(s.Config)
	input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
	key := "worker-" + uuid.NewString()
	s.InputEventManager.RegisterMirrorOfAIInputEvent(key, func(e *ypb.AIInputEvent) {
		if e.SyncType != aicommon.SYNC_TYPE_USER_INTERVENTION {
			input.SafeFeed(e)
		}
	})
	defer s.InputEventManager.UnregisterMirrorOfAIInputEvent(key)
	hotpatch := s.HotPatchBroadcaster.Subscribe()
	defer s.HotPatchBroadcaster.Unsubscribe(hotpatch)
	s.Timeline.PushText(s.AcquireId(), "[PLAN_TASK_DISPATCH]\nTask: %s (%s)\nAttempt: %d; approved version: %d\nFrozen execution brief: %s", a.Task.Name, a.Task.ID, a.ID, a.PlanVersion, a.Task.Goal)
	fork, err := s.Timeline.ForkForTask(a.Task.Index, a.Task.Name, s.Config, s.Config)
	if err != nil {
		return result, err
	}
	if fork != nil {
		opts = append(opts, aicommon.WithTimeline(fork.Branch))
		defer func() {
			if _, err := fork.MergeBack(); err != nil && retErr == nil {
				retErr = err
			}
		}()
	}
	s.mu.Lock()
	task := s.tasks[a.Task.ID]
	s.mu.Unlock()
	if task == nil {
		return result, fmt.Errorf("worker task was not projected before dispatch")
	}
	aicommon.WithStatefulTaskBaseContext(ctx)(task)
	task.SetName(a.Task.Name)
	opts = append(opts, aicommon.WithContext(ctx), aicommon.WithID(s.Id), aicommon.WithAICallbacks(s.GetRawAICallbacks()), aicommon.WithEventInputChanx(input), aicommon.WithHotPatchOptionChan(hotpatch), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnablePlanAndExec(false), aicommon.WithEmitter(s.GetEmitter().PushEventProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
		e.TaskUUID = task.GetUUID()
		e.TaskId = task.GetId()
		return e
	})))
	opts = append(opts, NativeOptions()...)
	runtime, err := aicommon.AIRuntimeInvokerGetter(ctx, opts...)
	if err != nil {
		return result, err
	}
	cfg := runtime.GetConfig().(*aicommon.Config)
	task.SetEmitter(cfg.GetEmitter())
	runtime.SetCurrentTask(task)
	loop, err := NewWorkerLoop(runtime, reactloops.BasicAICommonConfigOption(cfg)...)
	if err != nil {
		return result, err
	}
	if err := executeLoop(cfg, runtime, loop, task); err != nil {
		return result, err
	}
	result, ok := loop.GetVariable("coordinator_task_result").(Result)
	if !ok || result.Summary == "" {
		return result, fmt.Errorf("worker finished without a result")
	}
	return result, ctx.Err()
}

func (s *Session) Approve(ctx context.Context, p *Plan) (*Plan, error) {
	if s.detached {
		if s.PersistentSessionId == "" || s.GetDB() == nil {
			return nil, fmt.Errorf("detached plans require a persistent session")
		}
		payload := map[string]any{"id": s.Id, "coordinator_id": s.Id, "session_id": s.PersistentSessionId, "re-act_task": s.parentTaskID, "plan_payload": s.query, "detached": true, "selectors": []map[string]any{{"id": "detached-plan-execute-" + s.Id, "value": "continue", "prompt": "允许执行", "prompt_english": "Allow plan execution", "allow_extra_prompt": false}, {"id": "detached-plan-close-" + s.Id, "value": "close", "prompt": "关闭", "prompt_english": "Close review panel", "allow_extra_prompt": false}}, "plans": map[string]any{"root_task": p.Tree, "document": p.Document}, "plans_id": uuid.NewString()}
		if s.parent != nil {
			payload["re-act_id"] = s.parent.Id
		}
		s.EmitJSON(schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE, "detached-plan", payload)
		s.Timeline.PushText(s.AcquireId(), "[DETACHED_PLAN]\nCoordinator: %s\nSession: %s\nPlan document: %s\nPlan data: %s", s.Id, s.PersistentSessionId, p.Document, p.Tree)
		return nil, ErrDetachedPlanPublished
	}
	ep := s.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	selectors := []map[string]any{}
	for i, item := range []struct{ value, zh, en string }{{"freedom-review", "审阅模式", "Review and edit the plan"}, {"unclear", "目标不明确", "Clarify objectives"}, {"incomplete", "有遗漏", "Complete missing work"}, {"create-subtask", "需要拆分子任务", "Split tasks"}, {"continue", "继续执行", "Continue execution"}} {
		selector := map[string]any{"id": fmt.Sprintf("plan-review-suggestion-%s-%d", s.Id, i), "value": item.value, "prompt": item.zh, "prompt_english": item.en, "allow_extra_prompt": item.value != "continue"}
		if item.value == "freedom-review" {
			selector["param_schema"] = promptloader.MustLoad("ai/aid/jsonschema/plan-review/freedom-plan-review.json")
		}
		selectors = append(selectors, selector)
	}
	payload := map[string]any{"id": ep.GetId(), "selectors": selectors, "plans": map[string]any{"root_task": p.Tree, "document": p.Document}, "plans_id": uuid.NewString()}
	if s.ForceManualPlanReview {
		payload["force_manual_review"] = true
	}
	ep.SetReviewMaterials(payload)
	if err := s.SubmitCheckpointRequest(ep.GetCheckpoint(), payload); err != nil {
		s.EmitError("save plan review checkpoint: %v", err)
	}
	s.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, "review-require", payload)
	if s.ForceManualPlanReview {
		s.DoWaitAgreeWithPolicy(ctx, aicommon.AgreePolicyManual, ep)
	} else {
		s.DoWaitAgree(ctx, ep)
	}
	params := ep.GetParams()
	s.ReleaseInteractiveEvent(ep.GetId(), params)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if params == nil {
		return nil, fmt.Errorf("plan review returned no decision")
	}
	s.CallAfterReview(ep.GetSeq(), "Review the coordinator plan", params)
	suggestion := params.GetString("suggestion")
	if suggestion != "continue" && suggestion != "freedom-review" {
		return nil, fmt.Errorf("plan revision requested: %s", params)
	}
	data, document := string(p.Tree), p.Document
	// Yakit's editor submits the final tree with freedom-review. This is the
	// user's approval of that tree, not a request for another planning round.
	if suggestion == "freedom-review" {
		root := params.GetObject("reviewed-task-tree")
		if len(root) == 0 {
			return nil, fmt.Errorf("reviewed-task-tree must be a nonempty object")
		}
		raw, err := json.Marshal(root)
		if err != nil {
			return nil, err
		}
		data = string(raw)
	}
	if plans := params.GetObject("plans"); len(plans) > 0 {
		if raw, ok := plans["document"]; ok {
			var valid bool
			document, valid = raw.(string)
			if !valid {
				return nil, fmt.Errorf("edited document must be a string")
			}
		}
		if root, exists := plans["root_task"]; exists {
			raw, err := json.Marshal(root)
			if err != nil {
				return nil, err
			}
			data = string(raw)
		}
	}
	return ParseReviewedPlan(data, document, p)
}
