package reactloops

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_Finish = &LoopAction{
	ActionType:  "finish",
	Description: "Finish the task, MUST fill the 'human_readable_thought' field",
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		loop.invoker.AddToTimeline("finish", "AI decided mark the current Task is finished")
		operator.Exit()
	},
}

var loopAction_DirectlyAnswer = &LoopAction{
	ActionType:  "directly_answer",
	Description: "Directly the 'answer_payload' field",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"answer_payload",
		),
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
		payload := action.GetString("answer_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("answer_payload")
		}
		if payload == "" {
			return utils.Error("answer_payload is required for ActionDirectlyAnswer but empty")
		}
		loop.Set("directly_answer_payload", payload)
		return nil
	},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		payload := loop.Get(`directly_answer_payload`)
		if payload == "" {
			operator.Fail("directly_answer action must have 'answer_payload' field")
			return
		}
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)
		invoker.AddToTimeline("directly_answer", fmt.Sprintf("user input: \n"+
			"%s\n"+
			"ai directly answer:\n"+
			"%v",
			utils.PrefixLines(loop.GetCurrentTask().GetUserInput(), "  > "),
			utils.PrefixLines(payload, "  | "),
		))
		operator.Exit()
	},
}
