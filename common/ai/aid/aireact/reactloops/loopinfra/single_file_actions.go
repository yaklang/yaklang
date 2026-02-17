package loopinfra

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

// buildWriteAction creates the write_{suffix} action (e.g., write_code, write_content)
func (f *SingleFileModificationSuiteFactory) buildWriteAction() reactloops.ReActLoopOption {
	actionName := f.GetActionName("write")
	return reactloops.WithRegisterLoopAction(
		actionName,
		"If there is NO CODE, you need to create a new file, then use this. If there is already code, it is forbidden to use this action as it will forcibly overwrite the previous code. You must use 'modify_...' to modify the content.",
		nil,
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filenameVar := f.GetFilenameVariableName()
			fullCodeVar := f.GetFullCodeVariableName()
			codeVar := f.GetCodeVariableName()
			runtime := f.GetRuntime()

			filename := loop.Get(filenameVar)
			if filename == "" {
				filename = runtime.EmitFileArtifactWithExt("gen_code", f.GetFileExtension(), "")
				loop.Set(filenameVar, filename)
			}

			action.WaitStream(operator.GetContext())

			log.Infof("single file modification: start to write code to file %s", filename)
			invoker := loop.GetInvoker()

			invoker.AddToTimeline("initialize", "AI decided to initialize the code file: "+filename)
			code := loop.Get(codeVar)

			log.Infof("write_code: extracted code length=%d", len(code))
			loop.Set(fullCodeVar, code)
			if code == "" {
				runtime.AddToTimeline("error", "No code generated in write_code action. The AI must use AI tags to wrap the code.")
				operator.Fail("No code generated in 'write_code' action. Please use AI tags to wrap your code. Do NOT use markdown code blocks.")
				return
			}
			err := os.WriteFile(filename, []byte(code), 0644)
			if err != nil {
				runtime.AddToTimeline("error", "Failed to write code to file: "+err.Error())
				operator.Fail(err)
				return
			}

			// Call file changed callback
			errMsg, blocking := f.OnFileChanged(code, operator)
			if blocking {
				operator.DisallowNextLoopExit()
			} else if f.ShouldExitAfterWrite() {
				operator.Exit()
			}

			msg := utils.ShrinkTextBlock(code, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				operator.Feedback(errMsg)
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the code."
			}
			runtime.AddToTimeline("lint-message", msg)

			//fmt.Println("**********************CODE***************************")
			//fmt.Println(code)
			//fmt.Println("***********************LINT MSG****************************")
			//fmt.Println(msg)
			//fmt.Println("***********************BLOCKED???****************************")
			//fmt.Println("msg", errMsg, blocking)
			//os.Exit(1)

			log.Infof("write_code done: hasBlockingErrors=%v", blocking)
			loop.GetEmitter().EmitPinFilename(filename)
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "write_code", code)
		},
	)
}

// buildModifyAction creates the modify_{suffix} action (e.g., modify_code, modify_content)
func (f *SingleFileModificationSuiteFactory) buildModifyAction() reactloops.ReActLoopOption {
	actionName := f.GetActionName("modify")
	return reactloops.WithRegisterLoopActionWithStreamField(
		actionName,
		"do NOT use this action to create new code file, ONLY use it to modify existing code. Modify the code between the specified line numbers (inclusive). The line numbers are 1-based, meaning the first line of the file is line 1. Ensure that the 'modify_start_line' is less than or equal to 'modify_end_line'.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("modify_start_line"),
			aitool.WithIntegerParam("modify_end_line"),
			aitool.WithStringParam("modify_code_reason", aitool.WithParam_Description(`Fix code errors or issues, and summarize the fixing approach and lessons learned, keeping the original code content for future reference value`)),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "modify_code_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			start := action.GetInt("modify_start_line")
			end := action.GetInt("modify_end_line")
			if start <= 0 || end <= 0 || end < start {
				return utils.Error("modify_code action must have valid 'modify_start_line' and 'modify_end_line' parameters")
			}
			l.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing modify line:%v-%v", start, end))), l.GetCurrentTask().GetIndex())
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filenameVar := f.GetFilenameVariableName()
			fullCodeVar := f.GetFullCodeVariableName()
			codeVar := f.GetCodeVariableName()
			runtime := f.GetRuntime()

			filename := loop.Get(filenameVar)
			if filename == "" {
				op.Fail("no filename found in loop context for modify_code action")
				return
			}

			action.WaitStream(op.GetContext())

			if loop.GetInt("modify_attempts") >= 3 {
				op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
			}

			invoker := loop.GetInvoker()

		fullCode := loop.Get(fullCodeVar)
		partialCode := loop.Get(codeVar)

		editor := memedit.NewMemEditor(fullCode)
			modifyStartLine := action.GetInt("modify_start_line")
			modifyEndLine := action.GetInt("modify_end_line")

			msg := fmt.Sprintf("decided to modify code file, from start_line[%v] to end_line:[%v]", modifyStartLine, modifyEndLine)
			invoker.AddToTimeline("modify_code", msg)

			reason := action.GetString("modify_code_reason")
			if reason != "" {
				runtime.AddToTimeline("modify_reason", reason)
			}

			// Prettify the code (extract line numbers if present)
			start, end, codeSegment, fixedCode := f.PrettifyCode(partialCode)
			if fixedCode {
				if start == modifyStartLine && end == modifyEndLine {
					log.Infof("use prettified code segment for 'modify_code' action, fix range %d to %d", start, end)
					partialCode = codeSegment
				} else {
					runtime.AddToTimeline("modify_warning", fmt.Sprintf("The code segment line numbers [%v-%v] do not match the specified modify line numbers [%v-%v]. Using the original code segment.", start, end, modifyStartLine, modifyEndLine))
					op.Continue()
					return
				}
			}

			log.Infof("start to modify code lines %d to %d", modifyStartLine, modifyEndLine)
			err := editor.ReplaceLineRange(modifyStartLine, modifyEndLine, partialCode)
			if err != nil {
				runtime.AddToTimeline("modify_failed", "Failed to replace line range: "+err.Error())
				op.Fail("failed to replace line range: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			loop.Set(fullCodeVar, fullCode)
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullCode), 0644)

			// Call file changed callback
			errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
			if hasBlockingErrors {
				op.DisallowNextLoopExit()
			}

			// Check for spinning behavior
			isSpinning, spinReason := f.DetectSpinning(loop, modifyStartLine, modifyEndLine)
			if isSpinning {
				// Trigger anti-spinning mechanism
				reflectionPrompt := f.GetReflectionPrompt(modifyStartLine, modifyEndLine, spinReason)
				if reflectionPrompt != "" {
					op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
					op.Feedback(reflectionPrompt)
				}
				invoker.AddToTimeline("spinning_detected", spinReason)
				log.Warnf("spinning detected in modify_code: %s", spinReason)
			}

			msg = utils.ShrinkTextBlock(fmt.Sprintf("line[%v-%v]:\n", modifyStartLine, modifyEndLine)+partialCode, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				if !isSpinning {
					// Only feedback error message if not spinning to avoid repetition
					op.Feedback(errMsg)
				}
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the modified code segment."
			}
			runtime.AddToTimeline("code_modified", msg)
			log.Infof("modify_code done: hasBlockingErrors=%v", hasBlockingErrors)
			loop.GetEmitter().EmitPinFilename(filename)
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "modify_code", partialCode)

			if errMsg != "" && !isSpinning {
				invoker.AddToTimeline("advice", "use search tools to find more syntax samples or docs")
			}
		},
	)
}

// buildInsertAction creates the insert_{suffix} action (e.g., insert_code, insert_content)
func (f *SingleFileModificationSuiteFactory) buildInsertAction() reactloops.ReActLoopOption {
	actionName := f.GetActionName("insert")
	return reactloops.WithRegisterLoopActionWithStreamField(
		actionName,
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
			l.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing insert at line:%v", line))), l.GetCurrentTask().GetIndex())
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filenameVar := f.GetFilenameVariableName()
			fullCodeVar := f.GetFullCodeVariableName()
			codeVar := f.GetCodeVariableName()
			runtime := f.GetRuntime()

			filename := loop.Get(filenameVar)
			if filename == "" {
				op.Fail("no filename found in loop context for insert_lines action")
				return
			}

			action.WaitStream(op.GetContext())

			invoker := loop.GetInvoker()
			fullCode := loop.Get(fullCodeVar)
			partialCode := loop.Get(codeVar)
			editor := memedit.NewMemEditor(fullCode)
			insertLine := action.GetInt("insert_line")

			msg := fmt.Sprintf("decided to insert lines at line[%v]", insertLine)
			invoker.AddToTimeline("insert_lines", msg)

			reason := action.GetString("insert_lines_reason")
			if reason != "" {
				runtime.AddToTimeline("insert_reason", reason)
			}

			// Prettify the code
			start, end, codeSegment, fixedCode := f.PrettifyCode(partialCode)
			if fixedCode {
				log.Infof("use prettified code segment for 'insert_lines' action, original range %d to %d", start, end)
				partialCode = codeSegment
			}

			log.Infof("start to insert code at line %d", insertLine)
			err := editor.InsertAtLine(insertLine, partialCode)
			if err != nil {
				runtime.AddToTimeline("insert_failed", "Failed to insert at line: "+err.Error())
				op.Fail("failed to insert at line: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			loop.Set(fullCodeVar, fullCode)
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullCode), 0644)

			// Call file changed callback
			errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
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
			runtime.AddToTimeline("lines_inserted", msg)
			log.Infof("insert_lines done: hasBlockingErrors=%v", hasBlockingErrors)
			loop.GetEmitter().EmitPinFilename(filename)
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "insert_lines", partialCode)

			if errMsg != "" {
				invoker.AddToTimeline("advice", "use search tools to find more syntax samples or docs")
			}
		},
	)
}

// buildDeleteAction creates the delete_{suffix} action (e.g., delete_code, delete_content)
func (f *SingleFileModificationSuiteFactory) buildDeleteAction() reactloops.ReActLoopOption {
	actionName := f.GetActionName("delete")
	return reactloops.WithRegisterLoopActionWithStreamField(
		actionName,
		"Delete lines between the specified line numbers (inclusive). Use this action to remove unwanted lines, comments, or code blocks. The line numbers are 1-based, meaning the first line of the file is line 1. If only 'delete_start_line' is provided, only that single line will be deleted. If both 'delete_start_line' and 'delete_end_line' are provided, all lines in the range will be deleted. This is more precise than others for line deletion.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("delete_start_line"),
			aitool.WithIntegerParam("delete_end_line", aitool.WithParam_Required(false)),
			aitool.WithStringParam("delete_lines_reason", aitool.WithParam_Description(`Explain why deleting these lines, and summarize the deletion approach and lessons learned, keeping the original code content for future reference value`)),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "delete_lines_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			startLine := action.GetInt("delete_start_line")
			endLine := action.GetInt("delete_end_line")
			if startLine <= 0 {
				return utils.Error("delete_lines action must have valid 'delete_start_line' parameter")
			}
			if endLine > 0 && endLine < startLine {
				return utils.Error("delete_lines action: 'delete_end_line' must be greater than or equal to 'delete_start_line'")
			}

			var msg string
			if endLine > 0 {
				msg = fmt.Sprintf("Preparing delete lines:%v-%v", startLine, endLine)
			} else {
				msg = fmt.Sprintf("Preparing delete line:%v", startLine)
			}
			l.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(msg)), l.GetCurrentTask().GetIndex())
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filenameVar := f.GetFilenameVariableName()
			fullCodeVar := f.GetFullCodeVariableName()
			runtime := f.GetRuntime()

			filename := loop.Get(filenameVar)
			if filename == "" {
				op.Fail("no filename found in loop context for delete_lines action")
				return
			}

			invoker := loop.GetInvoker()

			fullCode := loop.Get(fullCodeVar)
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

			invoker.AddToTimeline("delete_lines", msg)

			reason := action.GetString("delete_lines_reason")
			if reason != "" {
				runtime.AddToTimeline("delete_reason", reason)
			}

			if err != nil {
				runtime.AddToTimeline("delete_failed", "Failed to delete lines: "+err.Error())
				op.Fail("failed to delete lines: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			loop.Set(fullCodeVar, fullCode)
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullCode), 0644)

			// Call file changed callback
			errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
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
			runtime.AddToTimeline("lines_deleted", msg)
			log.Infof("delete_lines done: hasBlockingErrors=%v", hasBlockingErrors)
			loop.GetEmitter().EmitPinFilename(filename)

			// Emit event with deletion info
			deletionInfo := map[string]interface{}{
				"start_line": deleteStartLine,
			}
			if deleteEndLine > 0 {
				deletionInfo["end_line"] = deleteEndLine
			}
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "delete_lines", deletionInfo)

			if errMsg != "" {
				invoker.AddToTimeline("advice", "use search tools to find more syntax samples or docs")
			}
		},
	)
}
