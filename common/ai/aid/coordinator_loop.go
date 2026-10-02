package aid

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/loop_coordinator"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// WithCoordinatorLoop replaces the PLAN engine while retaining public PLAN
// entrypoints and events. The default remains legacy for existing integrations.
func WithCoordinatorLoop(enabled bool) aicommon.ConfigOption {
	engine := "legacy"
	if enabled {
		engine = loop_coordinator.Name
	}
	return aicommon.WithPlanEngine(engine)
}

func (c *Coordinator) usesCoordinatorLoop() bool {
	if engine := c.GetConfigString("plan_engine"); engine != "" {
		return engine == loop_coordinator.Name
	}
	// Recovery follows the engine recorded with this plan, even when an older
	// frontend does not know the engine-selection option.
	if c.GetDB() != nil {
		if record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(c.GetDB(), c.GetRuntimeId()); err == nil && record != nil {
			var progress struct {
				State  *json.RawMessage `json:"coordinator_state"`
				Engine string           `json:"plan_engine"`
			}
			if json.Unmarshal([]byte(record.TaskProgress), &progress) == nil {
				return progress.State != nil || progress.Engine == loop_coordinator.Name
			}
		}
	}
	return false
}

type coordinatorLoopBridge struct {
	owner        *Coordinator
	controller   *loop_coordinator.Controller
	mu           sync.Mutex
	last         loop_coordinator.Snapshot
	opened       map[string]bool
	closed       map[string]bool
	planningOnly bool
	detached     bool
	finishLoop   func()
}

func (b *coordinatorLoopBridge) PlanningOnly() bool { return b.detached || b.planningOnly }

func (b *coordinatorLoopBridge) ReportRequired() bool {
	return b.owner.GenerateReport && b.owner.ResultHandler == nil
}

func (b *coordinatorLoopBridge) GetController() *loop_coordinator.Controller { return b.controller }

// LoopFinished releases a focus-owned PLAN route while its output channel is
// still open. Explicit AID entrypoints have no outer route to release here.
func (b *coordinatorLoopBridge) LoopFinished() {
	if b.finishLoop != nil {
		b.finishLoop()
	}
}

func (b *coordinatorLoopBridge) restoreApprovedTree(root *AiTask) error {
	plan, err := coordinatorPlan(root, "")
	if err != nil {
		return err
	}
	s := loop_coordinator.Snapshot{Schema: 1, DraftVersion: 1, ApprovedVersion: 1, Draft: plan, Approved: plan, Attempts: make(map[string]loop_coordinator.Attempt)}
	leaves := make(map[string]*AiTask)
	for _, t := range executableLeafTasks(root) {
		leaves[t.TaskId] = t
	}
	for _, brief := range plan.Tasks {
		t := leaves[brief.ID]
		a := loop_coordinator.Attempt{Task: brief, PlanVersion: 1, State: loop_coordinator.Pending}
		switch t.GetStatus() {
		case aicommon.AITaskState_Completed:
			s.NextAttempt++
			a.ID = s.NextAttempt
			a.State = loop_coordinator.Accepted
			a.Seen = true
			a.Result.Summary = t.GetSummary()
			if a.Result.Summary == "" {
				a.Result.Summary = "Previously completed and reviewed by the legacy PLAN runtime."
			}
		case aicommon.AITaskState_Skipped:
			s.NextAttempt++
			a.ID = s.NextAttempt
			a.State = loop_coordinator.Cancelled
			a.Seen = true
		}
		s.Attempts[brief.ID] = a
	}
	return b.controller.Restore(s)
}

func resetCoordinatorRecovery(s *loop_coordinator.Snapshot, reference string) error {
	if reference == "" {
		return nil
	}
	id := ""
	for key, a := range s.Attempts {
		if key == reference || a.Task.Index == reference {
			id = key
			break
		}
	}
	if id == "" {
		return fmt.Errorf("recovery start task %q not found", reference)
	}
	affected := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for key, a := range s.Attempts {
			for _, dep := range a.Task.DependsOn {
				if affected[dep] && !affected[key] {
					affected[key], changed = true, true
				}
			}
		}
	}
	for key := range affected {
		a := s.Attempts[key]
		a.ID, a.State, a.Seen, a.Result = 0, loop_coordinator.Pending, false, loop_coordinator.Result{}
		a.ReviewReason = "User requested recovery from this task."
		s.Attempts[key] = a
	}
	s.Finished = false
	return nil
}

func newCoordinatorLoopBridge(c *Coordinator) *coordinatorLoopBridge {
	_ = aicommon.WithPlanEngine(loop_coordinator.Name)(c.Config)
	_ = aicommon.WithEnableFunctionCallMode(true)(c.Config)
	_ = aicommon.WithDisableDynamicPlanning(true)(c.Config)
	_ = aicommon.WithAiAgreeRiskControl(loop_coordinator.NativeRiskReview)(c.Config)
	b := &coordinatorLoopBridge{owner: c, opened: make(map[string]bool), closed: make(map[string]bool)}
	b.detached = c.EnableDetachedPlan
	b.controller = loop_coordinator.New(c.GetContext(), b, c.GetPlanExecTaskConcurrency())
	c.coordinatorLoop = b
	return b
}

func (c *Coordinator) decodeCoordinatorTree(data []byte) (*AiTask, error) {
	var wire recoveredTask
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	var validateWire func(*recoveredTask) error
	validateWire = func(task *recoveredTask) error {
		if task == nil || strings.TrimSpace(task.Name) == "" || strings.TrimSpace(task.Goal) == "" {
			return fmt.Errorf("every plan node requires name and goal")
		}
		for _, child := range task.Subtasks {
			if err := validateWire(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validateWire(&wire); err != nil {
		return nil, err
	}
	var bind func(*recoveredTask, *AiTask) *AiTask
	bind = func(src *recoveredTask, parent *AiTask) *AiTask {
		id := src.TaskId
		if id == "" {
			id = "plan-task" + uuid.NewString()
		}
		t := &AiTask{TaskId: id, Coordinator: c, ParentTask: parent, Index: src.Index, Name: src.Name, Goal: src.Goal, SemanticIdentifier: src.SemanticIdentifier, DependsOn: src.DependsOn}
		t.AIStatefulTaskBase = aicommon.NewStatefulTaskBase(id, fmt.Sprintf("任务名称: %s\n任务目标: %s", t.Name, t.Goal), c.GetContext(), c.GetEmitter(), true)
		t.SetName(t.Name)
		t.SetSemanticIdentifier(t.SemanticIdentifier)
		for _, child := range src.Subtasks {
			t.Subtasks = append(t.Subtasks, bind(child, t))
		}
		return t
	}
	return bind(&wire, nil), nil
}

func coordinatorPlan(root *AiTask, document string) (*loop_coordinator.Plan, error) {
	graph, err := buildStrictExecutableTaskGraph(root)
	if err != nil {
		return nil, err
	}
	tree, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	p := &loop_coordinator.Plan{Document: document, Tree: tree}
	for _, n := range graph.nodes {
		p.Tasks = append(p.Tasks, loop_coordinator.Task{ID: n.id, Index: n.task.Index, Name: n.task.Name, Goal: n.task.Goal, DependsOn: append([]string(nil), n.deps...)})
	}
	return p, nil
}

func (b *coordinatorLoopBridge) Prepare(ctx context.Context, data, document string) (*loop_coordinator.Plan, error) {
	// Preparation must not replace the owner's approved root as the old builder
	// does. A shallow owner copy holds only this candidate tree.
	b.mu.Lock()
	candidate := *b.owner
	b.mu.Unlock()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &fields); err != nil {
		return nil, fmt.Errorf("plan_data must be a JSON object: %w", err)
	}
	var root *AiTask
	var err error
	if fields["root_task"] != nil {
		root, err = candidate.decodeCoordinatorTree(fields["root_task"])
	} else if fields["name"] != nil {
		root, err = candidate.decodeCoordinatorTree([]byte(data))
	} else {
		// Parse native arguments through the legacy wire shape without invoking
		// the old plan builder's auxiliary name-generation model.
		var spec struct {
			Name       string            `json:"main_task"`
			Goal       string            `json:"main_task_goal"`
			Identifier string            `json:"main_task_identifier"`
			Tasks      []json.RawMessage `json:"tasks"`
		}
		if err = json.Unmarshal([]byte(data), &spec); err == nil {
			newTask := func(name, goal, identifier string, parent *AiTask) *AiTask {
				id := "plan-task" + uuid.NewString()
				if identifier == "" {
					identifier = name
				}
				t := &AiTask{Coordinator: &candidate, TaskId: id, Name: name, Goal: goal, ParentTask: parent, SemanticIdentifier: aicommon.SanitizeTaskName(identifier)}
				t.AIStatefulTaskBase = aicommon.NewStatefulTaskBase(id, fmt.Sprintf("任务名称: %s\n任务目标: %s", name, goal), ctx, candidate.GetEmitter(), true)
				t.SetName(name)
				return t
			}
			root = newTask(spec.Name, spec.Goal, spec.Identifier, nil)
			var parse func(json.RawMessage, *AiTask) (*AiTask, error)
			parse = func(data json.RawMessage, parent *AiTask) (*AiTask, error) {
				var item struct {
					Name       string            `json:"subtask_name"`
					Goal       string            `json:"subtask_goal"`
					Identifier string            `json:"subtask_identifier"`
					Deps       []string          `json:"depends_on"`
					Children   []json.RawMessage `json:"sub_subtasks"`
				}
				if err := json.Unmarshal(data, &item); err != nil {
					return nil, err
				}
				t := newTask(item.Name, item.Goal, item.Identifier, parent)
				t.DependsOn = item.Deps
				for _, child := range item.Children {
					sub, err := parse(child, t)
					if err != nil {
						return nil, err
					}
					t.Subtasks = append(t.Subtasks, sub)
				}
				return t, nil
			}
			for _, data := range spec.Tasks {
				sub, parseErr := parse(data, root)
				if parseErr != nil {
					err = parseErr
					break
				}
				root.Subtasks = append(root.Subtasks, sub)
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// Stable semantic identities retain logical IDs across revisions, so the UI
	// and accepted results remain bound to the same task. Duplicate identities
	// are rejected by the same strict DAG validator used by legacy PLAN.
	s := b.controller.Snapshot()
	previous := s.Draft
	if previous == nil {
		previous = s.Approved
	}
	if previous != nil {
		old, err := candidate.decodeCoordinatorTree(previous.Tree)
		if err != nil {
			return nil, err
		}
		ids := make(map[string]string)
		var collect func(*AiTask)
		collect = func(t *AiTask) {
			ids[t.GetSemanticIdentifier()] = t.TaskId
			for _, child := range t.Subtasks {
				collect(child)
			}
		}
		collect(old)
		var retain func(*AiTask)
		retain = func(t *AiTask) {
			if id := ids[t.GetSemanticIdentifier()]; id != "" {
				t.TaskId = id
				t.AIStatefulTaskBase = aicommon.NewStatefulTaskBase(id, t.GetUserInput(), candidate.GetContext(), candidate.GetEmitter(), true)
			}
			for _, child := range t.Subtasks {
				retain(child)
			}
		}
		retain(root)
	}
	candidate.standardizeTaskTree(root)
	return coordinatorPlan(root, document)
}

func (b *coordinatorLoopBridge) Approve(ctx context.Context, p *loop_coordinator.Plan) (*loop_coordinator.Plan, error) {
	b.mu.Lock()
	candidate := *b.owner
	b.mu.Unlock()
	root, err := candidate.decodeCoordinatorTree(p.Tree)
	if err != nil {
		return nil, err
	}
	if b.detached {
		if candidate.PersistentSessionId == "" || candidate.GetDB() == nil {
			return nil, fmt.Errorf("detached plans require a persistent session")
		}
		id := candidate.GetRuntimeId()
		selectors := []map[string]any{
			{"id": "detached-plan-execute-" + id, "value": "continue", "prompt": "允许执行", "prompt_english": "Allow plan execution", "allow_extra_prompt": false},
			{"id": "detached-plan-close-" + id, "value": "close", "prompt": "关闭", "prompt_english": "Close review panel", "allow_extra_prompt": false},
		}
		candidate.EmitJSON(schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE, "detached-plan", map[string]any{"id": id, "coordinator_id": id, "session_id": candidate.PersistentSessionId, "re-act_id": candidate.GetConfigString("coordinator_parent_id"), "re-act_task": candidate.GetConfigString("coordinator_parent_task_id"), "detached": true, "selectors": selectors, "plans": &PlanResponse{RootTask: root, Document: p.Document}, "plans_id": uuid.NewString()})
		return nil, loop_coordinator.ErrDetachedPlanPublished
	}
	ep := candidate.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	candidate.EmitRequireReviewForPlan(&PlanResponse{RootTask: root, Document: p.Document}, ep.GetId())
	candidate.waitPlanReviewAgree(ctx, ep)
	params := ep.GetParams()
	candidate.ReleaseInteractiveEvent(ep.GetId(), params)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if params == nil {
		return nil, fmt.Errorf("plan review returned no decision")
	}
	candidate.CallAfterReview(ep.GetSeq(), "Review the coordinator plan", params)
	if params.GetString("suggestion") != "continue" {
		return nil, fmt.Errorf("plan not approved: %s; revise the draft in coordinator", params.GetString("suggestion"))
	}
	// Manual edits use the exact old plans.root_task payload. Revision requests
	// return to the coordinator; no legacy plan/replan JSON loop is entered.
	document := p.Document
	if plans := params.GetObject("plans"); len(plans) > 0 {
		if raw, exists := plans["document"]; exists {
			value, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("edited plan document must be a string")
			}
			document = value
		}
		if edited := plans.GetObject("root_task"); len(edited) > 0 {
			data, err := json.Marshal(edited)
			if err != nil {
				return nil, err
			}
			root, err = candidate.decodeCoordinatorTree(data)
			if err != nil {
				return nil, err
			}
			candidate.standardizeTaskTree(root)
		}
	}
	return coordinatorPlan(root, document)
}

func (b *coordinatorLoopBridge) Execute(ctx context.Context, a loop_coordinator.Attempt) (result loop_coordinator.Result, retErr error) {
	s := b.controller.Snapshot()
	if s.Approved == nil {
		return result, fmt.Errorf("approved plan missing")
	}
	b.mu.Lock()
	worker := *b.owner
	b.mu.Unlock()
	opts := aicommon.ConvertConfigToOptions(b.owner.Config)
	opts = append(opts, aicommon.WithContext(ctx), aicommon.WithAICallbacks(b.owner.GetRawAICallbacks()), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithEnablePlanAndExec(false), aicommon.WithEnableFunctionCallMode(true))
	worker.Config = aicommon.NewConfig(ctx, opts...)
	worker.ContextProvider = GetDefaultContextProvider()
	worker.ContextProvider.SetTimelineInstance(worker.Timeline)
	worker.coordinatorLoop = nil
	root, err := worker.decodeCoordinatorTree(s.Approved.Tree)
	if err != nil {
		return result, err
	}
	worker.rootTask = root
	worker.ContextProvider.StoreRootTask(root)
	var task *AiTask
	for _, t := range executableLeafTasks(root) {
		if t.TaskId == a.Task.ID {
			task = t
			break
		}
	}
	if task == nil {
		return result, fmt.Errorf("task %q disappeared", a.Task.ID)
	}
	// Use the frozen dispatch brief, even if another draft is being edited.
	task.Name = a.Task.Name
	task.Goal = a.Task.Goal
	task.SetUserInput(fmt.Sprintf("任务名称: %s\n任务目标: %s", a.Task.Name, a.Task.Goal))
	worker.runtime = worker.createRuntime()
	worker.runtime.RootTask = root
	if worker.Timeline != nil {
		worker.Timeline.PushText(worker.AcquireId(), "[PLAN_TASK_DISPATCH]\nTask: %s (%s)\nAttempt: %d; approved version: %d\nFrozen execution brief: %s", a.Task.Name, a.Task.ID, a.ID, a.PlanVersion, a.Task.Goal)
	}
	fork, err := worker.runtime.createTaskTimelineFork(task)
	if err != nil {
		return result, err
	}
	restore := task.withTimelineFork(fork)
	defer restore()
	if fork != nil {
		defer func() {
			if _, err := fork.MergeBack(); err != nil && retErr == nil {
				retErr = err
			}
		}()
	}
	task.ForceSetStatus(aicommon.AITaskState_Processing)
	// The new engine has exactly two native-function-call loops. Legacy task
	// summary/review/replan JSON transactions are not entered here.
	if err := worker.executeLoopTask("pe_task", task, loop_coordinator.NewWorkerLoop,
		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, _ int, _ aicommon.AIStatefulTask, isDone bool, _ any, _ *reactloops.OnPostIterationOperator) {
			if isDone {
				if captured, ok := loop.GetVariable("coordinator_task_result").(loop_coordinator.Result); ok {
					result = captured
				}
			}
		})); err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	if result.Summary == "" {
		return result, fmt.Errorf("worker finished without a submitted task result")
	}
	return result, nil
}

func (b *coordinatorLoopBridge) Changed(s loop_coordinator.Snapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c := b.owner
	plan := s.Approved
	if plan == nil {
		plan = s.Draft
	}
	if plan == nil {
		b.last = s
		return
	}
	root, err := c.decodeCoordinatorTree(plan.Tree)
	if err != nil {
		c.EmitError("coordinator projection: %v", err)
		return
	}
	var current *AiTask
	var activeIDs []string
	for _, t := range executableLeafTasks(root) {
		a := s.Attempts[t.TaskId]
		old := b.last.Attempts[t.TaskId]
		state := aicommon.AITaskState_Created
		switch a.State {
		case loop_coordinator.Running, loop_coordinator.Cancelling, loop_coordinator.AwaitingReview:
			state = aicommon.AITaskState_Processing
			current = t
			activeIDs = append(activeIDs, t.TaskId)
		case loop_coordinator.Accepted:
			state = aicommon.AITaskState_Completed
		case loop_coordinator.Cancelled:
			state = aicommon.AITaskState_Skipped
		case loop_coordinator.Failed, loop_coordinator.Rejected:
			state = aicommon.AITaskState_Aborted
		}
		t.ForceSetStatus(state)
		t.ShortSummary = a.Result.Summary
		t.TaskSummary = a.Result.Summary
		if a.State == loop_coordinator.Running && (old.ID != a.ID || old.State != a.State) {
			if !b.opened[t.TaskId] {
				c.EmitPushTask(t)
				b.opened[t.TaskId] = true
			} else if b.closed[t.TaskId] {
				// The existing frontend keys cards by logical ID, not attempt UUID.
				// React-route status reopens that same card without duplicate push.
				publisher := c.GetEmitter().PushEventProcesser(func(e *schema.AiOutputEvent) *schema.AiOutputEvent {
					if id := c.GetConfigString("coordinator_parent_id"); id != "" {
						e.CoordinatorId = id
					}
					return e
				})
				publisher.EmitJSON(schema.EVENT_TYPE_STRUCTURED, "react_task_status_changed", map[string]any{"react_task_id": t.TaskId, "react_task_status": "processing"})
			}
			b.closed[t.TaskId] = false
		}
		if a.State != old.State || a.ID != old.ID || a.ReviewReason != old.ReviewReason {
			c.EmitTextMarkdownStreamEvent("coordinator-task", strings.NewReader(fmt.Sprintf("%s %s · %s (attempt %d)\n%s\n%s", t.Index, t.Name, a.State, a.ID, a.Result.Summary, a.ReviewReason)), t.Index)
			if (a.State == loop_coordinator.Accepted || a.State == loop_coordinator.Cancelled) && b.opened[t.TaskId] && !b.closed[t.TaskId] {
				c.EmitUpdateTaskStatus(t)
				c.EmitPopTask(t)
				b.closed[t.TaskId] = true
			}
		}
	}
	c.rootTask = root
	if s.Finished {
		root.ForceSetStatus(aicommon.AITaskState_Completed)
	}
	c.runtime = c.createRuntime()
	c.runtime.RootTask = root
	c.runtime.activeTaskIDs = activeIDs
	c.ContextProvider.StoreRootTask(root)
	if s.Approved != nil && s.ApprovedVersion != b.last.ApprovedVersion && strings.TrimSpace(s.Approved.Document) != "" {
		appendPlanDocumentFrozenPartition(c.Config, fmt.Sprintf("Approved PLAN version %d\n%s", s.ApprovedVersion, s.Approved.Document))
	}
	c.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{"root_task": root})
	phase := Phase_NotCompleted
	if s.Approved == nil {
		phase = aicommon.PlanExecPhaseDetachedPendingApproval
	}
	if s.Finished {
		phase = Phase_Completed
	} else if b.planningOnly && s.Approved != nil {
		phase = Phase_PlanReady
	}
	progress := c.buildPlanAndExecProgress(root, current, phase)
	if c.PersistentSessionId != "" && c.GetDB() != nil {
		payload, _ := json.Marshal(progress)
		var fields map[string]any
		_ = json.Unmarshal(payload, &fields)
		fields["coordinator_state"] = s
		progressJSON, _ := json.Marshal(fields)
		treeJSON, _ := json.Marshal(root)
		if err := yakit.CreateOrUpdateAISessionPlanAndExec(c.GetDB(), &schema.AISessionPlanAndExec{SessionID: c.PersistentSessionId, CoordinatorID: c.GetRuntimeId(), TaskTree: string(treeJSON), TaskProgress: string(progressJSON)}); err != nil {
			log.Warnf("save coordinator state: %v", err)
		}
	}
	b.last = s
}

func (c *Coordinator) runCoordinatorLoop() error {
	return c.runCoordinatorLoopMode(false)
}

func (c *Coordinator) runCoordinatorLoopMode(planningOnly bool) error {
	b := newCoordinatorLoopBridge(c)
	b.planningOnly = planningOnly
	defer b.controller.Close()
	c.registerPEModeInputEventCallback()
	defer c.unregisterPEModeInputEventCallback()
	wakeID := "coordinator-wake-" + c.GetRuntimeId()
	c.InputEventManager.RegisterAfterInputEvent(wakeID, func(e *ypb.AIInputEvent) {
		// Free input belongs to the outer queue's next task. Current-task
		// interventions are journaled by their sync handler before this callback.
		if e.IsInteractiveMessage || e.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION {
			b.controller.Wake()
		}
	})
	defer c.InputEventManager.UnregisterAfterInputEvent(wakeID)
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()
	// Persisted coordinator snapshots take precedence over legacy progress.
	if c.GetDB() != nil {
		if record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(c.GetDB(), c.GetRuntimeId()); err == nil && record != nil {
			var persisted struct {
				State *loop_coordinator.Snapshot `json:"coordinator_state"`
			}
			_ = json.Unmarshal([]byte(record.TaskProgress), &persisted)
			if persisted.State != nil {
				if err := resetCoordinatorRecovery(persisted.State, c.getRecoveryStartTaskID()); err != nil {
					return err
				}
				if err := b.controller.Restore(*persisted.State); err != nil {
					return err
				}
			}
		}
	}
	if b.controller.Snapshot().Draft == nil && c.rootTask != nil {
		if err := b.restoreApprovedTree(c.rootTask); err != nil {
			return err
		}
	}
	if b.controller.Snapshot().Draft == nil {
		root, _, recovered, err := c.tryRecoverPlanAndExec(c.getRecoveryStartTaskID())
		if err != nil {
			return err
		}
		if recovered {
			if err = b.restoreApprovedTree(root); err != nil {
				return err
			}
		}
	}
	// One dedicated task owns the planner loop; worker completion never cancels
	// this task or the enclosing ReAct task.
	if b.controller.Snapshot().Approved != nil {
		b.detached = false
	} else if b.detached {
		planningOnly = true
	}
	task := aicommon.NewStatefulTaskBase("coordinator-"+c.GetRuntimeId(), c.userInput, c.GetContext(), c.GetEmitter(), true)
	options := []reactloops.ReActLoopOption{loop_coordinator.WithController(b.controller)}
	if planningOnly {
		options = append(options, loop_coordinator.WithPlanningOnly())
	}
	if err := c.ExecuteLoopTask(loop_coordinator.Name, task, options...); err != nil {
		return err
	}
	if planningOnly {
		if err := b.controller.CanFinishPlanning(); err != nil {
			return err
		}
		phase := Phase_PlanReady
		if b.detached {
			phase = aicommon.PlanExecPhaseDetachedPendingApproval
		}
		c.savePlanAndExecState(phase, nil)
		c.planUserStatus("执行方案已经准备好，等你开始", "The execution plan is ready to start", aicommon.WithStatusCode("plan.ready"), aicommon.WithStatusState(aicommon.StatusStateWaiting))
		c.Wait()
		return nil
	}
	if err := b.controller.CanFinish(); err != nil {
		return err
	}
	if c.ResultHandler != nil {
		c.ResultHandler(c)
	}
	c.savePlanAndExecState(Phase_Completed, nil)
	c.planUserStatus("任务已经处理完成", "The task has been completed", aicommon.WithStatusCode("plan.completed"), aicommon.WithStatusState(aicommon.StatusStateSuccess))
	c.Wait()
	return nil
}

func (b *coordinatorLoopBridge) control(event *ypb.AIInputEvent, retry bool) error {
	var p struct {
		ID      string `json:"subtask_id"`
		Index   string `json:"subtask_index"`
		Reason  string `json:"reason"`
		Message string `json:"user_message"`
	}
	err := json.Unmarshal([]byte(event.SyncJsonInput), &p)
	s := b.controller.Snapshot()
	if p.ID == "" {
		for id, a := range s.Attempts {
			if a.Task.Index == p.Index {
				p.ID = id
				break
			}
		}
	}
	if err == nil && p.ID == "" {
		err = fmt.Errorf("subtask_id or subtask_index is required")
	}
	if retry && p.Message == "" {
		err = fmt.Errorf("user_message is required")
	}
	if err == nil && retry && s.Attempts[p.ID].State != loop_coordinator.Accepted {
		err = fmt.Errorf("only completed plan tasks can be redone through this control")
	}
	if err == nil && !retry && s.Attempts[p.ID].State == loop_coordinator.Accepted {
		err = fmt.Errorf("completed plan tasks cannot be skipped")
	}
	if err == nil {
		reason := p.Reason
		if retry {
			reason = p.Message
		}
		if reason == "" {
			reason = "User requested skipping this plan task."
		}
		b.owner.AppendUserInputHistory(reason, time.Now())
		b.controller.Wake()
		if retry {
			a, ok := s.Attempts[p.ID]
			if !ok {
				err = fmt.Errorf("unknown task")
			} else {
				_, err = b.controller.RetryTask(p.ID, a.ID, reason)
			}
		} else {
			err = b.controller.CancelTasks([]string{p.ID}, reason)
		}
	}
	node := "skip_subtask_in_plan"
	if retry {
		node = "redo_subtask_in_plan"
	}
	ack := func(err error) {
		payload := map[string]any{"success": err == nil, "subtask_id": p.ID}
		if err != nil {
			payload["error"] = err.Error()
		}
		b.owner.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, node, payload, event.SyncID)
	}
	if err != nil || retry {
		ack(err)
		return nil
	}
	// Preserve the old skip acknowledgement: send success only when the owned
	// worker has stopped, not merely when its cancellation has been requested.
	go func() {
		for {
			result, err := b.controller.WaitTasks(b.owner.GetContext(), []string{p.ID}, 30*time.Second)
			if err != nil {
				ack(err)
				return
			}
			if len(result.Tasks) > 0 && result.Tasks[0].State != loop_coordinator.Cancelling && result.Tasks[0].State != loop_coordinator.Running {
				ack(nil)
				return
			}
		}
	}()
	return nil
}

func init() {
	loop_coordinator.HostFactory = func(r aicommon.AIInvokeRuntime) (loop_coordinator.Host, error) {
		parent, ok := r.GetConfig().(*aicommon.Config)
		if !ok {
			return nil, fmt.Errorf("coordinator requires a Config-backed runtime")
		}
		if !parent.EnablePlanAndExec {
			return nil, fmt.Errorf("PLAN is disabled for this session")
		}
		task := r.GetCurrentTask()
		if task == nil {
			return nil, fmt.Errorf("coordinator requires a current task")
		}
		ctx := task.GetContext()
		uid := uuid.NewString()
		input := chanx.NewUnlimitedChan[*ypb.AIInputEvent](ctx, 10)
		parent.InputEventManager.RegisterMirrorOfAIInputEvent(uid, func(e *ypb.AIInputEvent) {
			// The parent owns the shared session's intervention journal. Forwarding
			// it to each child would append the same user input multiple times.
			if e.SyncType != aicommon.SYNC_TYPE_USER_INTERVENTION {
				input.SafeFeed(e)
			}
		})
		opts := aicommon.ConvertConfigToOptions(parent)
		opts = append(opts, aicommon.WithID(uid), aicommon.WithContext(ctx), aicommon.WithEventInputChanx(input), aicommon.WithAICallbacks(parent.GetRawAICallbacks()), WithCoordinatorLoop(true), aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = uid
			if e.Type == schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE {
				e.CoordinatorId = parent.Id
			}
			parent.EventHandler(e)
		}))
		c, err := NewCoordinatorContext(ctx, task.GetUserInput(), opts...)
		if err != nil {
			parent.InputEventManager.UnregisterMirrorOfAIInputEvent(uid)
			return nil, err
		}
		c.SetConfig("coordinator_parent_id", parent.Id)
		c.SetConfig("coordinator_parent_task_id", task.GetId())
		b := newCoordinatorLoopBridge(c)
		c.registerPEModeInputEventCallback()
		wakeID := "coordinator-wake-" + uid
		parent.InputEventManager.RegisterAfterInputEvent(wakeID, func(e *ypb.AIInputEvent) {
			if e.IsInteractiveMessage || e.SyncType == aicommon.SYNC_TYPE_USER_INTERVENTION {
				b.controller.Wake()
			}
		})
		if !b.detached {
			parent.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, "plan", map[string]any{"coordinator_id": uid, "re-act_id": parent.Id, "re-act_task": task.GetId()})
		}
		var closeOnce sync.Once
		b.finishLoop = func() {
			closeOnce.Do(func() {
				b.controller.Close()
				c.unregisterPEModeInputEventCallback()
				parent.InputEventManager.UnregisterAfterInputEvent(wakeID)
				parent.InputEventManager.UnregisterMirrorOfAIInputEvent(uid)
				if !b.detached {
					parent.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, "plan", map[string]any{"coordinator_id": uid, "re-act_id": parent.Id, "re-act_task": task.GetId()})
				}
			})
		}
		go func() { <-ctx.Done(); b.LoopFinished() }()
		return b, nil
	}
}
