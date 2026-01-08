package loop_python_poc

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

var verifySyntax = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"verify_syntax",
		"After you have used bash tool to check Python syntax and confirmed the code has NO syntax errors, call this action to mark syntax as verified. You MUST have already executed a syntax check command (like 'python3 -m py_compile' or similar) and confirmed the output shows no errors before calling this action.",
		[]aitool.ToolOption{
			aitool.WithStringParam("verification_result",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("The result of your syntax verification. Should describe what command you ran and confirm there were no errors.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			verificationResult := action.GetString("verification_result")

			log.Infof("verify_syntax called with result: %s", verificationResult)

			// Mark syntax as verified
			loop.Set("syntax_verified", "true")

			r.AddToTimeline("syntax_verified", "Python 语法验证通过: "+verificationResult)

			// Now AI can exit the loop
			operator.Feedback("语法验证已确认。文件 " + filename + " 的 Python 代码语法正确。你现在可以使用 directly_answer 完成任务。")
		},
	)
}
