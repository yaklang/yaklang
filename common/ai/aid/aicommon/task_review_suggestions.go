package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"io"
)

var schemaRePlanSuggestion = promptloader.MustLoad("ai/aid/task_review_suggestion.json")

type ReviewSuggestion struct {
	Value            string `json:"value"`
	Prompt           string `json:"prompt"`
	PromptEnglish    string `json:"prompt_english"`
	AllowExtraPrompt bool   `json:"allow_extra_prompt"`

	ResponseCallback func(reader io.Reader) `json:"-"`
	ParamSchema      string                 `json:"param_schema"`
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
		Prompt:           "基于当前任务发现的新信息，后续计划需要调整（支持增删改查 delta 操作）",
		PromptEnglish:    "Based on new findings from the current task, the subsequent plan needs adjustment (supports delta operations: insert/remove/modify/append/replace)",
		AllowExtraPrompt: true,
		ParamSchema:      schemaRePlanSuggestion,
	},
}
