package aireact

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const recoveryTaskIDPrefix = "react-recovery-"

type invokePlanAndExecuteOptions struct {
	task             aicommon.AIStatefulTask
	planPayload      string
	forgeName        string
	forgeParams      any
	coordinatorID    string
	startTaskID      string
	executePlanInput *aicommon.ExecutePlanInput
}

type InvokePlanAndExecuteOption func(*invokePlanAndExecuteOptions)

func WithInvokePlanAndExecuteTask(task aicommon.AIStatefulTask) InvokePlanAndExecuteOption {
	return func(cfg *invokePlanAndExecuteOptions) {
		cfg.task = task
	}
}

func WithInvokePlanAndExecutePlanPayload(planPayload string) InvokePlanAndExecuteOption {
	return func(cfg *invokePlanAndExecuteOptions) {
		cfg.planPayload = planPayload
	}
}

func WithInvokePlanAndExecuteForge(name string, params any) InvokePlanAndExecuteOption {
	return func(cfg *invokePlanAndExecuteOptions) {
		cfg.forgeName = name
		cfg.forgeParams = params
	}
}

func WithInvokePlanAndExecuteCoordinatorID(coordinatorID string) InvokePlanAndExecuteOption {
	return func(cfg *invokePlanAndExecuteOptions) {
		cfg.coordinatorID = coordinatorID
	}
}

func WithInvokePlanAndExecuteStartTaskID(startTaskID string) InvokePlanAndExecuteOption {
	return func(cfg *invokePlanAndExecuteOptions) {
		cfg.startTaskID = startTaskID
	}
}

func WithInvokePlanAndExecuteExecutePlanInput(input *aicommon.ExecutePlanInput) InvokePlanAndExecuteOption {
	return func(cfg *invokePlanAndExecuteOptions) {
		cfg.executePlanInput = input
	}
}

func newInvokePlanAndExecuteOptions(opts ...InvokePlanAndExecuteOption) *invokePlanAndExecuteOptions {
	cfg := &invokePlanAndExecuteOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	return cfg
}

func formatRecoveryTaskID(coordinatorID string) string {
	return recoveryTaskIDPrefix + sanitizeForTaskId(coordinatorID) + uuid.New().String()
}

func newRecoveryPlanExecTask(ctx context.Context, emitter *aicommon.Emitter, coordinatorID string) aicommon.AIStatefulTask {
	return aicommon.NewStatefulTaskBase(
		formatRecoveryTaskID(coordinatorID),
		coordinatorID,
		ctx,
		emitter,
	)
}

func (r *ReAct) RequireAIForgeAndAsyncExecute(
	ctx context.Context, forgeName string,
	onFinished func(error),
) {

	doneOnce := utils.NewOnce()
	done := func(i error) {
		doneOnce.Do(func() {
			if onFinished != nil {
				onFinished(i)
			}
		})
	}

	// 验证 forgeName 不为空
	if forgeName == "" {
		errMsg := "AI Blueprint name is empty, cannot execute; AI 智能应用名称为空，无法执行。请指定正确的应用名称。"
		r.AddToTimeline("[BLUEPRINT_EMPTY_NAME]", errMsg)
		r.Emitter.EmitError(errMsg)
		done(utils.Error(errMsg))
		return
	}

	// 记录尝试调用 Blueprint
	r.AddToTimeline("[BLUEPRINT_INVOKE_START]", fmt.Sprintf("Invoking AI Blueprint: %s", forgeName))

	ins, forgeParams, err := r.invokeBlueprint(forgeName)
	if err != nil {
		// invokeBlueprint 已经记录了详细错误，这里只需要记录最终失败状态
		r.AddToTimeline("[BLUEPRINT_INVOKE_FAILED]", fmt.Sprintf("Failed to invoke '%s': %v", forgeName, err))
		r.Emitter.EmitError(fmt.Sprintf("Failed to invoke AI Blueprint '%s'", forgeName))
		// Merge result into timeline, do not emit result externally
		r.AddToTimeline("[BLUEPRINT_RESULT]", fmt.Sprintf("AI 智能应用 '%s' 调用失败，请检查应用名称和配置是否正确。错误详情：%v", forgeName, err))
		done(fmt.Errorf("failed to invoke ai-blueprint[%v]: %w", forgeName, err))
		return
	}

	// 再次验证返回的实例
	if ins == nil {
		r.AddToTimeline("[BLUEPRINT_NULL_AFTER_INVOKE]", fmt.Sprintf("AI Blueprint '%s' returned nil after invoke", forgeName))
		r.Emitter.EmitError(fmt.Sprintf("AI Blueprint '%s' returned invalid instance", forgeName))
		r.AddToTimeline("[BLUEPRINT_RESULT]", fmt.Sprintf("AI 智能应用 '%s' 执行异常。", forgeName))
		done(utils.Error(fmt.Sprintf("AI Blueprint '%s' returned nil after successful invoke", forgeName)))
		return
	}

	forgeName = ins.ForgeName

	r.AddToTimeline("[BLUEPRINT_INVOKE_SUCCESS]", fmt.Sprintf("AI Blueprint '%s' (%s) ready with params: %v", forgeName, ins.ForgeVerboseName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 256)))

	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")
	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			if finalError != nil {
				r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v finished with FAILURE: %v", utils.ShrinkString(forgeName, 128), finalError))
			} else {
				r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v is finished", utils.ShrinkString(forgeName, 128)))
			}
			r.emitArtifactsSummaryToTimeline()
			done(finalError)
		}()
		finalError = r.invokePlanAndExecute(taskDone, ctx,
			WithInvokePlanAndExecuteTask(r.GetCurrentTask()),
			WithInvokePlanAndExecuteForge(forgeName, forgeParams),
		)
		if finalError != nil {
			log.Errorf("AsyncPlanAndExecute error: %v", finalError)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan/forge: %v is started", utils.ShrinkString(forgeName, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinished func(error)) {
	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("plan: %v is finished", utils.ShrinkString(planPayload, 128)))
			r.emitArtifactsSummaryToTimeline()
			if onFinished != nil {
				onFinished(finalError)
			}
		}()
		finalError = r.invokePlanAndExecute(taskDone, ctx,
			WithInvokePlanAndExecuteTask(r.GetCurrentTask()),
			WithInvokePlanAndExecutePlanPayload(planPayload),
		)
		if finalError != nil {
			log.Errorf("AsyncPlanAndExecute error: %v", finalError)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan: %v is started", utils.ShrinkString(planPayload, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) AsyncRecoverPlanAndExecute(ctx context.Context, coordinatorID string, startTaskID string, onFinished func(error), opts ...InvokePlanAndExecuteOption) {
	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	recoveryTask := newRecoveryPlanExecTask(ctx, r.Emitter, coordinatorID)
	r.AddRuntimeTask(recoveryTask)

	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("plan recovery: %v is finished", utils.ShrinkString(coordinatorID, 128)))
			r.emitArtifactsSummaryToTimeline()
			if onFinished != nil {
				onFinished(finalError)
			}
		}()
		invokeOpts := []InvokePlanAndExecuteOption{
			WithInvokePlanAndExecuteTask(recoveryTask),
			WithInvokePlanAndExecuteCoordinatorID(coordinatorID),
			WithInvokePlanAndExecuteStartTaskID(startTaskID),
		}
		invokeOpts = append(invokeOpts, opts...)
		finalError = r.invokePlanAndExecute(taskDone, recoveryTask.GetContext(), invokeOpts...)
		if finalError != nil {
			log.Errorf("AsyncRecoverPlanAndExecute error: %v", finalError)
			recoveryTask.SetStatus(aicommon.AITaskState_Aborted)
			r.AddToTimeline("error", fmt.Sprintf("recovery task execution failed: %v", finalError))
		} else {
			recoveryTask.SetStatus(aicommon.AITaskState_Completed)
			r.AddToTimeline("success", "recovery task execution succeeded")
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan recovery: %v is started", utils.ShrinkString(coordinatorID, 128)))
		startupBarrier.Done()
	}
}

// RequireAIForgeAndExecute is the synchronous version of RequireAIForgeAndAsyncExecute.
// It invokes the AI Blueprint and blocks until execution completes.
func (r *ReAct) RequireAIForgeAndExecute(ctx context.Context, forgeName string) error {
	// 验证 forgeName 不为空
	if forgeName == "" {
		errMsg := "AI Blueprint name is empty, cannot execute; AI 智能应用名称为空，无法执行。请指定正确的应用名称。"
		r.AddToTimeline("[BLUEPRINT_EMPTY_NAME]", errMsg)
		r.Emitter.EmitError(errMsg)
		return utils.Error(errMsg)
	}

	// 记录尝试调用 Blueprint
	r.AddToTimeline("[BLUEPRINT_INVOKE_START]", fmt.Sprintf("Invoking AI Blueprint: %s", forgeName))

	ins, forgeParams, err := r.invokeBlueprint(forgeName)
	if err != nil {
		r.AddToTimeline("[BLUEPRINT_INVOKE_FAILED]", fmt.Sprintf("Failed to invoke '%s': %v", forgeName, err))
		r.Emitter.EmitError(fmt.Sprintf("Failed to invoke AI Blueprint '%s'", forgeName))
		r.AddToTimeline("[BLUEPRINT_RESULT]", fmt.Sprintf("AI 智能应用 '%s' 调用失败，请检查应用名称和配置是否正确。错误详情：%v", forgeName, err))
		return fmt.Errorf("failed to invoke ai-blueprint[%v]: %w", forgeName, err)
	}

	// 再次验证返回的实例
	if ins == nil {
		r.AddToTimeline("[BLUEPRINT_NULL_AFTER_INVOKE]", fmt.Sprintf("AI Blueprint '%s' returned nil after invoke", forgeName))
		r.Emitter.EmitError(fmt.Sprintf("AI Blueprint '%s' returned invalid instance", forgeName))
		r.AddToTimeline("[BLUEPRINT_RESULT]", fmt.Sprintf("AI 智能应用 '%s' 执行异常。", forgeName))
		return utils.Error(fmt.Sprintf("AI Blueprint '%s' returned nil after successful invoke", forgeName))
	}

	forgeName = ins.ForgeName

	r.AddToTimeline("[BLUEPRINT_INVOKE_SUCCESS]", fmt.Sprintf("AI Blueprint '%s' (%s) ready with params: %v", forgeName, ins.ForgeVerboseName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 256)))

	taskDone := make(chan struct{})
	finalErr := r.invokePlanAndExecute(taskDone, ctx,
		WithInvokePlanAndExecuteTask(r.GetCurrentTask()),
		WithInvokePlanAndExecuteForge(forgeName, forgeParams),
	)
	if finalErr != nil {
		r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v finished with FAILURE: %v", utils.ShrinkString(forgeName, 128), finalErr))
	} else {
		r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v is finished", utils.ShrinkString(forgeName, 128)))
	}
	r.emitArtifactsSummaryToTimeline()
	return finalErr
}

// PlanAndExecute is the synchronous version of AsyncPlanAndExecute.
// It executes the plan and blocks until completion.
func (r *ReAct) PlanAndExecute(ctx context.Context, planPayload string) error {
	taskDone := make(chan struct{})
	finalErr := r.invokePlanAndExecute(taskDone, ctx,
		WithInvokePlanAndExecuteTask(r.GetCurrentTask()),
		WithInvokePlanAndExecutePlanPayload(planPayload),
	)
	if finalErr != nil {
		log.Errorf("PlanAndExecute error: %v", finalErr)
	}
	r.AddToTimeline("plan_executeion", fmt.Sprintf("plan: %v is finished", utils.ShrinkString(planPayload, 128)))
	r.emitArtifactsSummaryToTimeline()
	return finalErr
}

// executeRecovery runs invokePlanAndExecute with the recovery-specific
// parameters stored in RecoveryTaskData.  It is called by processRecoveryTask
// after the recovery task is dequeued by the QueueProcessor.
func (r *ReAct) executeRecovery(task aicommon.AIStatefulTask, data *aicommon.RecoveryTaskData) error {
	invokeOpts := []InvokePlanAndExecuteOption{
		WithInvokePlanAndExecuteTask(task),
		WithInvokePlanAndExecuteCoordinatorID(data.CoordinatorID),
		WithInvokePlanAndExecuteStartTaskID(data.StartTaskID),
	}
	if data.ExecutePlanInput != nil {
		invokeOpts = append(invokeOpts,
			WithInvokePlanAndExecutePlanPayload(data.ExecutePlanInput.PlanPayload))
		invokeOpts = append(invokeOpts,
			WithInvokePlanAndExecuteExecutePlanInput(data.ExecutePlanInput))
	}

	taskDone := make(chan struct{})
	return r.invokePlanAndExecute(taskDone, task.GetContext(), invokeOpts...)
}

// RecoverPlanAndExecute is the synchronous version of AsyncRecoverPlanAndExecute.
// It recovers and executes a plan, blocking until completion.
func (r *ReAct) RecoverPlanAndExecute(ctx context.Context, coordinatorID string, startTaskID string, input *aicommon.ExecutePlanInput) error {
	recoveryTask := newRecoveryPlanExecTask(ctx, r.Emitter, coordinatorID)
	r.AddRuntimeTask(recoveryTask)

	invokeOpts := []InvokePlanAndExecuteOption{
		WithInvokePlanAndExecuteTask(recoveryTask),
		WithInvokePlanAndExecuteCoordinatorID(coordinatorID),
		WithInvokePlanAndExecuteStartTaskID(startTaskID),
	}
	if input != nil {
		invokeOpts = append(invokeOpts, WithInvokePlanAndExecutePlanPayload(input.PlanPayload))
		invokeOpts = append(invokeOpts, WithInvokePlanAndExecuteExecutePlanInput(input))
	}

	taskDone := make(chan struct{})
	finalErr := r.invokePlanAndExecute(taskDone, recoveryTask.GetContext(), invokeOpts...)
	if finalErr != nil {
		log.Errorf("RecoverPlanAndExecute error: %v", finalErr)
		recoveryTask.SetStatus(aicommon.AITaskState_Aborted)
		r.AddToTimeline("error", fmt.Sprintf("recovery task execution failed: %v", finalErr))
	} else {
		recoveryTask.SetStatus(aicommon.AITaskState_Completed)
		r.AddToTimeline("success", "recovery task execution succeeded")
	}
	r.AddToTimeline("plan_executeion", fmt.Sprintf("plan recovery: %v is finished", utils.ShrinkString(coordinatorID, 128)))
	r.emitArtifactsSummaryToTimeline()
	return finalErr
}
