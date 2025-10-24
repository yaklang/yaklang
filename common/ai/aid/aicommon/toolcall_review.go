package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type ToolUseReviewSuggestion struct {
	Value                   string `json:"value"`
	Prompt                  string `json:"prompt"`
	PromptEnglish           string `json:"prompt_english"`
	AllowExtraPrompt        bool   `json:"allow_extra_prompt"`
	AllowParamsModification bool   `json:"allow_params_modification"`
}

// ToolUseReviewSuggestions 是工具使用审查时的建议(内置一些常见选项)
var ToolUseReviewSuggestions = []*ToolUseReviewSuggestion{
	{
		Value:            "wrong_tool",
		Prompt:           "工具选择不当",
		PromptEnglish:    "Wrong tool used, need to change to a more appropriate tool",
		AllowExtraPrompt: true,
	},
	{
		Value:                   "wrong_params",
		Prompt:                  "参数不合理",
		PromptEnglish:           "Tool parameters are not used properly, need to adjust parameters",
		AllowExtraPrompt:        true,
		AllowParamsModification: true,
	},
	{
		Value:                   "direct_answer",
		Prompt:                  "要求AI直接回答",
		PromptEnglish:           "Tool parameters are not used properly, need to adjust parameters",
		AllowExtraPrompt:        true,
		AllowParamsModification: true,
	},
	{
		Value:         "continue",
		Prompt:        "同意工具使用",
		PromptEnglish: "Tool usage is correct, continue execution",
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
		// Check context before processing
		select {
		case <-t.ctx.Done():
			e.EmitError("context cancelled during tool review")
			return targetTool, param, nil, HandleToolUseNext_Default, t.ctx.Err()
		default:
		}

		if t.reviewWrongToolHandler == nil {
			e.EmitError("no review wrong tool handler defined")
			return targetTool, param, nil, HandleToolUseNext_Default, nil
		}
		newTool, directlyAnswer, err := t.reviewWrongToolHandler(
			t.ctx,
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
			userCancelHandler("tool directly answer(recursive call tool failed)")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		return targetTool, param, result, HandleToolUseNext_Override, nil
	case "wrong_params":
		// Check context before processing
		select {
		case <-t.ctx.Done():
			e.EmitError("context cancelled during tool review")
			return targetTool, param, nil, HandleToolUseNext_Default, t.ctx.Err()
		default:
		}

		if t.reviewWrongParamHandler == nil {
			e.EmitError("wrong params suggestion received, but no handler defined")
			return targetTool, param, nil, HandleToolUseNext_Override, nil
		}
		newParam, err := t.reviewWrongParamHandler(t.ctx, targetTool, param, userInput.GetString("extra_prompt"))
		if err != nil {
			e.EmitError("error handling tool review: %v", err)
			userCancelHandler("tool directly answer (err in review-wrong-params)")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}

		result, directlyAnswer, err := t.CallToolWithExistedParams(targetTool, true, newParam)
		if err != nil {
			e.EmitError("error handling tool review: %v", err)
			userCancelHandler("tool directly answer (err in call tool with new params)")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		if directlyAnswer {
			userCancelHandler("tool directly answer (after param re-generation)")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		return targetTool, newParam, result, HandleToolUseNext_Override, nil
	case "direct_answer":
		userCancelHandler("direct answer without tool")
		return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
	default:
		e.EmitError("unknown review suggestion: %s", suggestion)
		return targetTool, param, nil, HandleToolUseNext_Default, utils.Errorf("unknown review suggestion: %s", suggestion)
	}
}
