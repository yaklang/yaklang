package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_AskForClarification = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION,
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
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		result := action.GetInvokeParams("ask_for_clarification_payload")
		if result.GetString("question") == "" {
			return utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'")
		}
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		result := action.GetInvokeParams("ask_for_clarification_payload")
		if result.GetString("question") == "" {
			operator.Feedback(utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'"))
			return
		}
		question := result.GetString("question")
		options := result.GetStringSlice("options")

		invoker := loop.GetInvoker()
		suggestion := invoker.AskForClarification(question, options)
		if suggestion == "" {
			suggestion = "user did not provide a valid suggestion, using default 'continue' action"
		}
	},
}
