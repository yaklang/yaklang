package aicommon

import (
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
		editedParam, hasEditedParam, err := reviewEditedParams(userInput)
		if err != nil {
			return targetTool, param, nil, HandleToolUseNext_Default, err
		}
		if hasEditedParam && !invokeParamsEqual(param, editedParam) {
			return t.reviewWithEditedParams(targetTool, param, editedParam, true, userCancelHandler)
		}
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
			e.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, HandleToolUseNext_Default, err
		}
		if directlyAnswer {
			userCancelHandler("tool directly answer (user 's choice)")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}

		targetTool = newTool
		// Review 换了工具, 原始 reason 已与新工具不符; 重置 reason 状态, 让递归的
		// CallTool -> CallToolWithExistedParams 的统一 reason 处理点重新生成一次.
		t.resetReasonForReview()
		result, directlyAnswer, err := t.CallTool(newTool)
		if directlyAnswer {
			userCancelHandler("tool directly answer")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		if err != nil {
			e.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, HandleToolUseNext_Default, err
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

		newParam, hasEditedParam, err := reviewEditedParams(userInput)
		if err != nil {
			return targetTool, param, nil, HandleToolUseNext_Default, err
		}
		if !hasEditedParam {
			if t.reviewWrongParamHandler == nil {
				e.EmitError("wrong params suggestion received, but no handler defined")
				return targetTool, param, nil, HandleToolUseNext_Override, nil
			}
			newParam, err = t.reviewWrongParamHandler(t.ctx, targetTool, param, userInput.GetString("extra_prompt"))
			if err != nil {
				e.EmitError("error handling tool review: %v", err)
				return targetTool, param, nil, HandleToolUseNext_Default, err
			}
		}
		return t.reviewWithEditedParams(targetTool, param, newParam, hasEditedParam, userCancelHandler)
	case "direct_answer":
		userCancelHandler("direct answer without tool")
		return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
	default:
		e.EmitError("unknown review suggestion: %s", suggestion)
		return targetTool, param, nil, HandleToolUseNext_Default, utils.Errorf("unknown review suggestion: %s", suggestion)
	}
}

// reviewEditedParams reads the established review response shape
// {"suggestion": "...", "params": {...}}. Presence is tracked separately so
// an explicit empty object remains distinguishable from the legacy response that
// omitted params and asked the AI repair handler to regenerate them.
func reviewEditedParams(userInput aitool.InvokeParams) (aitool.InvokeParams, bool, error) {
	raw, ok := userInput["params"]
	if !ok {
		return nil, false, nil
	}
	var edited aitool.InvokeParams
	switch params := raw.(type) {
	case aitool.InvokeParams:
		edited = cloneEndpointParams(params)
	case map[string]any:
		edited = cloneEndpointParams(aitool.InvokeParams(params))
	default:
		return nil, true, utils.Errorf("review edited params must be a JSON object, got %T", raw)
	}
	return edited, true, nil
}

// reviewWithEditedParams treats edited values as a new proposal, not an approval.
// They are schema-validated before any recursion and then enter the ordinary
// CallToolWithExistedParams path, which emits a second review card (or replays its
// checkpoint) before the real callback can run.
func (t *ToolCaller) reviewWithEditedParams(
	targetTool *aitool.Tool,
	originalParam, editedParam aitool.InvokeParams,
	approveUnchangedExplicitEdit bool,
	userCancelHandler func(reason any),
) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, HandleToolUseNext, error) {
	valid, validationErrors := targetTool.ValidateParams(editedParam)
	if !valid {
		return targetTool, originalParam, nil, HandleToolUseNext_Default,
			utils.Errorf("invalid review edited params for tool[%s]: %v", targetTool.Name, validationErrors)
	}
	if approveUnchangedExplicitEdit && invokeParamsEqual(originalParam, editedParam) {
		// An explicit wrong_params response may echo an unchanged form. Treat it as
		// approval after validation; recursively reviewing an identical proposal
		// would otherwise create an endless sequence of duplicate cards. AI-generated
		// repairs keep the legacy behavior and always receive a second review card,
		// even when the repair model happens to return the original values.
		return targetTool, originalParam, nil, HandleToolUseNext_Default, nil
	}

	// Review 改了参数, 原始 reason 与新参数不符; 重置 reason 状态, 让递归的
	// CallToolWithExistedParams 的统一 reason 处理点重新生成一次.
	t.resetReasonForReview()
	result, directlyAnswer, err := t.CallToolWithExistedParams(targetTool, true, editedParam)
	if err != nil {
		t.emitter.EmitError("error handling tool review: %v", err)
		return targetTool, originalParam, nil, HandleToolUseNext_Default, err
	}
	if directlyAnswer {
		userCancelHandler("tool directly answer (after param modification)")
		return targetTool, editedParam, nil, HandleToolUseNext_DirectlyAnswer, nil
	}
	return targetTool, editedParam, result, HandleToolUseNext_Override, nil
}
