package aireact

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func (r *ReAct) AsyncExecuteCod(ctx context.Context, coordinatorID string, onFinished func(error)) {
	if strings.TrimSpace(coordinatorID) == "" {
		if onFinished != nil {
			onFinished(utils.Error("coordinator id is empty"))
		}
		return
	}

	task := r.GetCurrentTask()
	reactTaskID := ""
	if task != nil {
		reactTaskID = task.GetId()
	}
	eventParams := map[string]any{
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"coordinator_id": coordinatorID,
		"mode":           "execute_cod",
	}

	go func() {
		var finalErr error
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("AsyncExecuteCod panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
			if finalErr != nil {
				r.EmitPlanExecFail(finalErr.Error())
			}
			r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.Id, eventParams)
			if onFinished != nil {
				onFinished(finalErr)
			}
		}()

		if ctx == nil {
			ctx = r.config.Ctx
		}

		execDone := make(chan struct{})
		finalErr = r.invokePlanExecuteOnly(execDone, ctx,
			WithInvokePlanAndExecuteCoordinatorID(coordinatorID),
			WithInvokePlanAndExecuteTask(task),
		)
		<-execDone
	}()
}
