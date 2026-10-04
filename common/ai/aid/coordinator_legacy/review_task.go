package coordinator_legacy

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// Shared wire DTOs are retained for callers of the isolated old library.
type ReviewSuggestion = aicommon.ReviewSuggestion

var TaskReviewSuggestions = aicommon.TaskReviewSuggestions

func (t *AiTask) handleReviewResult(param aitool.InvokeParams) error {
	defer t.runtime.updateTaskLink()
	planChanged := false

	// 1. 获取审查建议
	suggestion := param.GetString("suggestion")
	if suggestion == "" {
		return utils.Error("suggestion is empty")
	}

	// 2. 根据审查建议处理
	switch suggestion {
	case "deeply_think":
		t.EmitInfo("deeply think")
		err := t.DeepThink(utils.InterfaceToString(param))
		if err != nil {
			t.EmitError("invoke planRequest failed: %v", err)
			return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
		}
		t.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
			"root_task": t.getCurrentTaskPlan(),
		})
		planChanged = true
	case "inaccurate":
		t.EmitInfo("inaccurate")
		return t.executeTask() // 重新执行
	case "continue":
		t.EmitInfo("continue")
		return nil
	case "end":
		t.EmitInfo("end")

		parentTask := t.ParentTask
		index := -1
		for i, subtask := range parentTask.Subtasks {
			if subtask.Name == t.Name {
				index = i
				break
			}
		}
		if index == -1 {
			t.EmitError("current task not found in parent task")
			return utils.Error("current task not found in parent task")
		}
		parentTask.Subtasks = parentTask.Subtasks[:index+1]
		t.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
			"root_task": t.getCurrentTaskPlan(),
		})
		planChanged = true
	case "adjust_plan":
		deltas := ParseTaskDeltas(param)
		if len(deltas) > 0 {
			t.EmitInfo("adjust plan via task deltas (%d operations)", len(deltas))
			err := t.ApplyTaskDeltas(deltas)
			if err != nil {
				t.EmitError("apply task deltas failed: %v", err)
				return utils.Errorf("coordinator: apply task deltas failed: %v", err)
			}
		} else {
			reason := param.GetString("reason")
			if reason == "" {
				reason = param.GetString("extra_prompt")
			}
			t.EmitInfo("adjust plan via AI re-plan (reason: %s)", reason)
			err := t.AdjustPlan(reason)
			if err != nil {
				t.EmitError("invoke planRequest failed: %v", err)
				return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
			}
		}
		t.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
			"root_task": t.getCurrentTaskPlan(),
		})
		planChanged = true
	default:
		t.EmitError("unknown review suggestion: %s", suggestion)
		return utils.Errorf("unknown review suggestion: %s", suggestion)
	}
	if planChanged {
		t.Coordinator.savePlanAndExecState(Phase_NotCompleted, t)
	}
	return nil
}
