package reactloops

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type LoopActionVerifierFunc func(loop *ReActLoop, action *aicommon.Action) error
type LoopActionHandlerFunc func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator)

type LoopAction struct {
	ActionType     string `json:"type"`
	Description    string `json:"description"`
	Options        []aitool.ToolOption
	ActionVerifier LoopActionVerifierFunc
	ActionHandler  LoopActionHandlerFunc
}

var loopAction_RequireTool = &LoopAction{
	ActionType:  "require_tool",
	Description: "Require tool call",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF type is 'require_tool'. Provide the exact name of the tool you need to use (e.g., 'check-yaklang-syntax', 'yak-document'). Another system will handle the parameter generation based on this name. Do NOT include tool arguments here."),
		),
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
		payload := action.GetString("tool_require_payload")
		if payload == "" {
			return utils.Error("tool_require_payload is required for ActionRequireTool but empty")
		}
		return nil
	},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		toolPayload := action.GetString("tool_require_payload")
		if toolPayload == "" {
			operator.Feedback(utils.Error("tool_require_payload is required for ActionRequireTool but empty"))
			return
		}
		invoker := loop.invoker
		result, directly, err := invoker.ExecuteToolRequiredAndCall(toolPayload)
		if err != nil {
			operator.Fail(utils.Error("ExecuteToolRequiredAndCall fail"))
			return
		}
		if directly {
			answer, err := invoker.DirectlyAnswer("在上一次工具调用中，用户中断了工具执行，要求直接回答一些问题", nil)
			if err != nil {
				operator.Fail(utils.Error("DirectlyAnswer fail, reason: " + err.Error()))
				return
			}
			invoker.AddToTimeline("directly-answer", answer)
			operator.Continue()
			return
		}

		if result == nil {
			msg := fmt.Sprintf("ExecuteToolRequiredAndCall[%v] returned nil result", toolPayload)
			invoker.AddToTimeline("error", msg)
			operator.Continue()
			return
		}

		if result.Error != "" {
			invoker.AddToTimeline("call["+toolPayload+"] error", result.Error)
		}
	},
}

var loopAction_AskForClarification = &LoopAction{
	ActionType:  "ask_for_clarification",
	Description: "Ask for clarification",
	Options: []aitool.ToolOption{
		aitool.WithStructParam(
			"ask_for_clarification_payload",
			[]aitool.PropertyOption{
				aitool.WithParam_Description("Use this action when user's intent is ambiguous or incomplete."),
			},
			aitool.WithStringParam("question", aitool.WithParam_Required(true), aitool.WithParam_Description("A clear, concise question to ask the user for more information. This should help clarify their intent or provide necessary details.")),
			aitool.WithStringArrayParam(
				"options",
				aitool.WithParam_Description(
					`Optional additional context that may help the user understand what information is needed. This can include examples or explanations of why the clarification is necessary.`,
				),
			),
		),
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
		result := action.GetInvokeParams("ask_for_clarification_payload")
		if result.GetString("question") == "" {
			return utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'")
		}
		return nil
	},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		result := action.GetInvokeParams("ask_for_clarification_payload")
		if result.GetString("question") == "" {
			operator.Feedback(utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'"))
			return
		}
		question := result.GetString("question")
		options := result.GetStringSlice("options")

		invoker := loop.invoker
		suggestion := invoker.AskForClarification(question, options)
		if suggestion == "" {
			suggestion = "user did not provide a valid suggestion, using default 'continue' action"
		}
	},
}

var loopAction_Finish = &LoopAction{
	ActionType:  "finish",
	Description: "Finish the task, MUST fill the 'human_readable_thought' field",
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		loop.invoker.AddToTimeline("finish", "AI decided mark the current Task is finished")
		operator.Exit()
	},
}

func buildSchema(actions ...*LoopAction) string {
	var actionNames []string
	for _, action := range actions {
		actionNames = append(actionNames, action.ActionType)
	}
	var opts []any = []any{
		aitool.WithStringParam(
			"@action",
			aitool.WithParam_Description("required '@action' field to identify the action type"),
			aitool.WithParam_EnumString(actionNames...),
			aitool.WithParam_Required(true),
		),
		aitool.WithStringParam(
			"human_readable_thought",
			aitool.WithParam_Description("Provide a brief, user-friendly status message here, explaining what you are currently doing. This will be shown to the user in real-time. "),
		),
	}

	existed := make(map[string]struct{})
	existed["@action"] = struct{}{}
	existed["human_readable_thought"] = struct{}{}

	for _, action := range actions {
		if action == nil {
			continue
		}
		if len(action.Options) <= 0 {
			continue
		}
		for _, opt := range action.Options {
			var rawOpt = opt
			opts = append(opts, rawOpt)
		}
	}

	return aitool.NewObjectSchema(opts...)
}
