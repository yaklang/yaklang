package loop_yaklangcode

import (
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var writeCode = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"write_code",
		"If there is NO CODE, you need to create a new file, then use 'write_code'. If there is already code, it is forbidden to use 'write_code' as it will forcibly overwrite the previous code. You must use 'modify_code' to modify the code.",
		nil,
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				filename = r.EmitFileArtifactWithExt("gen_code", ".yak", "")
			}

			invoker := loop.GetInvoker()

			invoker.AddToTimeline("initialize", "AI decided to initialize the code file: "+filename)
			code := loop.Get("yak_code")
			loop.Set("full_code", code)
			if code == "" {
				r.AddToTimeline("error", "No code generated in write_code action")
				operator.Fail("No code generated in 'write_code' action")
				return
			}
			err := os.WriteFile(filename, []byte(code), 0644)
			if err != nil {
				r.AddToTimeline("error", "Failed to write code to file: "+err.Error())
				operator.Fail(err)
				return
			}
			errMsg, blocking := checkCodeAndFormatErrors(code)
			if blocking {
				operator.DisallowNextLoopExit()
				loop.RemoveAction("write_code")
			}
			msg := utils.ShrinkTextBlock(code, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				operator.Feedback(errMsg)
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the modified code segment."
			}
			r.AddToTimeline("initial-yaklang-code", msg)
			log.Infof("write_code done: hasBlockingErrors=%v, will show errors in next iteration", blocking)
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "write_code", code)
		},
	)
}
