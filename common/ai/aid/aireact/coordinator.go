package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/loop_coordinator"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Version selection happens at this outer boundary, before either runtime is
// constructed. Recovery uses the persisted owner, not the current UI selection.
func (r *ReAct) coordinatorChannel(id string) (string, error) {
	if id != "" && r.config.GetDB() != nil {
		record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(r.config.GetDB(), id)
		if err != nil {
			return "", err
		}
		var progress struct {
			Engine string          `json:"plan_engine"`
			State  json.RawMessage `json:"coordinator_state"`
		}
		if err := json.Unmarshal([]byte(record.TaskProgress), &progress); err != nil {
			return "", err
		}
		if progress.Engine != "" && progress.Engine != loop_coordinator.Name && progress.Engine != "legacy" && progress.Engine != loop_coordinator.LegacyName {
			return "", fmt.Errorf("unknown stored plan engine %q", progress.Engine)
		}
		hasSnapshot := len(progress.State) > 0 && string(progress.State) != "null"
		if hasSnapshot && (progress.Engine == "legacy" || progress.Engine == loop_coordinator.LegacyName) {
			return "", fmt.Errorf("stored legacy plan contains a native coordinator snapshot")
		}
		if progress.Engine == loop_coordinator.Name || hasSnapshot {
			return loop_coordinator.Name, nil
		}
		return loop_coordinator.LegacyName, nil
	}
	if r.config.Focus == loop_coordinator.Name {
		return loop_coordinator.Name, nil
	}
	return loop_coordinator.LegacyName, nil
}

type nativePlanCoordinatorSession struct {
	r               *ReAct
	session         *loop_coordinator.Session
	input, approved *aicommon.ExecutePlanInput
	manual          bool
	events          map[string]any
}

func (s *nativePlanCoordinatorSession) CoordinatorID() string { return s.session.Id }
func (s *nativePlanCoordinatorSession) ApprovedPlanInput() *aicommon.ExecutePlanInput {
	return s.approved
}
func (s *nativePlanCoordinatorSession) Close() { s.session.Close() }
func (s *nativePlanCoordinatorSession) ReviewPlan(ctx context.Context) error {
	var err error
	s.approved, err = s.session.SubmitInput(ctx, s.input, false, s.manual)
	if err != nil {
		s.r.EmitPlanExecFail(err.Error())
		s.r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, "plan", s.events)
	}
	return err
}

func (r *ReAct) BeginPlanCoordinatorSession(ctx context.Context, input *aicommon.ExecutePlanInput, forceManual bool) (aicommon.PlanCoordinatorSession, error) {
	channel, err := r.coordinatorChannel("")
	if err != nil {
		return nil, err
	}
	if channel == loop_coordinator.LegacyName {
		return r.beginLegacyPlanCoordinatorSession(ctx, input, forceManual)
	}
	if input == nil || strings.TrimSpace(input.PlanData) == "" {
		return nil, fmt.Errorf("execute plan input is empty")
	}
	session, err := r.newNativeInputSession(ctx, input.PlanPayload, "")
	if err != nil {
		return nil, err
	}
	events := map[string]any{"coordinator_id": session.Id, "re-act_id": r.config.Id, "mode": "request_plan"}
	if task := r.GetCurrentTask(); task != nil {
		events["re-act_task"] = task.GetId()
	}
	r.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, "plan", events)
	return &nativePlanCoordinatorSession{r: r, session: session, input: input, manual: forceManual, events: events}, nil
}

func (r *ReAct) newNativeInputSession(ctx context.Context, query, taskID string) (*loop_coordinator.Session, error) {
	if ctx == nil {
		ctx = r.config.GetContext()
	}
	task := r.GetCurrentTask()
	if task == nil || taskID != "" {
		if taskID == "" {
			taskID = "coordinator-request"
		}
		task = aicommon.NewStatefulTaskBase(taskID, query, ctx, r.Emitter, true)
	}
	return loop_coordinator.FromRuntime(ctx, r, task, "")
}

func (r *ReAct) PublishDetachedPlan(ctx context.Context, input *aicommon.ExecutePlanInput, reactTaskID string) (string, error) {
	channel, err := r.coordinatorChannel("")
	if err != nil {
		return "", err
	}
	if channel == loop_coordinator.LegacyName {
		return r.publishLegacyDetachedPlan(ctx, input, reactTaskID)
	}
	if input == nil {
		return "", fmt.Errorf("execute plan input is nil")
	}
	session, err := r.newNativeInputSession(ctx, input.PlanPayload, reactTaskID)
	if err != nil {
		return "", err
	}
	defer session.Close()
	if _, err := session.SubmitInput(ctx, input, true, false); err != nil {
		return "", err
	}
	return session.Id, nil
}

func (r *ReAct) invokePlanOnly(done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	channel, err := r.coordinatorChannel(newInvokePlanAndExecuteOptions(opts...).coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	if channel == loop_coordinator.LegacyName {
		return r.invokeLegacyPlanOnly(done, ctx, opts...)
	}
	return r.invokeNativeCoordinator(done, ctx, true, opts...)
}

func (r *ReAct) invokePlanExecuteOnly(done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	channel, err := r.coordinatorChannel(newInvokePlanAndExecuteOptions(opts...).coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	if channel == loop_coordinator.LegacyName {
		return r.invokeLegacyPlanExecuteOnly(done, ctx, opts...)
	}
	return r.invokeNativeCoordinator(done, ctx, false, opts...)
}

func (r *ReAct) invokeExecutePlan(done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	channel, err := r.coordinatorChannel(newInvokePlanAndExecuteOptions(opts...).coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	if channel == loop_coordinator.LegacyName {
		return r.invokeLegacyExecutePlan(done, ctx, opts...)
	}
	if newInvokePlanAndExecuteOptions(opts...).executePlanInput == nil {
		close(done)
		return fmt.Errorf("execute plan input is nil")
	}
	return r.invokeNativeCoordinator(done, ctx, false, opts...)
}

func init() {
	// Expose the legacy channel through the same focus metadata API. Normal
	// entry dispatches before loop construction; this adapter also supports a
	// caller that directly uses the loop registry, without an extra model call.
	_ = reactloops.RegisterLoopFactory(loop_coordinator.LegacyName, func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
		owner, ok := r.(*ReAct)
		if !ok {
			return nil, fmt.Errorf("coordinator_legacy requires a ReAct session")
		}
		opts = append(opts, reactloops.WithInitTask(func(_ *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			if err := owner.invokeLegacyPlanAndExecute(make(chan struct{}), task.GetContext(), WithInvokePlanAndExecuteTask(task), WithInvokePlanAndExecutePlanPayload(task.GetUserInput())); err != nil {
				op.Failed(err)
			} else {
				op.Done()
			}
		}))
		return reactloops.NewReActLoop(loop_coordinator.LegacyName, r, opts...)
	}, reactloops.WithVerboseName("Coordinator Legacy"), reactloops.WithVerboseNameZh("任务协调（旧版）"), reactloops.WithLoopDescription("Original PLAN implementation, isolated from the native coordinator runtime."))
}

func configureCoordinatorChannel(cfg *aicommon.Config) {
	if cfg.Focus == loop_coordinator.Name {
		_ = loop_coordinator.WithNativeHelpers()(cfg)
	}
}

func (r *ReAct) invokePlanAndExecute(done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	cfg := newInvokePlanAndExecuteOptions(opts...)
	channel, err := r.coordinatorChannel(cfg.coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	return r.invokeCoordinatorChannel(channel, done, ctx, opts...)
}

func (r *ReAct) invokeCoordinatorChannel(channel string, done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) (err error) {
	if channel == loop_coordinator.LegacyName {
		return r.invokeLegacyPlanAndExecute(done, ctx, opts...)
	}
	return r.invokeNativeCoordinator(done, ctx, false, opts...)
}

func (r *ReAct) invokeNativeCoordinator(done chan struct{}, ctx context.Context, planningOnly bool, opts ...InvokePlanAndExecuteOption) (err error) {
	var doneOnce sync.Once
	ready := func() { doneOnce.Do(func() { close(done) }) }
	defer ready()
	cfg := newInvokePlanAndExecuteOptions(opts...)
	if cfg.task != nil {
		defer func() { cfg.task.CallAsyncDeferCallback(err) }()
	}
	if cfg.forgeName != "" {
		return fmt.Errorf("coordinator business execution requires plan tasks; forge execution belongs to coordinator_legacy")
	}
	task := cfg.task
	if task == nil {
		task = aicommon.NewStatefulTaskBase("coordinator-request", cfg.planPayload, ctx, r.Emitter, true)
	}
	session, err := loop_coordinator.FromRuntime(ctx, r, task, cfg.coordinatorID)
	if err != nil {
		return err
	}
	defer session.Close()
	session.SetRecoveryStartTaskID(cfg.startTaskID)
	if cfg.executePlanInput != nil {
		root, err := session.BuildRootTaskFromPlanData(cfg.executePlanInput.PlanData, cfg.executePlanInput.PlanPayload)
		if err != nil {
			return err
		}
		if err := session.CommitApprovedPlan(root, cfg.executePlanInput.PlanDocument); err != nil {
			return err
		}
	}
	if strings.TrimSpace(cfg.planPayload) != "" && cfg.planPayload != task.GetUserInput() {
		session.Timeline.PushText(session.AcquireId(), "[PLAN_REQUEST]\n%s", cfg.planPayload)
	}
	ready()
	if planningOnly {
		return session.RunPlanOnly()
	}
	return session.Run()
}
