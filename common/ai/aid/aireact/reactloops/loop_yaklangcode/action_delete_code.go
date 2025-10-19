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

var deleteCode = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"delete_code",
		"Delete code lines between the specified line numbers (inclusive). The line numbers are 1-based, meaning the first line of the file is line 1. If only 'delete_start_line' is provided, only that single line will be deleted. If both 'delete_start_line' and 'delete_end_line' are provided, all lines in the range will be deleted.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("delete_start_line"),
			aitool.WithIntegerParam("delete_end_line", aitool.WithParam_Required(false)),
			aitool.WithStringParam("delete_code_reason", aitool.WithParam_Description(`Explain why deleting these lines, and summarize the deletion approach and lessons learned, keeping the original code content for future reference value`)),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "delete_code_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			startLine := action.GetInt("delete_start_line")
			endLine := action.GetInt("delete_end_line")
			if startLine <= 0 {
				return utils.Error("delete_code action must have valid 'delete_start_line' parameter")
			}
			if endLine > 0 && endLine < startLine {
				return utils.Error("delete_code action: 'delete_end_line' must be greater than or equal to 'delete_start_line'")
			}

			var msg string
			if endLine > 0 {
				msg = fmt.Sprintf("Preparing delete lines:%v-%v", startLine, endLine)
			} else {
				msg = fmt.Sprintf("Preparing delete line:%v", startLine)
			}
			l.GetEmitter().EmitTextPlainTextStreamEvent(
				"thought",
				bytes.NewReader([]byte(msg)), l.GetCurrentTask().GetIndex())
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for delete_code action")
				return
			}

			invoker := loop.GetInvoker()

			fullCode := loop.Get("full_code")
			editor := memedit.NewMemEditor(fullCode)
			deleteStartLine := action.GetInt("delete_start_line")
			deleteEndLine := action.GetInt("delete_end_line")

			var msg string
			var err error

			if deleteEndLine > 0 {
				// Delete line range
				msg = fmt.Sprintf("decided to delete code lines, from start_line[%v] to end_line:[%v]", deleteStartLine, deleteEndLine)
				log.Infof("start to delete code lines %d to %d", deleteStartLine, deleteEndLine)
				err = editor.DeleteLineRange(deleteStartLine, deleteEndLine)
			} else {
				// Delete single line
				msg = fmt.Sprintf("decided to delete code line[%v]", deleteStartLine)
				log.Infof("start to delete code line %d", deleteStartLine)
				err = editor.DeleteLine(deleteStartLine)
			}

			invoker.AddToTimeline("delete_code", msg)

			reason := action.GetString("delete_code_reason")
			if reason != "" {
				r.AddToTimeline("delete_reason", reason)
			}

			if err != nil {
				r.AddToTimeline("delete_failed", "Failed to delete lines: "+err.Error())
				op.Fail("failed to delete lines: " + err.Error())
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

			if deleteEndLine > 0 {
				msg = fmt.Sprintf("deleted lines[%v-%v]", deleteStartLine, deleteEndLine)
			} else {
				msg = fmt.Sprintf("deleted line[%v]", deleteStartLine)
			}

			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				op.Feedback(errMsg)
			} else {
				msg += "\n\n--[linter]--\nNo issues found after code deletion."
			}
			r.AddToTimeline("code_deleted", msg)
			log.Infof("delete_code done: hasBlockingErrors=%v, will show errors in next iteration", hasBlockingErrors)

			// Emit event with deletion info
			deletionInfo := map[string]interface{}{
				"start_line": deleteStartLine,
			}
			if deleteEndLine > 0 {
				deletionInfo["end_line"] = deleteEndLine
			}
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "delete_code", deletionInfo)

			if errMsg != "" {
				invoker.AddToTimeline("advice", "use 'query_document' to find more syntax sample or docs")
			}
		},
	)
}
