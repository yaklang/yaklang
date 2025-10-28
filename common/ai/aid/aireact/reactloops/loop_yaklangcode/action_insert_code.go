package loop_yaklangcode

import (
	"bytes"
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

var insertLines = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"insert_lines",
		"Insert new lines at the specified line number. Use this action to add new code, comments, or blank lines. The line number is 1-based, meaning the first line of the file is line 1. The lines will be inserted at the beginning of the specified line, pushing existing content down. This is ideal for adding new functionality or fixing missing code.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("insert_line"),
			aitool.WithStringParam("insert_lines_reason", aitool.WithParam_Description(`Explain why inserting lines at this position, and summarize the insertion approach and lessons learned, keeping the original code content for future reference value`)),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "insert_lines_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			line := action.GetInt("insert_line")
			if line <= 0 {
				return utils.Error("insert_lines action must have valid 'insert_line' parameter")
			}
			l.GetEmitter().EmitTextPlainTextStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing insert at line:%v", line))), l.GetCurrentTask().GetIndex())
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for insert_lines action")
				return
			}

			invoker := loop.GetInvoker()
			fullCode := loop.Get("full_code")
			partialCode := loop.Get("yak_code")
			editor := memedit.NewMemEditor(fullCode)
			insertLine := action.GetInt("insert_line")

			msg := fmt.Sprintf("decided to insert lines at line[%v]", insertLine)
			invoker.AddToTimeline("insert_lines", msg)

			reason := action.GetString("insert_lines_reason")
			if reason != "" {
				r.AddToTimeline("insert_reason", reason)
			}

			start, end, codeSegment, fixedCode := prettifyAITagCode(partialCode)
			if fixedCode {
				log.Infof("use prettified code segment for 'insert_lines' action, original range %d to %d", start, end)
				partialCode = codeSegment
			}

			fmt.Println("=================================================")
			fmt.Println(string(partialCode))
			fmt.Println("=================================================")

			log.Infof("start to insert code at line %d", insertLine)
			err := editor.InsertAtLine(insertLine, partialCode)
			if err != nil {
				r.AddToTimeline("insert_failed", "Failed to insert at line: "+err.Error())
				op.Fail("failed to insert at line: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			loop.Set("full_code", fullCode)
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullCode), 0644)

			errMsg, hasBlockingErrors := checkCodeAndFormatErrors(fullCode)
			if hasBlockingErrors {
				op.DisallowNextLoopExit()
			}
			msg = utils.ShrinkTextBlock(fmt.Sprintf("inserted at line[%v]:\n", insertLine)+partialCode, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				op.Feedback(errMsg)
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the inserted code segment."
			}
			r.AddToTimeline("lines_inserted", msg)
			log.Infof("insert_lines done: hasBlockingErrors=%v, will show errors in next iteration", hasBlockingErrors)
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "insert_lines", partialCode)

			if errMsg != "" {
				invoker.AddToTimeline("advice", "use 'grep_yaklang_samples' to find more syntax sample or docs")
			}
		},
	)
}
