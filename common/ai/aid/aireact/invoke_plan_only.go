package aireact

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	runCoordinatorForPlanOnly    = func(c *aid.Coordinator) error { return c.RunPlanOnly() }
	runCoordinatorForExecuteOnly = func(c *aid.Coordinator) error { return c.RunExecuteOnly() }
)

// AsyncPlanOnly runs plan loop + user review, then asynchronously executes the approved plan.
func (r *ReAct) AsyncPlanOnly(ctx context.Context, planPayload string, onFinished func(error)) {
	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	taskDone := make(chan struct{})
	go func() {
		var finalError error
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_only", fmt.Sprintf("plan only finished: %v", utils.ShrinkString(planPayload, 128)))
			r.emitArtifactsSummaryToTimeline()
			if onFinished != nil {
				onFinished(finalError)
			}
		}()
		finalError = r.invokePlanOnly(taskDone, ctx,
			WithInvokePlanAndExecuteTask(r.GetCurrentTask()),
			WithInvokePlanAndExecutePlanPayload(planPayload),
		)
		if finalError != nil {
			log.Errorf("AsyncPlanOnly error: %v", finalError)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_only", fmt.Sprintf("plan only started: %v", utils.ShrinkString(planPayload, 128)))
		startupBarrier.Done()
	}
}

func (r *ReAct) invokePlanOnly(doneChannel chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) (finalErr error) {
	cfg := newInvokePlanAndExecuteOptions(opts...)
	task := cfg.task
	planPayload := cfg.planPayload

	doneOnce := new(sync.Once)
	done := func() {
		doneOnce.Do(func() {
			close(doneChannel)
		})
	}
	defer func() {
		done()
		if err := recover(); err != nil {
			log.Errorf("invokePlanOnly panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	defer func() {
		task.CallAsyncDeferCallback(finalErr)
	}()

	uid := uuid.New().String()
	reactTaskID := ""
	if task != nil {
		reactTaskID = task.GetId()
	}
	eventParams := map[string]any{
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"coordinator_id": uid,
		"mode":           "plan_only",
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

	planPayload = enhancePlanPayloadWithTaskUserInput(planPayload, task)

	if r.config.HijackPERequest != nil {
		done()
		return r.config.HijackPERequest(planCtx, planPayload)
	}

	inputChannel, unregisterMirror := r.registerPlanExecInputMirror(uid)
	defer unregisterMirror()

	hotpatchChan := r.config.HotPatchBroadcaster.Subscribe()
	defer r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)

	baseOpts := buildPlanExecBaseOptions(r, uid, planCtx, inputChannel, hotpatchChan)

	cod, err := newCoordinatorContextForPlanExec(planCtx, planPayload, baseOpts...)
	if err != nil {
		return utils.Errorf("failed to create coordinator for plan-only: %v", err)
	}

	done()
	if err := runCoordinatorForPlanOnly(cod); err != nil {
		return utils.Errorf("plan-only phase failed: %v", err)
	}

	r.AddToTimeline("plan_execute", fmt.Sprintf("approved plan executing: coordinator=%s", uid))

	execDone := make(chan struct{})
	execErr := r.invokePlanExecuteOnly(execDone, planCtx,
		WithInvokePlanAndExecuteCoordinatorID(uid),
		WithInvokePlanAndExecuteTask(task),
	)
	<-execDone
	return execErr
}

func enhancePlanPayloadWithTaskUserInput(planPayload string, task aicommon.AIStatefulTask) string {
	if planPayload == "" || task == nil {
		return planPayload
	}
	userOriginalInput := task.GetUserInput()
	if userOriginalInput == "" || strings.Contains(planPayload, userOriginalInput) {
		return planPayload
	}
	nonce := utils.RandStringBytes(4)
	return utils.MustRenderTemplate(`
<|用户原始需求_{{.nonce}}|>
{{ .UserOriginalInput }}
<|用户原始需求_END_{{.nonce}}|>
---
{{ .PlanPayload }}
`,
		map[string]any{
			"nonce":             nonce,
			"UserOriginalInput": userOriginalInput,
			"PlanPayload":       planPayload,
		})
}

func (r *ReAct) registerPlanExecInputMirror(uid string) (*chanx.UnlimitedChan[*ypb.AIInputEvent], func()) {
	inputChannel := chanx.NewUnlimitedChan[*ypb.AIInputEvent](r.config.Ctx, 10)
	r.config.InputEventManager.RegisterMirrorOfAIInputEvent(uid, func(event *ypb.AIInputEvent) {
		go func() {
			switch event.SyncType {
			case SYNC_TYPE_QUEUE_INFO:
				return
			case aicommon.SYNC_TYPE_USER_INTERVENTION, aicommon.SYNC_TYPE_RECOVERY_HISTORY:
				return
			default:
				log.Infof("plan-exec mirror: received AI input event: %v", event)
			}
			inputChannel.SafeFeed(event)
		}()
	})
	return inputChannel, func() {
		r.config.InputEventManager.UnregisterMirrorOfAIInputEvent(uid)
	}
}

func appendApprovedPlanArtifactOptions(baseOpts []aicommon.ConfigOption, input *aicommon.ExecutePlanInput) []aicommon.ConfigOption {
	if input == nil {
		return baseOpts
	}
	if facts := strings.TrimSpace(input.PlanFacts); facts != "" {
		factsCopy := facts
		baseOpts = append(baseOpts, func(c *aicommon.Config) error {
			c.AppendFrozenBlockPartition("plan_facts", "Plan Facts", factsCopy, aicommon.PlanFactsFrozenPartitionOrder)
			return nil
		})
	}
	if document := strings.TrimSpace(input.PlanDocument); document != "" {
		documentCopy := document
		baseOpts = append(baseOpts, func(c *aicommon.Config) error {
			c.AppendFrozenBlockPartition("plan_document", "Plan Document", documentCopy, aicommon.PlanDocumentFrozenPartitionOrder)
			return nil
		})
	}
	return baseOpts
}

func buildPlanExecBaseOptions(
	r *ReAct,
	uid string,
	planCtx context.Context,
	inputChannel *chanx.UnlimitedChan[*ypb.AIInputEvent],
	hotpatchChan *chanx.UnlimitedChan[aicommon.ConfigOption],
) []aicommon.ConfigOption {
	baseOpts := aicommon.ConvertConfigToOptions(r.config)
	baseOpts = append(baseOpts,
		aicommon.WithID(uid),
		aicommon.WithTimeline(r.config.Timeline),
		aicommon.WithInheritTieredAICallback(r.config, false),
		aicommon.WithAllowPlanUserInteract(true),
		aicommon.WithEventInputChanx(inputChannel),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithContext(planCtx),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = uid
			r.config.EventHandler(e)
		}),
	)
	return baseOpts
}

// invokePlanExecuteOnly runs subtask execution for a coordinator that already has plan_ready state.
func (r *ReAct) invokePlanExecuteOnly(doneChannel chan struct{}, ctx context.Context, opts ...InvokePlanAndExecuteOption) error {
	cfg := newInvokePlanAndExecuteOptions(opts...)
	coordinatorID := cfg.coordinatorID
	task := cfg.task

	doneOnce := new(sync.Once)
	done := func() {
		doneOnce.Do(func() {
			close(doneChannel)
		})
	}
	defer done()

	if coordinatorID == "" {
		return utils.Error("coordinator id is empty for plan execute-only")
	}

	uid := coordinatorID
	eventParams := map[string]any{
		"re-act_id":      r.config.Id,
		"coordinator_id": uid,
		"mode":           "execute_only",
	}
	if task != nil {
		eventParams["re-act_task"] = task.GetId()
	}

	if ctx == nil {
		ctx = r.config.Ctx
	}
	planCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	inputChannel, unregisterMirror := r.registerPlanExecInputMirror(uid)
	defer unregisterMirror()

	hotpatchChan := r.config.HotPatchBroadcaster.Subscribe()
	defer r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)

	baseOpts := buildPlanExecBaseOptions(r, uid, planCtx, inputChannel, hotpatchChan)
	if cfg.startTaskIndex != "" {
		baseOpts = append(baseOpts, aid.WithRecoveryStartTaskIndex(cfg.startTaskIndex))
	}

	cod, err := newCoordinatorContextForPlanExec(planCtx, "", baseOpts...)
	if err != nil {
		return utils.Errorf("failed to create coordinator for execute-only: %v", err)
	}

	if err := runCoordinatorForExecuteOnly(cod); err != nil {
		return utils.Errorf("execute-only phase failed: %v", err)
	}
	return nil
}
