package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type ToolUseReviewSuggestion struct {
	Value                   string `json:"value"`
	Suggestion              string `json:"prompt"`
	SuggestionEnglish       string `json:"prompt_english"`
	AllowExtraPrompt        bool   `json:"allow_extra_prompt"`
	AllowParamsModification bool   `json:"allow_params_modification"`
}

// ToolUseReviewSuggestions 是工具使用审查时的建议(内置一些常见选项)
var ToolUseReviewSuggestions = []*ToolUseReviewSuggestion{
	{
		Value:             "wrong_tool",
		Suggestion:        "工具选择不当",
		SuggestionEnglish: "Wrong tool used, need to change to a more appropriate tool",
		AllowExtraPrompt:  true,
	},
	{
		Value:                   "wrong_params",
		Suggestion:              "参数不合理",
		SuggestionEnglish:       "Tool parameters are not used properly, need to adjust parameters",
		AllowExtraPrompt:        true,
		AllowParamsModification: true,
	},
	{
		Value:                   "direct_answer",
		Suggestion:              "要求AI直接回答",
		SuggestionEnglish:       "Tool parameters are not used properly, need to adjust parameters",
		AllowExtraPrompt:        true,
		AllowParamsModification: true,
	},
	{
		Value:             "continue",
		Suggestion:        "同意工具使用",
		SuggestionEnglish: "Tool usage is correct, continue execution",
	},
}

type HandleToolUseNext string

const (
	HandleToolUseNext_Override       HandleToolUseNext = "override"
	HandleToolUseNext_DirectlyAnswer HandleToolUseNext = "directly-answer"
	HandleToolUseNext_Default        HandleToolUseNext = ""
)

func (t *ToolCaller) review(
	targetTool *aitool.Tool, param aitool.InvokeParams, userInput aitool.InvokeParams,
	userCancelHandler func(reason any),
) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, HandleToolUseNext, error) {
	suggestion := userInput.GetString("suggestion")
	if suggestion == "" {
		return targetTool, param, nil, HandleToolUseNext_Default, nil
	}
	suggestion = strings.ToLower(strings.TrimSpace(suggestion))
	if suggestion == "continue" {
		return targetTool, param, nil, HandleToolUseNext_Default, nil
	}

	extraPrompt := userInput.GetString("extra_prompt")
	_ = extraPrompt
	e := t.emitter
	switch suggestion {
	case "wrong_tool":
		if t.reviewWrongToolHandler == nil {
			e.EmitError("no review wrong tool handler defined")
			return targetTool, param, nil, HandleToolUseNext_Default, nil
		}
		newTool, directlyAnswer, err := t.reviewWrongToolHandler(
			targetTool,
			userInput.GetString("suggestion_tool"),
			userInput.GetString("suggestion_tool_keyword"),
		)
		if err != nil {
			userCancelHandler(fmt.Sprintf("tool directly answer for review-wrong-tool failed: %v", err))
			e.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		if directlyAnswer {
			userCancelHandler("tool directly answer (user 's choice)")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}

		targetTool = newTool
		result, directlyAnswer, err := t.CallTool(newTool)
		if directlyAnswer {
			userCancelHandler("tool directly answer")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		if err != nil {
			e.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, HandleToolUseNext_Default, nil
		}
		return targetTool, param, result, HandleToolUseNext_Override, nil
	case "wrong_params":
		if t.reviewWrongParamHandler == nil {
			e.EmitError("wrong params suggestion received, but no handler defined")
			return targetTool, param, nil, HandleToolUseNext_Override, nil
		}
		newParam, err := t.reviewWrongParamHandler(targetTool, param, userInput.GetString("extra_prompt"))
		if err != nil {
			e.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, HandleToolUseNext_Default, nil
		}
		return targetTool, newParam, nil, HandleToolUseNext_Default, nil
	case "direct_answer":
		userCancelHandler("direct answer without tool")
		return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
	default:
		e.EmitError("unknown review suggestion: %s", suggestion)
		return targetTool, param, nil, HandleToolUseNext_Default, utils.Errorf("unknown review suggestion: %s", suggestion)
	}
}
