package aid

import (
	"io"
	"slices"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type ReviewSuggestion struct {
	Value             string `json:"value"`
	Suggestion        string `json:"prompt"`
	SuggestionEnglish string `json:"prompt_english"`
	AllowExtraPrompt  bool   `json:"allow_extra_prompt"`

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
		AllowExtraPrompt:  true,
	},
	{
		Value:             "inaccurate",
		Suggestion:        "回答不够精准，存在未使用工具导致幻觉，或者工具参数不合适",
		SuggestionEnglish: "The answer is not accurate enough, there is an illusion caused by not using the tool, or the tool parameters are not appropriate",
		AllowExtraPrompt:  true,
	},
	{
		Value:             "continue",
		Suggestion:        "继续执行任务",
		SuggestionEnglish: "Continue to execute the task",
	},
	{
		Value:             "adjust_plan",
		Suggestion:        "任务需要调整，用户会输入更新后任务",
		SuggestionEnglish: "The task needs to be adjusted, and the user will enter the updated task",
		AllowExtraPrompt:  true,
	},
}

func (t *aiTask) handleReviewResult(param aitool.InvokeParams) error {
	// 1. 获取审查建议
	suggestion := param.GetString("suggestion")
	if suggestion == "" {
		return utils.Error("suggestion is empty")
	}

	// 2. 根据审查建议处理
	switch suggestion {
	case "deeply_think":
		t.config.EmitInfo("deeply think")
		planReq, err := createPlanRequest(t.Goal)
		if err != nil {
			t.config.EmitError("create planRequest failed: %v", err)
			return utils.Errorf("coordinator: create planRequest failed: %v", err)
		}
		planReq.config = t.config
		t.config.EmitInfo("start to invoke plan request")
		rsp, err := planReq.Invoke()
		if err != nil {
			t.config.EmitError("invoke planRequest failed: %v", err)
			return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
		}

		if rsp.RootTask == nil {
			t.config.EmitError("root aiTask is nil, plan failed")
			return utils.Errorf("coordinator: root aiTask is nil")
		}
		t.Subtasks = rsp.RootTask.Subtasks
		r := &runtime{
			config: t.config,
			Stack:  utils.NewStack[*aiTask](),
		}
		r.Invoke(t)
	case "inaccurate":
		t.config.EmitInfo("inaccurate")
		return t.executeTask()
	case "continue":
		t.config.EmitInfo("continue")
		return nil
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
		planPrompt, err := t.generateDynamicPlanPrompt(plan)
		if err != nil {
			t.config.EmitError("error generating dynamic plan prompt: %v", err)
			return utils.Errorf("error generating dynamic plan prompt: %v", err)
		}

		err = t.config.callAiTransaction(
			planPrompt,
			t.callAI,
			func(response *AIResponse) error {
				// 读取 AI 的响应
				responseReader := response.GetOutputStreamReader("dynamic-plan", false, t.config)
				taskResponse, err := io.ReadAll(responseReader)
				if err != nil {
					t.config.EmitError("error reading AI response: %v", err)
					return utils.Errorf("error reading AI response: %v", err)
				}
				nextPlanTask, err := ExtractNextPlanTaskFromRawResponse(t.config, string(taskResponse))
				if err != nil {
					t.config.EmitError("error extracting task from raw response: %v", err)
					return utils.Errorf("error extracting task from raw response: %v", err)
				}

				if len(nextPlanTask) <= 0 {
					t.config.EmitError("any task not found in next plan")
					return utils.Errorf("any task not found in next plan task, re-do-plan")
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
				parentTask.Subtasks = slices.Grow(parentTask.Subtasks, len(parentTask.Subtasks)+len(nextPlanTask))

				// 添加新的任务
				for _, subTask := range nextPlanTask {
					subTask.config = t.config
					subTask.ParentTask = parentTask
					parentTask.Subtasks = append(parentTask.Subtasks, subTask)
					subTask.config.EmitInfo("new dynamic plan: %s", subTask.Name)
				}
				return nil
			},
		)
		if err != nil {
			t.config.EmitError("error calling AI transaction: %v", err)
			return utils.Errorf("error calling AI transaction: %v", err)
		}
	default:
		t.config.EmitError("unknown review suggestion: %s", suggestion)
		return utils.Errorf("unknown review suggestion: %s", suggestion)
	}
	return nil
}
