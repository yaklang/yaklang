package aid

import (
	"io"
	"slices"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type ReviewSuggestion struct {
	Value             string `json:"value"`
	Suggestion        string `json:"prompt"`
	SuggestionEnglish string `json:"prompt_english"`

	PromptBuilder    func(task *aiTask, rt *runtime) `json:"-"`
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
		Value:             "deeply_think",
		Suggestion:        "思考不够深入，根据当前上下文，为当前任务拆分更多子任务",
		SuggestionEnglish: "Not deep enough, split more sub-tasks for the current task according to the current context",
	},
	{
		Value:             "inaccurate",
		Suggestion:        "回答不够精准，存在未使用工具导致幻觉，或者工具参数不合适",
		SuggestionEnglish: "The answer is not accurate enough, there is an illusion caused by not using the tool, or the tool parameters are not appropriate",
	},
	{
		Value:             "continue",
		Suggestion:        "继续执行任务",
		SuggestionEnglish: "Continue to execute the task",
	},
	{
		Value:             "end",
		Suggestion:        "到此结束，后续不要做新任务了",
		SuggestionEnglish: "End here, no new tasks should be done later",
	},
	{
		Value:             "adjust_plan",
		Suggestion:        "任务需要调整，用户会输入更新后任务",
		SuggestionEnglish: "The task needs to be adjusted, and the user will enter the updated task",
	},
}

func (t *aiTask) handleReviewResult(ctx *taskContext, param aitool.InvokeParams) error {
	// 1. 获取审查建议
	suggestion := param.GetString("suggestion")
	if suggestion == "" {
		return utils.Error("suggestion is empty")
	}

	// 2. 根据审查建议处理
	switch suggestion {
	case "deeply_think":
		//TODO: 深度思考
		t.config.EmitInfo("deeply think")
	case "inaccurate":
		t.config.EmitInfo("inaccurate")
		t.rerun = true
	case "continue":
		t.config.EmitInfo("continue")
	case "end":
		t.config.EmitInfo("end")

		parentTask := t.ParentTask
		index := -1
		for i, subtask := range parentTask.Subtasks {
			if subtask.Name == t.Name {
				index = i
				break
			}
		}
		if index == -1 {
			t.config.EmitError("current task not found in parent task")
			return utils.Error("current task not found in parent task")
		}
		parentTask.Subtasks = parentTask.Subtasks[:index+1]
	case "adjust_plan":
		plan := param.GetString("plan")
		if plan == "" {
			t.config.EmitError("plan is empty")
			return utils.Error("plan is empty")
		}
		t.config.EmitInfo("adjust plan")
		planPrompt, err := t.generateDynamicPlanPrompt(ctx, plan)
		if err != nil {
			t.config.EmitError("error generating dynamic plan prompt: %v", err)
			return utils.Errorf("error generating dynamic plan prompt: %v", err)
		}

		// 调用 AI 生成新的任务计划
		request := NewAIRequest(planPrompt, WithAIRequest_TaskContext(ctx))
		response, err := t.callAI(request)
		if err != nil {
			t.config.EmitError("error calling AI: %v", err)
			return utils.Errorf("error calling AI: %v", err)
		}
		// 读取 AI 的响应
		responseReader := response.GetOutputStreamReader("dynamic-plan", false, t.config)
		taskResponse, err := io.ReadAll(responseReader)
		if err != nil {
			t.config.EmitError("error reading AI response: %v", err)
			return utils.Errorf("error reading AI response: %v", err)
		}
		taskResponseJson := gjson.ParseBytes(taskResponse)
		action := taskResponseJson.Get("@action").String()
		if action != "re-plan" {
			t.config.EmitError("invalid action: %s", action)
			return utils.Errorf("invalid action: %s", action)
		}
		// 解析 AI 的响应
		parentTask := t.ParentTask
		index := -1
		for i, subtask := range parentTask.Subtasks {
			if subtask.Name == t.Name {
				index = i
				break
			}
		}
		if index == -1 {
			t.config.EmitError("current task not found in parent task")
			return utils.Error("current task not found in parent task")
		}
		// 保留之前的任务, 删除后续任务
		parentTask.Subtasks = parentTask.Subtasks[:index+1]
		plans := taskResponseJson.Get("next_plans").Array()
		if len(plans) == 0 {
			t.config.EmitError("no new dynamic plans found")
			return utils.Error("no new dynamic plans found")
		}
		parentTask.Subtasks = slices.Grow(parentTask.Subtasks, len(parentTask.Subtasks)+len(plans))

		// 添加新的任务
		for _, plan := range plans {
			name, goal := plan.Get("name").String(), plan.Get("goal").String()
			if name == "" || goal == "" {
				t.config.EmitError("invalid plan: %s", plan.String())
				return utils.Errorf("invalid plan: %s", plan.String())
			}
			parentTask.Subtasks = append(parentTask.Subtasks, &aiTask{
				config:     t.config,
				Name:       name,
				Goal:       goal,
				ParentTask: parentTask,
			})

			t.config.EmitInfo("new dynamic plan: %s", name)
		}
	default:
		t.config.EmitError("unknown review suggestion: %s", suggestion)
		return utils.Errorf("unknown review suggestion: %s", suggestion)
	}
	return nil
}
