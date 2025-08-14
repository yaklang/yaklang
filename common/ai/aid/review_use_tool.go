package aid

import (
	"io"
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

	PromptBuilder    func(task *AiTask, rt *runtime) `json:"-"`
	ResponseCallback func(reader io.Reader)          `json:"-"`
	ParamSchema      string                          `json:"param_schema"`
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

func (t *AiTask) handleToolUseReview(
	targetTool *aitool.Tool, param aitool.InvokeParams, userInput aitool.InvokeParams,
	userCancelHandler func(reason any),
) (*aitool.Tool, aitool.InvokeParams, *aitool.ToolResult, HandleToolUseNext, error) {
	// 1. 获取审查建议
	suggestion := userInput.GetString("suggestion")
	if suggestion == "" {
		return targetTool, param, nil, "", utils.Error("suggestion is empty")
	}

	suggestion = strings.ToLower(strings.TrimSpace(suggestion))

	if suggestion == "continue" {
		return targetTool, param, nil, "", nil
	}

	extraPrompt := userInput.GetString("extra_prompt")
	_ = extraPrompt

	// 2. 根据审查建议处理
	switch suggestion {
	case "wrong_tool":
		targetTool, err := t.toolReviewPolicy_wrongTool(targetTool, userInput.GetString("suggestion_tool"), userInput.GetString("suggestion_tool_keyword"))
		if err != nil {
			t.config.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, "", nil
		}
		userCancelHandler("tool reselect")
		result, directlyAnswer, err := t.callTool(targetTool)
		if directlyAnswer {
			userCancelHandler("tool directly answer")
			return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
		}
		if err != nil {
			t.config.EmitError("error handling tool review: %v", err)
			return targetTool, param, nil, "", nil
		}
		return targetTool, param, result, HandleToolUseNext_Override, nil
	case "wrong_params":
		return targetTool, param, nil, "", nil
	case "direct_answer":
		userCancelHandler("direct answer without tool")
		return targetTool, param, nil, HandleToolUseNext_DirectlyAnswer, nil
	default:
		t.config.EmitError("unknown review suggestion: %s", suggestion)
		return targetTool, param, nil, "", utils.Errorf("unknown review suggestion: %s", suggestion)
	}
}

func (t *AiTask) toolReviewPolicy_wrongTool(oldTool *aitool.Tool, suggestionToolName string, suggestionKeyword string) (*aitool.Tool, error) {
	var tools []*aitool.Tool
	if suggestionToolName != "" {
		for _, item := range utils.PrettifyListFromStringSplited(suggestionToolName, ",") {
			toolins, err := t.config.aiToolManager.GetToolByName(item)
			if err != nil || utils.IsNil(toolins) {
				if err != nil {
					t.config.EmitError("error searching tool: %v", err)
				} else {
					t.config.EmitInfo("suggestion tool: %v but not found it.", suggestionToolName)
				}
			}
			tools = append(tools, toolins)
		}
	}

	var err error
	if suggestionKeyword != "" {
		searched, err := t.config.aiToolManager.SearchTools("", suggestionKeyword)
		if err != nil {
			t.config.EmitError("error searching tool: %v", err)
		}
		tools = append(tools, searched...)
	}

	if len(tools) <= 0 {
		return oldTool, utils.Error("tool not found via user prompt")
	}

	prompt, err := t.config.quickBuildPrompt(__prompt_toolReSelect, map[string]any{
		"OldTool":  oldTool,
		"ToolList": tools,
	})
	if err != nil {
		return oldTool, err
	}

	var selecteddTool *aitool.Tool
	transErr := t.config.callAiTransaction(prompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.callAI(request)
	}, func(rsp *AIResponse) error {
		action, err := ExtractActionFromStream(
			rsp.GetOutputStreamReader("call-tools", true, t.config),
			"require-tool", "abandon")
		if err != nil {
			return err
		}
		switch action.ActionType() {
		case "require-tool":
			toolName := action.GetString("tool")
			selecteddTool, err = t.config.aiToolManager.GetToolByName(toolName)
			if err != nil {
				return utils.Errorf("error searching tool: %v", err)
			}
		case "abandon":
		default:
			return utils.Errorf("unknown action type: %s", action.ActionType())
		}
		return nil
	})
	if transErr != nil {
		return oldTool, transErr
	}
	if selecteddTool == nil {
		return oldTool, nil
	}
	return selecteddTool, nil
}

func (t *AiTask) toolReviewPolicy_wrongParam(oldTool *aitool.Tool, suggestionToolName string, suggestionKeyword string) (*aitool.Tool, error) {
	var tools []*aitool.Tool
	if suggestionToolName != "" {
		for _, item := range utils.PrettifyListFromStringSplited(suggestionToolName, ",") {
			toolins, err := t.config.aiToolManager.GetToolByName(item)
			if err != nil || utils.IsNil(toolins) {
				if err != nil {
					t.config.EmitError("error searching tool: %v", err)
				} else {
					t.config.EmitInfo("suggestion tool: %v but not found it.", suggestionToolName)
				}
			}
			tools = append(tools, toolins)
		}
	}

	var err error
	if suggestionKeyword != "" {
		searched, err := t.config.aiToolManager.SearchTools("", suggestionKeyword)
		if err != nil {
			t.config.EmitError("error searching tool: %v", err)
		}
		tools = append(tools, searched...)
	}

	if len(tools) <= 0 {
		return oldTool, utils.Error("tool not found via user prompt")
	}

	prompt, err := t.config.quickBuildPrompt(__prompt_toolReSelect, map[string]any{
		"OldTool":  oldTool,
		"ToolList": tools,
	})
	if err != nil {
		return oldTool, err
	}

	var selecteddTool *aitool.Tool
	transErr := t.config.callAiTransaction(prompt, func(request *AIRequest) (*AIResponse, error) {
		request.SetTaskIndex(t.Index)
		return t.callAI(request)
	}, func(rsp *AIResponse) error {
		action, err := ExtractActionFromStream(
			rsp.GetOutputStreamReader("call-tools", true, t.config),
			"require-tool", "abandon")
		if err != nil {
			return err
		}
		switch action.ActionType() {
		case "require-tool":
			toolName := action.GetString("tool")
			selecteddTool, err = t.config.aiToolManager.GetToolByName(toolName)
			if err != nil {
				return utils.Errorf("error searching tool: %v", err)
			}
		case "abandon":
		default:
			return utils.Errorf("unknown action type: %s", action.ActionType())
		}
		return nil
	})
	if transErr != nil {
		return oldTool, transErr
	}
	if selecteddTool == nil {
		return oldTool, nil
	}
	return selecteddTool, nil
}
