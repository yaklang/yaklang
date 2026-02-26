package loop_smart_qa

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func makeFinalAnswerAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	desc := "Provide the final, comprehensive answer to the user's question. Use this when you have gathered enough information or can answer directly."

	toolOpts := []aitool.ToolOption{
		aitool.WithStringParam("answer",
			aitool.WithParam_Description("The comprehensive answer to the user's question, synthesizing all gathered information."),
			aitool.WithParam_Required(true)),
	}

	return reactloops.WithRegisterLoopAction(
		"final_answer",
		desc, toolOpts,
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
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			loop.Set("final_answer", answer)
			invoker.AddToTimeline("smart_qa_final_answer", answer)

			result, err := invoker.DirectlyAnswer(ctx, answer, nil)
			_ = result
			log.Infof("smart_qa final answer result: %s", utils.ShrinkTextBlock(result, 512))
			if err != nil {
				op.Continue()
			} else {
				op.Exit()
			}
		},
	)
}

var finalAnswerAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return makeFinalAnswerAction(r)
}
