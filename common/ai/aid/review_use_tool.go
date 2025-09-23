package aid

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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

//const (
//	HandleToolUseNext_Override       HandleToolUseNext = "override"
//	HandleToolUseNext_DirectlyAnswer HandleToolUseNext = "directly-answer"
//	HandleToolUseNext_Default        HandleToolUseNext = ""
//)
//
//func (t *AiTask) handleToolUseReview(
//	targetTool *aitool.Tool, param aitool.InvokeParams, userInput aitool.InvokeParams,
//	userCancelHandler func(reason any),
//) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, HandleToolUseNext, error) {
//	// 1. 获取审查建议
//	suggestion := userInput.GetString("suggestion")
//	if suggestion == "" {
//		return targetTool, param, nil, "", utils.Error("suggestion is empty")
//	}
//
//	suggestion = strings.ToLower(strings.TrimSpace(suggestion))
//
//	if suggestion == "continue" {
//		return targetTool, param, nil, "", nil
//	}
//
//	extraPrompt := userInput.GetString("extra_prompt")
//	_ = extraPrompt
//
//	// 2. 根据审查建议处理
//	switch suggestion {
//	case "wrong_tool":
//		targetTool, err := t.toolReviewPolicy_wrongTool(targetTool, userInput.GetString("suggestion_tool"), userInput.GetString("suggestion_tool_keyword"))
//		if err != nil {
//			t.EmitError("error handling tool review: %v", err)
//			return targetTool, param, nil, "", nil
//		}
//		userCancelHandler("tool reselect")
//		result, directlyAnswer, err := t.callTool(targetTool)
//		if directlyAnswer {
//			userCancelHandler("tool directly answer")
//			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
//		}
//		if err != nil {
//			t.EmitError("error handling tool review: %v", err)
//			return targetTool, param, nil, "", nil
//		}
//		return targetTool, param, result, HandleToolUseNext_Override, nil
//	case "wrong_params":
//		return targetTool, param, nil, "", nil
//	case "direct_answer":
//		userCancelHandler("direct answer without tool")
//		return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
//	default:
//		t.EmitError("unknown review suggestion: %s", suggestion)
//		return targetTool, param, nil, "", utils.Errorf("unknown review suggestion: %s", suggestion)
//	}
//}

func (t *AiTask) toolReviewPolicy_wrongTool(oldTool *aitool.Tool, suggestionToolName string, suggestionKeyword string) (*aitool.Tool, bool, error) {
	var tools []*aitool.Tool
	if suggestionToolName != "" {
		for _, item := range utils.PrettifyListFromStringSplited(suggestionToolName, ",") {
			toolins, err := t.aiToolManager.GetToolByName(item)
			if err != nil || utils.IsNil(toolins) {
				if err != nil {
					t.EmitError("error searching tool: %v", err)
				} else {
					t.EmitInfo("suggestion tool: %v but not found it.", suggestionToolName)
				}
			}
			tools = append(tools, toolins)
		}
	}

	var err error
	if suggestionKeyword != "" {
		searched, err := t.aiToolManager.SearchTools("", suggestionKeyword)
		if err != nil {
			t.EmitError("error searching tool: %v", err)
		}
		tools = append(tools, searched...)
	}

	if len(tools) <= 0 {
		tools, _ = t.aiToolManager.GetEnableTools()
	}

	if len(tools) <= 0 {
		return oldTool, true, utils.Error("tool not found via user prompt")
	}

	prompt, err := t.quickBuildPrompt(__prompt_toolReSelect, map[string]any{
		"OldTool":  oldTool,
		"ToolList": tools,
	})
	if err != nil {
		return oldTool, true, err
	}

	var selecteddTool *aitool.Tool
	var directlyAnswer bool
	transErr := t.callAiTransaction(prompt, func(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.CallAI(request)
	}, func(rsp *aicommon.AIResponse) error {
		action, err := aicommon.ExtractActionFromStream(
			rsp.GetOutputStreamReader("call-tools", true, t.GetEmitter()),
			"require-tool", "abandon")
		if err != nil {
			return err
		}
		switch action.ActionType() {
		case "require-tool":
			toolName := action.GetString("tool")
			selecteddTool, err = t.aiToolManager.GetToolByName(toolName)
			if err != nil {
				return utils.Errorf("error searching tool: %v", err)
			}
		case "abandon":
			directlyAnswer = true
			return nil
		default:
			return utils.Errorf("unknown action type: %s", action.ActionType())
		}
		return nil
	})
	if transErr != nil {
		return oldTool, true, transErr
	}
	if selecteddTool == nil {
		return oldTool, directlyAnswer, nil
	}
	return selecteddTool, directlyAnswer, nil
}

func (t *AiTask) toolReviewPolicy_wrongParam(tool *aitool.Tool, oldParam aitool.InvokeParams, suggestion string) (aitool.InvokeParams, error) {

	prompt, err := t.quickBuildPrompt(__prompt_ParamsReGenerate, map[string]any{
		"Tool":      tool,
		"OldParam":  oldParam,
		"UserInput": suggestion,
	})
	if err != nil {
		return oldParam, err
	}

	var invokeParams = aitool.InvokeParams{}
	transErr := t.callAiTransaction(prompt, func(request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.CallAI(request)
	}, func(rsp *aicommon.AIResponse) error {
		action, err := aicommon.ExtractActionFromStream(
			rsp.GetOutputStreamReader("call-tools", true, t.GetEmitter()),
			"call-tool")
		if err != nil {
			return err
		}
		for k, v := range action.GetInvokeParams("params") {
			invokeParams.Set(k, v)
		}
		return nil
	})
	if transErr != nil || len(invokeParams) <= 0 {
		return oldParam, transErr
	}
	return invokeParams, nil
}
