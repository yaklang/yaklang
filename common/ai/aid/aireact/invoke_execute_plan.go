package aireact

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	runCoordinatorForExecuteApprovedPlan = func(c *aid.Coordinator) error { return c.RunExecuteApprovedPlan() }
)

// AsyncExecutePlan runs an already-generated plan through Coordinator execution only.
func (r *ReAct) AsyncExecutePlan(ctx context.Context, input *aicommon.ExecutePlanInput, onFinished func(error)) {
	if input == nil {
		if onFinished != nil {
			onFinished(utils.Error("execute plan input is nil"))
		}
		return
	}

	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("execute plan finished: %v", utils.ShrinkString(input.PlanPayload, 128)))
			r.emitArtifactsSummaryToTimeline()
			if onFinished != nil {
				onFinished(finalError)
			}
		}()
		finalError = r.invokeExecutePlan(taskDone, ctx,
			WithInvokePlanAndExecuteTask(r.GetCurrentTask()),
			WithInvokePlanAndExecuteExecutePlanInput(input),
		)
		if finalError != nil {
			log.Errorf("AsyncExecutePlan error: %v", finalError)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("execute plan started: %v", utils.ShrinkString(input.PlanPayload, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) invokeExecutePlan(doneChannel chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) (finalErr error) {
	cfg := newInvokePlanAndExecuteOptions(opts...)
	task := cfg.task
	input := cfg.executePlanInput
	if input == nil {
		return utils.Error("execute plan input is nil")
	}

	doneOnce := new(sync.Once)
	done := func() {
		doneOnce.Do(func() {
			close(doneChannel)
		})
	}
	defer func() {
		done()
		if err := recover(); err != nil {
			log.Errorf("invokeExecutePlan panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	defer func() {
		if task != nil {
			task.CallAsyncDeferCallback(finalErr)
		}
	}()

	uid := cfg.coordinatorID
	if uid == "" {
		uid = uuid.New().String()
	}
	reactTaskID := ""
	if task != nil {
		reactTaskID = task.GetId()
	}
	eventParams := map[string]any{
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"coordinator_id": uid,
		"mode":           "execute_plan",
	}
	r.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, r.config.Id, eventParams)
	defer func() {
		if finalErr != nil {
			r.EmitPlanExecFail(finalErr.Error())
		}
		r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.Id, eventParams)
	}()

	if ctx == nil {
		ctx = r.config.Ctx
	}
	planCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	planPayload := enhancePlanPayloadWithTaskUserInput(input.PlanPayload, task)

	if r.config.HijackPERequest != nil {
		done()
		payload := planPayload
		if payload == "" {
			payload = input.PlanData
		}
		return r.config.HijackPERequest(planCtx, payload)
	}

	inputChannel, unregisterMirror := r.registerPlanExecInputMirror(uid)
	defer unregisterMirror()

	hotpatchChan := r.config.HotPatchBroadcaster.Subscribe()
	defer r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)

	baseOpts := buildPlanExecBaseOptions(r, uid, planCtx, inputChannel, hotpatchChan)

	cod, err := newCoordinatorContextForPlanExec(planCtx, planPayload, baseOpts...)
	if err != nil {
		return utils.Errorf("failed to create coordinator for execute plan: %v", err)
	}

	rootTask, err := cod.BuildRootTaskFromPlanData(input.PlanData, planPayload)
	if err != nil {
		return utils.Errorf("failed to build root task from plan data: %v", err)
	}
	if err := cod.CommitApprovedPlan(rootTask, input.PlanFacts, input.PlanDocument); err != nil {
		return utils.Errorf("failed to commit approved plan: %v", err)
	}

	done()
	if err := runCoordinatorForExecuteApprovedPlan(cod); err != nil {
		return utils.Errorf("execute approved plan failed: %v", err)
	}
	return nil
}
