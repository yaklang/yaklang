package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"io"
)

type ReviewSuggestion struct {
	Value             string `json:"value"`
	Suggestion        string `json:"prompt"`
	SuggestionEnglish string `json:"prompt_english"`

	PromptBuilder    func(task *aiTask, rt *runtime)
	ResponseCallback func(reader io.Reader)
	ParamSchema      string `json:"param_schema"`
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
	case "inaccurate":
		t.config.EmitInfo("inaccurate")
	case "end":
		t.config.EmitInfo("end")
	case "adjust_plan":
		t.config.EmitInfo("adjust plan")
	default:
		t.config.EmitError("unknown review suggestion: %s", suggestion)
		return utils.Errorf("unknown review suggestion: %s", suggestion)
	}
}
