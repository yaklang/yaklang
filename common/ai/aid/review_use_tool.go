package aid

import (
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type ToolUseReviewSuggestion struct {
	Value                   string `json:"value"`
	Suggestion              string `json:"prompt"`
	SuggestionEnglish       string `json:"prompt_english"`
	AllowExtraPrompt        bool   `json:"allow_extra_prompt"`
	AllowParamsModification bool   `json:"allow_params_modification"`

	PromptBuilder    func(task *aiTask, rt *runtime) `json:"-"`
	ResponseCallback func(reader io.Reader)          `json:"-"`
	ParamSchema      string                          `json:"param_schema"`
}

// ToolUseReviewSuggestions 是工具使用审查时的建议(内置一些常见选项)
var ToolUseReviewSuggestions = []*ToolUseReviewSuggestion{
	{
		Value:             "wrong_tool",
		Suggestion:        "使用了错误的工具，需要更换更合适的工具",
		SuggestionEnglish: "Wrong tool used, need to change to a more appropriate tool",
		AllowExtraPrompt:  true,
	},
	{
		Value:                   "wrong_params",
		Suggestion:              "工具参数使用不当，需要调整参数",
		SuggestionEnglish:       "Tool parameters are not used properly, need to adjust parameters",
		AllowExtraPrompt:        true,
		AllowParamsModification: true,
	},
	{
		Value:             "continue",
		Suggestion:        "工具使用正确，继续执行",
		SuggestionEnglish: "Tool usage is correct, continue execution",
	},
}

func (t *aiTask) handleToolUseReview(targetTools *aitool.Tool, param aitool.InvokeParams, userInput aitool.InvokeParams) (*aitool.Tool, aitool.InvokeParams, error) {
	// 1. 获取审查建议
	suggestion := userInput.GetString("suggestion")
	if suggestion == "" {
		return targetTools, param, utils.Error("suggestion is empty")
	}
	//extraPrompt := userInput.GetString("extra_prompt", "prompt")

	// 2. 根据审查建议处理
	switch suggestion {
	case "wrong_tool":
		t.config.EmitInfo("wrong tool used")
		// todo
		return targetTools, param, nil
	case "wrong_params":
		t.config.EmitInfo("wrong parameters used")
		// todo
		return targetTools, param, nil
	case "continue":
		t.config.EmitInfo("tool usage is correct, continue")
		// 继续执行现有任务
		return targetTools, param, nil
	default:
		t.config.EmitError("unknown review suggestion: %s", suggestion)
		return targetTools, param, utils.Errorf("unknown review suggestion: %s", suggestion)
	}
}
