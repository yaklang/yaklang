package aid

import (
	_ "embed"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed jsonschema/plan-review/re-plan-suggestion.json
var schemaRePlanSuggestion string

type ReviewSuggestion struct {
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`

	PromptBuilder    func(task *AiTask, rt *runtime) `json:"-"`
	ResponseCallback func(reader io.Reader)          `json:"-"`
	ParamSchema      string                          `json:"param_schema"`
}

/*
	"思考不够深入，根据当前上下文，为当前任务拆分更多子任务",
	"回答不够精准，存在未使用工具导致幻觉，或者工具参数不合适",
	"到此结束，后续不要做新任务了",
	"任务需要调整，用户会输入更新后任务",
*/

// TaskReviewSuggestions 是任务审查时的建议(内置一些常见选项)
var TaskReviewSuggestions = []*ReviewSuggestion{
	{
		Value:            "deeply_think",
		Prompt:           "思考不够深入，根据当前上下文，为当前任务拆分更多子任务",
		PromptEnglish:    "Not deep enough, split more sub-tasks for the current task according to the current context",
		AllowExtraPrompt: true,
		ParamSchema:      schemaRePlanSuggestion,
	},
	{
		Value:            "inaccurate",
		Prompt:           "回答不够精准，存在未使用工具导致幻觉，或者工具参数不合适",
		PromptEnglish:    "The answer is not accurate enough, there is an illusion caused by not using the tool, or the tool parameters are not appropriate",
		AllowExtraPrompt: true,
	},
	{
		Value:         "continue",
		Prompt:        "继续执行任务",
		PromptEnglish: "Continue to execute the task",
	},
	{
		Value:            "adjust_plan",
		Prompt:           "任务需要调整，用户会输入更新后任务建议",
		PromptEnglish:    "The task needs to be adjusted, and the user will enter the updated task",
		AllowExtraPrompt: true,
		ParamSchema:      schemaRePlanSuggestion,
	},
}

func (t *AiTask) handleReviewResult(param aitool.InvokeParams) error {
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

		return t.aiTaskRuntime.executeSubTask(1, t)
	case "inaccurate":
		t.EmitInfo("inaccurate")
		return t.executeTask()
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
	case "adjust_plan":
		suggestion := param.GetString("suggestion")
		if suggestion == "" {
			t.EmitError("suggestion is empty")
			return utils.Error("suggestion is empty")
		}
		t.EmitInfo("adjust plan")
		err := t.AdjustPlan(utils.InterfaceToString(suggestion))
		if err != nil {
			t.EmitError("invoke planRequest failed: %v", err)
			return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
		}
		t.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
			"root_task": t.getCurrentTaskPlan(),
		})
	default:
		t.EmitError("unknown review suggestion: %s", suggestion)
		return utils.Errorf("unknown review suggestion: %s", suggestion)
	}
	return nil
}
