package aireact

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) RequireAIForgeAndAsyncExecute(
	ctx context.Context, forgeName string,
	onFinished func(),
) error {
	ins, forgeParams, err := r.invokeBlueprint(forgeName)
	if err != nil {
		r.AddToTimeline("plan_error", fmt.Sprintf("failed to invoke ai-blueprint[%v]: %v", forgeName, err))
		return err
	}
	forgeName = ins.ForgeName

	r.AddToTimeline("ai-blueprint", fmt.Sprintf("invoke ai-blueprint[%v] success, params: %v", forgeName, utils.ShrinkString(utils.InterfaceToString(forgeParams), 256)))

	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")
	taskDone := make(chan struct{})
	go func() {
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("plan/forge: %v is finished", utils.ShrinkString(forgeName, 128)))
			if onFinished != nil {
				onFinished()
			}
		}()
		err := r.invokePlanAndExecute(taskDone, ctx, "", forgeName, forgeParams)
		if err != nil {
			log.Errorf("AsyncPlanAndExecute error: %v", err)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan/forge: %v is started", utils.ShrinkString(forgeName, 128)))
		startupBarrier.Done()
		return nil
	}
}

func (r *ReAct) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinished func()) error {
	cb := utils.NewCondBarrierContext(ctx)
	startupBarrier := cb.CreateBarrier("startup")

	taskDone := make(chan struct{})
	go func() {
		defer func() {
			if err := cb.Wait("startup"); err != nil {
				log.Warnf("start up failed: %v", err)
			}
			r.AddToTimeline("plan_executeion", fmt.Sprintf("plan: %v is finished", utils.ShrinkString(planPayload, 128)))
			if onFinished != nil {
				onFinished()
			}
		}()
		err := r.invokePlanAndExecute(taskDone, ctx, planPayload, "", nil)
		if err != nil {
			log.Errorf("AsyncPlanAndExecute error: %v", err)
		}
	}()
	select {
	case <-taskDone:
		r.AddToTimeline("plan_execute", fmt.Sprintf("plan: %v is started", utils.ShrinkString(planPayload, 128)))
		startupBarrier.Done()
		return nil
	}
}

func (r *ReAct) invokePlanAndExecute(doneChannel chan struct{}, ctx context.Context, planPayload string, forgeName string, forgeParams any) error {
	doneOnce := new(sync.Once)
	done := func() {
		doneOnce.Do(func() {
			close(doneChannel)
		})
	}
	defer func() {
		done()
		if err := recover(); err != nil {
			log.Errorf("invokePlanAndExecute panic: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	// create config with timeline
	// generate config
	uid := uuid.New().String()
	params := map[string]any{
		"re-act_id":      r.config.id,
		"re-act_task":    r.GetCurrentTask().GetId(),
		"coordinator_id": uid,
	}
	r.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, r.config.id, params)
	defer func() {
		r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.id, params)
	}()
	r.EmitAction(fmt.Sprintf("Plan request: %s", planPayload))

	if ctx == nil {
		ctx = r.config.ctx
	}
	planCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// if hijackPlanRequest is set, use it to handle the plan request
	// this is useful for testing/mocking and advanced usage
	if r.config.hijackPlanRequest != nil {
		r.EmitAction("hijack plan and execute in re-act mode")
		var payload string
		if planPayload == "" {
			payload = utils.InterfaceToString(forgeParams)
		} else {
			payload = planPayload
		}
		log.Infof("hijack plan and execute in re-act mode with payload: %v", utils.ShrinkString(planPayload, 200))
		done()
		return r.config.hijackPlanRequest(planCtx, payload)
	}

	inputChannel := make(chan *aid.InputEvent, 100)
	r.RegisterMirrorOfAIInputEvent(uid, func(event *ypb.AIInputEvent) {
		go func() {
			log.Infof("Received AI input event: %v", event)
			result, err := aid.ConvertAIInputEventToAIDInputEvent(event)
			if err != nil {
				log.Errorf("Failed to convert AI input event to AID input event: %v, data: %v", err, event)
				return
			}
			inputChannel <- result
		}()
	})
	defer func() {
		r.UnregisterMirrorOfAIInputEvent(uid)
	}()

	baseOpts := ConvertReActConfigToAIDConfigOptions(r.config)
	baseOpts = append(baseOpts, aid.WithCoordinatorId(uid),
		aid.WithMemory(r.config.memory),
		aid.WithAICallback(r.config.aiCallback),
		aid.WithAllowPlanUserInteract(true),
		aid.WithAgreeManual(),
		aid.WithEventInputChan(inputChannel),
		aid.WithAgreePolicy(r.config.reviewPolicy),
		aid.WithEventHandler(func(e *schema.AiOutputEvent) {
			e.CoordinatorId = uid
			emitErr := r.config.Emit(e)
			if emitErr != nil {
				log.Errorf("Failed to emit event: %v", emitErr)
			}
		}),
	)

	if forgeName != "" {
		var opts []any = make([]any, len(baseOpts))
		for i, o := range baseOpts {
			opts[i] = o
		}
		done()
		result, err := yak.ExecuteForge(forgeName, forgeParams, opts...)
		if err != nil {
			log.Errorf("Failed to execute forge: %v", err)
			return utils.Errorf("failed to execute forge %s: %v", forgeName, err)
		}
		_ = result
		return nil
	} else {
		cod, err := aid.NewCoordinatorContext(planCtx, planPayload, baseOpts...)
		if err != nil {
			log.Errorf("Failed to create coordinator for plan execution: %v", err)
			return utils.Errorf("failed to create coordinator for plan execution: %v", err)
		}
		done()
		if err := cod.Run(); err != nil {
			log.Errorf("Plan execution failed: %v", err)
			return utils.Errorf("plan execution failed: %v", err)
		}
		return nil
	}
}
