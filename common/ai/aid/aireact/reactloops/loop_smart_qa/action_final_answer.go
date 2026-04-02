package loop_smart_qa

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func makeFinalAnswerAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "Provide the final, comprehensive answer to the user's question. Use this when you have gathered enough information or can answer directly."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("answer",
			aitool.WithParam_Description("The comprehensive answer to the user's question, synthesizing all gathered information."),
			aitool.WithParam_Required(true)),
	}

	streamFields := []*reactloops.LoopStreamField{
		{
			FieldName:   "answer",
			AINodeId:    "re-act-loop-answer-payload",
			ContentType: aicommon.TypeTextMarkdown,
		},
	}

	return reactloops.WithRegisterLoopActionWithStreamField(
		"final_answer",
		desc, toolOpts, streamFields,
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			answer := strings.TrimSpace(action.GetString("answer"))
			if answer == "" {
				return utils.Error("answer is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.LoadingStatus("preparing final answer")

			answer := strings.TrimSpace(action.GetString("answer"))
			if answer == "" {
				op.Fail("final_answer action requires non-empty answer")
				return
			}

			invoker := loop.GetInvoker()

			loop.Set("final_answer", answer)
			invoker.EmitFileArtifactWithExt("smart_qa_final_answer", ".md", answer)
			invoker.EmitResultAfterStream(answer)
			invoker.AddToTimeline("smart_qa_final_answer", answer)
			op.Exit()
		},
	)
}

var finalAnswerAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeFinalAnswerAction(r)
}
