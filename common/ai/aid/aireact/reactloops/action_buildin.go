package reactloops

import (
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
			return utils.Error("answer_payload is required for ActionDirectlyAnswer but empty")
		}
		return nil
	},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		invoker := loop.GetInvoker()
		payload := action.GetString("answer_payload")
		invoker.EmitFileArtifactWithExt("directly_answer", ".md", payload)
		invoker.EmitResultAfterStream(payload)

		// directly_answer 默认继续循环
		operator.Continue()
	},
}
