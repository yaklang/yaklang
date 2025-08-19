package aireact

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) invokePlanAndExecute(planPayload string) error {
	// create config with timeline
	// generate config

	r.EmitAction(fmt.Sprintf("Plan request: %s", planPayload))

	planCtx, cancel := context.WithCancel(r.config.GetContext())
	defer cancel()

	cod, err := aid.NewCoordinatorContext(
		planCtx,
		planPayload,
		aid.WithMemory(r.config.memory),
		aid.WithAICallback(r.config.aiCallback),
		aid.WithAllowPlanUserInteract(true),
		aid.WithAgreeManual(),
		aid.WithEventHandler(func(e *schema.AiOutputEvent) {
			emitErr := r.config.Emit(e)
			if emitErr != nil {
				log.Errorf("Failed to emit event: %v", emitErr)
			}
		}),
		aid.WithDisallowRequireForUserPrompt(),
	)
	if err != nil {
		r.config.finished = true
		log.Errorf("Failed to create coordinator for plan execution: %v", err)
		return utils.Errorf("failed to create coordinator for plan execution: %v", err)
	}
	if err := cod.Run(); err != nil {
		r.config.finished = true
		log.Errorf("Plan execution failed: %v", err)
		return utils.Errorf("plan execution failed: %v", err)
	}
	// Emit the final result from the coordinator
	r.config.finished = true

	return nil
}
