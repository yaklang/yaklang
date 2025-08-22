package aireact

import (
	"context"
	"fmt"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) invokePlanAndExecute(planPayload string) error {
	// create config with timeline
	// generate config

	r.EmitAction(fmt.Sprintf("Plan request: %s", planPayload))

	planCtx, cancel := context.WithCancel(r.config.GetContext())
	defer cancel()

	inputChannel := make(chan *aid.InputEvent, 100)
	uid := ksuid.New().String()
	r.RegisterMirrorOfAIInputEvent(uid, func(event *ypb.AIInputEvent) {
		go func() {
			log.Infof("Received AI input event: %v", event)
			result, err := aid.ConvertAIInputEventToAIDInputEvent(event)
			if err != nil {
				log.Errorf("Failed to convert AI input event to AID input event: %v", err)
				return
			}
			inputChannel <- result
		}()
	})
	defer func() {
		r.UnregisterMirrorOfAIInputEvent(uid)
	}()

	cod, err := aid.NewCoordinatorContext(
		planCtx,
		planPayload,
		aid.WithMemory(r.config.memory),
		aid.WithAICallback(r.config.aiCallback),
		aid.WithAllowPlanUserInteract(true),
		aid.WithAgreeManual(),
		aid.WithEventInputChan(inputChannel),
		aid.WithEventHandler(func(e *schema.AiOutputEvent) {
			emitErr := r.config.Emit(e)
			if emitErr != nil {
				log.Errorf("Failed to emit event: %v", emitErr)
			}
		}),
		aid.WithDisallowRequireForUserPrompt(),
	)
	if err != nil {
		r.finished = true
		log.Errorf("Failed to create coordinator for plan execution: %v", err)
		return utils.Errorf("failed to create coordinator for plan execution: %v", err)
	}
	if err := cod.Run(); err != nil {
		r.finished = true
		log.Errorf("Plan execution failed: %v", err)
		return utils.Errorf("plan execution failed: %v", err)
	}
	// Emit the final result from the coordinator
	r.finished = true

	return nil
}
