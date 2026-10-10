package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Session owns the new coordinator's lifecycle. No legacy Coordinator, AiTask,
// plan builder, execution runtime or persistence implementation is involved.
type Session struct {
	*aicommon.Config
	invoker                aicommon.AITaskInvokeRuntime
	controller             *Controller
	query                  string
	cancel                 context.CancelFunc
	cleanup                func()
	parent                 *aicommon.Config
	parentTaskID           string
	planningOnly           bool
	delivery               ResultDelivery
	externalLifecycle      bool
	detached               bool
	startTaskID            string
	mu                     sync.Mutex
	evidenceMu             sync.Mutex
	last                   Snapshot
	lastTree               json.RawMessage
	opened, closed         map[string]bool
	tasks                  map[string]*aicommon.AIStatefulTaskBase
	workerTimelines        map[string]*aicommon.Timeline
	stateErr               error
	taskReviewEndpoints    map[string]bool
	recordedReviewFeedback map[string]bool
	planReviewEndpoints    map[string]bool
	persisted              uint64
	closeOnce              sync.Once
}

func NewSession(ctx context.Context, query string, opts ...aicommon.ConfigOption) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	opts = append(append([]aicommon.ConfigOption{}, opts...), NativeOptions()...)
	opts = append(opts, aicommon.WithContext(ctx))
	opts = append(opts, func(cfg *aicommon.Config) error {
		cfg.EnhanceKnowledgeManager = cfg.EnhanceKnowledgeManager.ForkForSubAgent()
		return nil
	})
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
	if content := strings.TrimSpace(strings.Join(cfg.PersistentMemory, "\n")); content != "" {
		// Legacy Forge exposed persistent business instructions in frozen context.
		// Own a copy so adding this invocation's instructions cannot change the
		// parent's prefix. A stable ID replaces inherited context without doubling it.
		producer := aicommon.NewFrozenBlockPartitionProducer(aicommon.FrozenBlockPartitionsFromConfig(cfg)...)
		producer.AppendNewPartition("persistent_context", "persistent_context", content, aicommon.PersistentMemoryOrder)
		_ = aicommon.WithFrozenBlockPartitionProducer(producer)(cfg)
	}
	s := &Session{Config: cfg, invoker: runtime, query: query, cancel: cancel, detached: cfg.EnableDetachedPlan, opened: map[string]bool{}, closed: map[string]bool{}, tasks: map[string]*aicommon.AIStatefulTaskBase{}}
	for _, option := range cfg.OtherOption {
		if d, ok := option.(deliveryOption); ok {
			s.delivery = d.handler
		}
	}
	cfg.Guardian.SetOutputEmitter(cfg.Id, cfg.EventHandler)
	cfg.Guardian.SetAICaller(cfg)
	if cfg.EnableAISearch {
		if err := s.configureToolSearch(); err != nil {
			cancel()
			return nil, err
		}
	}
	s.controller = New(ctx, s, cfg.GetPlanExecTaskConcurrency())
	s.controller.patchDir = filepath.Join(cfg.GetOrCreateWorkDir(), "artifacts", "coordinator-"+cfg.GetRuntimeId(), "plan-patches")
	return s, nil
}

// FromRuntime is the outer-session adapter. Input is journaled once by the
// parent; only completed notifications wake the coordinator. Each child owns
// its event processor and lifecycle, so no handlers overwrite the parent.
func FromRuntime(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, id string) (*Session, error) {
	return fromRuntime(ctx, r, task, id, false)
}

func fromRuntime(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, id string, forge bool, extra ...aicommon.ConfigOption) (*Session, error) {
	if r == nil {
		return nil, fmt.Errorf("coordinator requires a parent runtime")
	}
	parent, ok := r.GetConfig().(*aicommon.Config)
	if !ok || task == nil {
		return nil, fmt.Errorf("coordinator requires a session task")
	}
	if !parent.EnablePlanAndExec && !forge {
		return nil, fmt.Errorf("PLAN is disabled for this session")
	}
	if id == "" {
		id = uuid.NewString()
	}
	if ctx == nil {
		ctx = task.GetContext()
	}
	ctx, cancel := context.WithCancel(ctx)
	// The Blueprint adapter already owns a private event channel and hotpatch
	// subscription. Adopt them for Forge instead of adding a second mirror.
	owned := []aicommon.ConfigOption{aicommon.WithID(id), aicommon.WithContext(ctx)}
	var input *chanx.UnlimitedChan[*ypb.AIInputEvent]
	var hotpatch *chanx.UnlimitedChan[aicommon.ConfigOption]
	if !forge {
		input = chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
		hotpatch = parent.HotPatchBroadcaster.Subscribe()
		owned = append(owned, aicommon.WithEventInputChanx(input), aicommon.WithHotPatchOptionChan(hotpatch))
	}
	var opts []aicommon.ConfigOption
	if !forge {
		opts = aicommon.ConvertConfigToOptionsWithoutHotPatch(parent)
		opts = append(opts, nativePlanOptions(parent)...)
		opts = append(opts, aicommon.WithPlanPrompt(parent.PlanPrompt))
	}
	// Forge's outer adapter already supplied the inherited configuration.
	// Reapplying it here would duplicate append-only options and could carry
	// the parent's preset plan into an unrelated Blueprint invocation.
	opts = append(opts, aicommon.WithForceManualPlanReview(parent.ForceManualPlanReview))
	opts = append(opts, aicommon.WithAICallbacks(parent.GetRawAICallbacks()), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
		e.CoordinatorId = id
		if e.Type == schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE {
			e.CoordinatorId = parent.Id
		}
		if parent.EventHandler != nil {
			parent.EventHandler(e)
		}
	}))
	opts = append(opts, extra...)
	opts = append(opts, owned...)
	s, err := NewSession(ctx, task.GetUserInput(), opts...)
	if err != nil {
		cancel()
		if !forge {
			parent.HotPatchBroadcaster.Unsubscribe(hotpatch)
		}
		return nil, err
	}
	s.parent, s.parentTaskID = parent, task.GetId()
	// Preserve the caller's database boundary, including isolated test/session stores.
	s.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(id, parent.GetDB())
	key := "coordinator-input-" + id
	if !forge {
		parent.InputEventManager.RegisterMirrorOfAIInputEvent(key, func(e *ypb.AIInputEvent) {
			switch e.SyncType {
			case aicommon.SYNC_TYPE_USER_INTERVENTION, "queue_info", "react_cancel_task", "react_cancel_current_task":
				// The outer runtime owns queue cancellation. Its context already
				// cancels this child; replaying the root task ID here is invalid.
				return
			}
			input.SafeFeed(e)
		})
	}
	parent.InputEventManager.RegisterAfterInputEvent(key, func(e *ypb.AIInputEvent) {
		if e.IsInteractiveMessage || e.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION {
			s.notifyUserInput(e)
		}
	})
	s.cleanup = func() {
		if !forge {
			parent.InputEventManager.UnregisterMirrorOfAIInputEvent(key)
			parent.HotPatchBroadcaster.Unsubscribe(hotpatch)
		}
		parent.InputEventManager.UnregisterAfterInputEvent(key)
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
	// A directly submitted plan is live while its approval call is pending,
	// even though no ReAct loop has started yet.
	runningSessions.Store(s.Id, s)
	defer runningSessions.Delete(s.Id)
	s.detached, s.planningOnly = detached, true
	_ = aicommon.WithForceManualPlanReview(forceManual)(s.Config)
	_, err := s.controller.CreatePlan(ctx, input.PlanData, input.PlanDocument)
	if err != nil {
		return nil, err
	}
	if err := s.controller.SubmitPlan(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	err = s.stateErr
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	snapshot := s.controller.Snapshot()
	p := snapshot.Plan
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
	previous := snapshot.Plan
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

	s.detached = false
	return s.controller.LoadApproved(p)
}

func (s *Session) restore() error {
	if s.controller.Snapshot().Plan != nil {
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
		return fmt.Errorf("stored plan belongs to coordinator_legacy, whose execution route is disabled; create a new native coordinator plan")
	}
	if progress.State == nil {
		return fmt.Errorf("coordinator snapshot is missing; refusing implicit legacy recovery")
	}
	if err := resetCoordinatorRecovery(progress.State, s.startTaskID); err != nil {
		return err
	}
	if progress.State.Phase == PhaseExec {
		s.detached = false
	}
	return s.controller.Restore(*progress.State)
}

func (s *Session) run(planningOnly bool) (err error) {
	runningSessions.Store(s.Id, s)
	defer func() { s.Close(); s.controller.owned.Wait(); runningSessions.Delete(s.Id) }()
	s.planningOnly = planningOnly
	if err = s.restore(); err != nil {
		return err
	}
	if s.controller.Snapshot().Phase == PhaseExec {
		s.detached = false
	}
	s.registerControls()
	s.EmitCurrentConfigInfo()
	key := "coordinator-wake-" + s.GetRuntimeId()
	s.InputEventManager.RegisterAfterInputEvent(key, func(e *ypb.AIInputEvent) {
		// Forwarded input is journaled and notified by its owning parent once.
		if s.parent == nil && (e.IsInteractiveMessage || e.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION) {
			s.notifyUserInput(e)
		}
	})
	defer s.InputEventManager.UnregisterAfterInputEvent(key)
	if s.parent != nil && !s.detached && !s.externalLifecycle {
		payload := map[string]any{"coordinator_id": s.Id, "re-act_id": s.parent.Id, "re-act_task": s.parentTaskID, "start_task_id": s.startTaskID}
		s.parent.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, "plan", payload)
		defer func() {
			payload["completed"] = err == nil && s.Snapshot().Finished
			if err != nil {
				s.controller.Close()
				s.controller.owned.Wait()
				payload["completed"] = false
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
	if _, enabled := s.Config.MemoryTriage.(aimem.TimelineMemoryPersister); enabled && s.parent == nil && !s.PlanningOnly() {
		release := task.DeferCompletion()
		defer release()
		defer func() {
			if err != nil || s.GetContext().Err() != nil {
				if saveErr := s.Timeline.RequestMemoryFinalization(); saveErr != nil {
					s.EmitError("save interrupted PLAN memory source failed: %v", saveErr)
				}
			}
		}()
	}
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
	if !s.PlanningOnly() {
		interval := s.IntervalReviewDuration
		if interval <= 0 {
			interval = 60 * time.Second
		}
		if s.DisableIntervalReview {
			interval = 0
		}
		s.controller.EnableExecution(usesManualTaskReview(s.AgreePolicy), interval)
	}
	err = executeLoop(s.Config, s.invoker, loop, task)
	if err != nil {
		return err
	}
	if s.PlanningOnly() {
		err = s.controller.CanFinishPlanning()
	} else {
		err = s.controller.CanFinish()
		if err == nil && (!s.Snapshot().Finished || !s.Snapshot().Report.Submitted) {
			err = fmt.Errorf("EXEC ended without a current submitted report and host completion")
		}
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	err = s.stateErr
	s.mu.Unlock()
	if err == nil && s.parent == nil && !s.PlanningOnly() {
		if finalizeErr := aimem.CompleteTimelineMemory(s.Config, s.GetContext(), s.query); finalizeErr != nil {
			s.EmitError("memory finalization pending: %v", finalizeErr)
		}
	}
	return err
}

func executeLoop(cfg *aicommon.Config, invoker aicommon.AITaskInvokeRuntime, loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	invoker.SetCurrentTask(task)
	aicommon.BeginSessionSnapshotExecutionForTask(cfg, task, time.Now())
	reactloops.EmitSessionSnapshot(cfg, loop, task)
	err := loop.ExecuteWithExistedTask(task)
	if waitErr, ok := loop.GetVariable("coordinator_wait_error").(error); err == nil && ok {
		err = waitErr
	}
	status := aicommon.SessionSnapshotStatusFromTask(task)
	if status == "processing" {
		if err != nil {
			status = "aborted"
		} else {
			status = "completed"
		}
	}
	cfg.FinalizeSessionSnapshotExecution(status, time.Now())
	reactloops.EmitSessionSnapshot(cfg, loop, task)
	return err
}

func (s *Session) Execute(ctx context.Context, a Attempt) (result Result, retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	opts := aicommon.ConvertConfigToOptionsWithoutHotPatch(s.Config)
	opts = append(opts, aicommon.WithEnhanceKnowledgeManager(s.EnhanceKnowledgeManager.ForkForSubAgent()))
	if a.Plan != nil {
		state := s.GetSessionPromptState().ForkForSubAgent()
		opts = append(opts, aicommon.WithSessionPromptState(state), aicommon.WithFrozenBlockPartitionProducer(aicommon.NewFrozenBlockPartitionProducer(s.GetOrCreateFrozenBlockPartitionProducer().ProducePartitions()...)))
	}
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
	s.Timeline.PushText(s.AcquireId(), "[PLAN_TASK_DISPATCH]\nTask: %s (%s)\nAttempt: %d\nFrozen execution brief: %s", a.Task.Name, a.Task.ID, a.ID, a.Task.Goal)
	fork, err := s.Timeline.ForkForTask(a.Task.Index, a.Task.Name, s.Config, s.Config)
	if err != nil {
		return result, err
	}
	if fork != nil {
		opts = append(opts, aicommon.WithTimeline(fork.Branch))
		s.mu.Lock()
		if s.workerTimelines == nil {
			s.workerTimelines = map[string]*aicommon.Timeline{}
		}
		s.workerTimelines[a.Task.ID] = fork.Branch
		s.mu.Unlock()
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
	opts = append(opts, aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithContext(ctx), aicommon.WithID(s.Id), aicommon.WithAICallbacks(s.GetRawAICallbacks()), aicommon.WithEventInputChanx(input), aicommon.WithHotPatchOptionChan(hotpatch), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnablePlanAndExec(false), aicommon.WithEmitter(s.GetEmitter().PushEventProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
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
	cfg.BaseCheckpointableStorage = aicommon.NewCheckpointableStorageWithDB(cfg.GetRuntimeId(), s.GetDB())
	task.SetEmitter(cfg.GetEmitter())
	runtime.SetCurrentTask(task)
	if a.Plan != nil {
		cfg.AppendFrozenBlockPartition("plan_document", "PLAN DOCUMENT", "# PLAN DOCUMENT\n"+a.Plan.Document, aicommon.PlanDocumentFrozenPartitionOrder)
		cfg.AppendFrozenBlockPartition("plan_definition", "PLAN DEFINITION", (Snapshot{Plan: a.Plan}).PlanDefinition(), aicommon.PlanDocumentFrozenPartitionOrder+1)
	}
	workerOptions := append(reactloops.BasicAICommonConfigOption(cfg), func(l *reactloops.ReActLoop) {
		// session 相同，但每次任务尝试的结果记录必须独立。
		l.Set("coordinator_worker_attempt", workerAttemptRef{TaskID: a.Task.ID, AttemptID: a.ID})
		l.Set("coordinator_discovery_callback", func(id, content string) error {
			return s.saveTaskDiscovery(a, id, content)
		})
	})
	brief, _ := json.Marshal(map[string]any{"task_id": a.Task.ID, "attempt_id": a.ID, "name": a.Task.Name, "goal": a.Task.Goal, "accepted_predecessors": taskResultRecords(a.Predecessors), "unaccepted_prior_results": taskResultRecords(a.PriorResults)})

	loop, err := NewWorkerLoop(runtime, workerOptions...)
	if err != nil {
		return result, err
	}
	// Apply after NewWorkerLoop's invariant role options so the frozen brief is
	// not overwritten by WithPersistentInstruction. This is scoped to this worker.
	reactloops.WithPersistentContextProvider(func(l *reactloops.ReActLoop, _ string) (string, error) {
		role, err := utils.RenderTemplate(workerInstruction, map[string]any{"FunctionCallMode": l.FunctionCallModeEnabled()})
		if err != nil {
			return "", err
		}
		return role + "\n[CURRENT_EXECUTION]\n" + string(brief) + "\n本次任务书已冻结；基于直接前置的已验收结果执行并验证。历史初步结果仅供背景参考。", nil
	})(loop)
	if err := executeLoop(cfg, runtime, loop, task); err != nil {
		// Preserve an already-submitted partial result when later execution fails.
		result, _ = loop.GetVariable("coordinator_task_result").(Result)
		return result, err
	}
	result, ok := loop.GetVariable("coordinator_task_result").(Result)
	if !ok || result.Summary == "" {
		return result, fmt.Errorf("worker finished without a result")
	}
	return result, ctx.Err()
}

func (s *Session) Approve(ctx context.Context, p *Plan) (*Plan, error) {
	displayTree, err := displayPlanTree(p)
	if err != nil {
		return nil, err
	}
	if s.detached {
		if s.PersistentSessionId == "" || s.GetDB() == nil {
			return nil, fmt.Errorf("detached plans require a persistent session")
		}
		payload := map[string]any{"id": s.Id, "coordinator_id": s.Id, "session_id": s.PersistentSessionId, "re-act_task": s.parentTaskID, "plan_payload": s.query, "detached": true, "selectors": []map[string]any{{"id": "detached-plan-execute-" + s.Id, "value": "continue", "prompt": "允许执行", "prompt_english": "Allow plan execution", "allow_extra_prompt": false}, {"id": "detached-plan-close-" + s.Id, "value": "close", "prompt": "关闭", "prompt_english": "Close review panel", "allow_extra_prompt": false}}, "plans": map[string]any{"root_task": displayTree, "document": p.Document}, "plans_id": uuid.NewString()}
		if s.parent != nil {
			payload["re-act_id"] = s.parent.Id
		}
		s.EmitJSON(schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE, "detached-plan", payload)
		s.Timeline.PushText(s.AcquireId(), "[DETACHED_PLAN]\nCoordinator: %s\nSession: %s\n等待用户审核当前 PLAN DOCUMENT 和 PLAN DEFINITION。", s.Id, s.PersistentSessionId)
		return nil, ErrDetachedPlanPublished
	}
	ep := s.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	s.mu.Lock()
	if s.planReviewEndpoints == nil {
		s.planReviewEndpoints = map[string]bool{}
	}
	s.planReviewEndpoints[ep.GetId()] = true
	s.mu.Unlock()
	selectors := []map[string]any{}
	for i, item := range []struct{ value, zh, en string }{{"freedom-review", "审阅模式", "Review and edit the plan"}, {"unclear", "目标不明确", "Clarify objectives"}, {"incomplete", "有遗漏", "Complete missing work"}, {"create-subtask", "需要拆分子任务", "Split tasks"}, {"continue", "继续执行", "Continue execution"}} {
		selector := map[string]any{"id": fmt.Sprintf("plan-review-suggestion-%s-%d", s.Id, i), "value": item.value, "prompt": item.zh, "prompt_english": item.en, "allow_extra_prompt": item.value != "continue"}
		if item.value == "freedom-review" {
			selector["param_schema"] = promptloader.MustLoad("ai/aid/jsonschema/plan-review/freedom-plan-review.json")
		}
		selectors = append(selectors, selector)
	}
	payload := map[string]any{"id": ep.GetId(), "selectors": selectors, "plans": map[string]any{"root_task": displayTree, "document": p.Document}, "plans_id": uuid.NewString()}
	if s.ForceManualPlanReview {
		payload["force_manual_review"] = true
	}
	ep.SetReviewMaterials(payload)
	if err := s.SubmitCheckpointRequest(ep.GetCheckpoint(), payload); err != nil {
		return nil, fmt.Errorf("save plan review checkpoint: %w", err)
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
		return nil, s.planRevisionFeedback(ep.GetId(), params)
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
