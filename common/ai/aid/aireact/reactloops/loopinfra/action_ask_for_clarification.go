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
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName: "question",
		},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		result := action.GetInvokeParams("ask_for_clarification_payload")
		if !result.Has("question") {
			result = action.GetInvokeParams("next_action").GetObject("ask_for_clarification_payload")
		}
		question := result.GetString("question")
		if question == "" {
			return utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'")
		}
		loop.Set("question", question)
		loop.Set("options", result.GetStringSlice("options"))
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		callCount := loop.GetInt("ask_for_clarification_call_count")

		delta := func(i int) {
			loop.Set("ask_for_clarification_call_count", callCount+i)
			if callCount+i >= int(loop.GetConfig().GetUserInteractiveLimitedTimes()) {
				loop.RemoveAction(schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION)
			}
		}

		question := loop.Get("question")
		if question == "" {
			operator.Feedback(utils.Error("ask_for_clarification action must have 'question' field in 'ask_for_clarification_payload'"))
			operator.Continue()
			delta(1)
			return
		}
		options := loop.GetStringSlice("options")
		invoker := loop.GetInvoker()
		suggestion := invoker.AskForClarification(question, options)
		if suggestion == "" {
			suggestion = "user did not provide a valid suggestion, using default 'continue' action"
		}
		delta(1)
	},
}
