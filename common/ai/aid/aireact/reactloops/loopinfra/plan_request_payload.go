package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

func extractPlanRequestPayload(action *aicommon.Action) string {
	if action == nil {
		return ""
	}
	payload := action.GetString("plan_request_payload")
	if payload == "" {
		payload = action.GetInvokeParams("next_action").GetString("plan_request_payload")
	}
	return payload
}

func verifyPlanRequestPayload(loop *reactloops.ReActLoop, action *aicommon.Action, actionName string) error {
	invoker := loop.GetInvoker()
	if reactInvoker, ok := invoker.(interface {
		GetCurrentPlanExecutionTask() aicommon.AIStatefulTask
	}); ok {
		if reactInvoker.GetCurrentPlanExecutionTask() != nil {
			return utils.Errorf("another plan or plan-execution task is already running, please wait for it to complete or use directly_answer to provide the result")
		}
	}

	payload := extractPlanRequestPayload(action)
	if payload == "" {
		return utils.Errorf("%s action must have 'plan_request_payload' field", actionName)
	}
	loop.Set("plan_request_payload", payload)
	return nil
}
