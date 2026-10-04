package aireact

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
