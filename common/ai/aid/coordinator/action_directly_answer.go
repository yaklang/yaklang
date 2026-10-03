package coordinator

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func configureCoordinatorAnswer(loop *reactloops.ReActLoop) error {
	answer, err := loop.GetActionHandler("directly_answer")
	if err != nil {
		return err
	}
	answerCopy := *answer
	answerCopy.Description = "向用户说明进展或答复，之后继续协调。短答复使用 answer_payload；长答复使用主循环声明的 FINAL_ANSWER AITAG，两者不可同时输出。"
	answerCopy.NativeDescription = "向用户说明进展或答复，之后继续协调；完整正文写入 arguments.answer_payload，不使用外置 AITAG。"
	answerCopy.Options = []aitool.ToolOption{aitool.WithStringParam("answer_payload", aitool.WithParam_Description("短答复正文；长答复使用主循环声明的 FINAL_ANSWER AITAG，两者不可同时输出。"))}
	answerCopy.NativeOptions = []aitool.ToolOption{aitool.WithStringParam("answer_payload", aitool.WithParam_Description("向用户交付的完整答复，支持 Markdown 或代码；不使用外置 AITAG。"), aitool.WithParam_Required())}
	answerCopy.ActionHandler = func(l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		if !l.FunctionCallModeEnabled() {
			answer.ActionHandler(l, a, op)
			return
		}
		l.GetEmitter().EmitTextMarkdownStreamEvent("re-act-loop-answer-payload", strings.NewReader(a.GetString("answer_payload")), "")
		op.Continue()
	}
	answerCopy.FunctionCallAction = nil
	reactloops.WithOverrideLoopAction(&answerCopy)(loop)
	return nil
}
