package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
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
		if progress.Engine != "" && progress.Engine != coordinator.Name && progress.Engine != "legacy" && progress.Engine != coordinator_legacy.Name {
			return "", fmt.Errorf("unknown stored plan engine %q", progress.Engine)
		}
		hasSnapshot := len(progress.State) > 0 && string(progress.State) != "null"
		if hasSnapshot && (progress.Engine == "legacy" || progress.Engine == coordinator_legacy.Name) {
			return "", fmt.Errorf("stored legacy plan contains a native coordinator snapshot")
		}
		if progress.Engine == coordinator.Name || hasSnapshot {
			return coordinator.Name, nil
		}
		return "", fmt.Errorf("legacy PLAN execution is disabled; create a new coordinator plan")
	}
	return coordinator.Name, nil
}

type nativePlanCoordinatorSession struct {
	r               *ReAct
	session         *coordinator.Session
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

func (r *ReAct) newNativeInputSession(ctx context.Context, query, taskID string) (*coordinator.Session, error) {
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
	return coordinator.FromRuntime(ctx, r, task, "")
}

func (r *ReAct) PublishDetachedPlan(ctx context.Context, input *aicommon.ExecutePlanInput, reactTaskID string) (string, error) {
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
	_, err := r.coordinatorChannel(newInvokePlanAndExecuteOptions(opts...).coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	return r.invokeNativeCoordinator(done, ctx, true, opts...)
}

func (r *ReAct) invokePlanExecuteOnly(done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	_, err := r.coordinatorChannel(newInvokePlanAndExecuteOptions(opts...).coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	return r.invokeNativeCoordinator(done, ctx, false, opts...)
}

func (r *ReAct) invokeExecutePlan(done chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	_, err := r.coordinatorChannel(newInvokePlanAndExecuteOptions(opts...).coordinatorID)
	if err != nil {
		close(done)
		return err
	}
	if newInvokePlanAndExecuteOptions(opts...).executePlanInput == nil {
		close(done)
		return fmt.Errorf("execute plan input is nil")
	}
	return r.invokeNativeCoordinator(done, ctx, false, opts...)
}

func configureCoordinatorChannel(cfg *aicommon.Config) {
	if cfg.Focus == coordinator.Name {
		_ = coordinator.WithNativeHelpers()(cfg)
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
	if channel != coordinator.Name {
		close(done)
		return fmt.Errorf("legacy PLAN execution is disabled; use coordinator")
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
	session, err := coordinator.FromRuntime(ctx, r, task, cfg.coordinatorID)
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
