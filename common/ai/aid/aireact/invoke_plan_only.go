package aireact

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
