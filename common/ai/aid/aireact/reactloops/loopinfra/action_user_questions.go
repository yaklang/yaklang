package loopinfra

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
)

func userQuestionsAction(blocking bool) *reactloops.LoopAction {
	name := "ask_for_clarification_async"
	description := "Ask 1–3 structured questions. Returns accepted IMMEDIATELY, does NOT wait. Continue independent work. Answers arrive later as ordinary user messages; merge them as new constraints. Never poll, repeat the question, or idle waiting."
	if blocking {
		name = "ask_for_clarification"
		description = "Ask structured questions and WAIT for an answer or explicit cancellation. Use only when no useful work can continue without the answer; otherwise prefer ask_for_clarification_async."
	}
	return &reactloops.LoopAction{
		ActionType: name, Description: description,
		Options: []aitool.ToolOption{
			aitool.WithRawParam("questions", interaction.QuestionSchema(), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("timeoutSeconds", aitool.WithParam_Description("Optional async deadline in seconds, 0 means no deadline. Any user interaction permanently cancels the deadline. Ignored in blocking mode.")),
		},
		ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			cfg, ok := loop.GetConfig().(*aicommon.Config)
			if !ok || !cfg.UIQuestionsEnabled() {
				return fmt.Errorf("structured questions are unavailable")
			}
			return loop.CheckClarificationAllowed(blocking)
		},
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			defer operator.Continue()
			if err := loop.CheckClarificationAllowed(blocking); err != nil {
				operator.Feedback(err.Error())
				return
			}
			cfg := loop.GetConfig().(*aicommon.Config)
			params := action.GetParams()
			if !params.Has("questions") && !params.Has("ask_for_clarification_payload") {
				params = action.GetInvokeParams("next_action")
			}
			// Accept the original payload while clients/models migrate to questions[].
			if blocking && !params.Has("questions") && params.Has("ask_for_clarification_payload") {
				if err := loopAction_AskForClarification.ActionVerifier(loop, action); err != nil {
					operator.Feedback(err.Error())
					return
				}
				loopAction_AskForClarification.ActionHandler(loop, action, operator)
				return
			}
			if err := interaction.ValidateSchema(interaction.QuestionSchema(), params["questions"]); err != nil {
				operator.Feedback(err.Error())
				return
			}
			raw, _ := json.Marshal(params["questions"])
			var questions []interaction.Question
			if err := json.Unmarshal(raw, &questions); err != nil {
				operator.Feedback(err.Error())
				return
			}
			ctx, taskID := cfg.GetContext(), ""
			if task := loop.GetCurrentTask(); task != nil {
				ctx, taskID = task.GetContext(), task.GetId()
			}
			result, err := cfg.RequestQuestions(ctx, taskID, questions, blocking, int(params.GetInt("timeoutSeconds")))
			if err != nil {
				operator.Feedback(err.Error())
				return
			}
			operator.Feedback(result)
		},
	}
}
