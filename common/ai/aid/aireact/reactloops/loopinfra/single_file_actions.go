package loopinfra

import (
	"fmt"
	"os"
	"strings"

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
			fullCodeVar := f.GetFullCodeVariableName()
			existing := l.Get(fullCodeVar)
			if existing != "" && !AllowWriteCodeDespiteExistingSeed(l, fullCodeVar) {
				return fmt.Errorf("code already exists (%d bytes). Use 'modify_%s' to make changes instead of 'write_%s'",
					len(existing), f.actionSuffix, f.actionSuffix)
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filenameVar := f.GetFilenameVariableName()
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
			loopInfraActionStart(loop, loopInfraNodeSingleFileWrite,
				fmt.Sprintf("写入文件: %s / Write file: %s", filename, filename),
				"写入文件中 / Writing File...")

			invoker.AddToTimeline("initialize", "AI decided to initialize the code file: "+filename)
			code := loop.Get(codeVar)

			log.Infof("write_code: extracted code length=%d", len(code))
			if code == "" {
				// 空 write_code 不应让整个任务夭折: AI 常常是"想先查文档/样例, 却误把动作选成了
				// write_code 而没附代码块"。这里给出纠正反馈并继续循环, 让 AI 下一轮改用查询动作或
				// 补上完整代码块; 仅当连续多次空写(模型真卡住)才放弃, 既容错又避免死循环。
				// 关键词: 空 write_code 容错, 反馈而非夭折, 连续空写阈值
				const maxEmptyWriteRetry = 3
				emptyCountVar := actionName + "_empty_write_count"
				emptyCount := loop.GetInt(emptyCountVar) + 1
				loop.Set(emptyCountVar, fmt.Sprint(emptyCount))
				failMsg := f.DiagnoseMissingWriteCode(loop)
				runtime.AddToTimeline("error", failMsg)
				if emptyCount >= maxEmptyWriteRetry {
					operator.Fail(fmt.Sprintf("%s (consecutive empty write_code x%d, give up)", failMsg, emptyCount))
					return
				}
				operator.Feedback(failMsg + "\n\nHINT: 你很可能【输出完 @action JSON 就停下了】, 没有紧接着输出代码块。@action JSON 与紧随其后的 `<|" + f.aiTagName + "_<nonce>|> ... <|" + f.aiTagName + "_END_<nonce>|>` 代码块是【同一次回复、不可分割的一体】——发完 JSON 不要结束, 必须在同一条消息里把代码块完整写完(含 END 结束标记)再停。\n如果你只是想先确认 API 签名或语法, 请改用查询动作 (yakdoc_function_details / grep_yaklang_samples / semantic_search_yaklang_samples), 不要发空的 write_code。")
				loopInfraStatus(loop, "write_code 缺少代码, 已反馈纠正并继续 / write_code missing code, fed back and continue")
				return
			}
			// 成功提取到代码, 重置空写计数。
			loop.Set(actionName+"_empty_write_count", "0")
			err := f.persistLoopFileContent(
				runtime, filename, code,
				"write_success", "write_failed",
				fmt.Sprintf("SUCCESS: wrote %d bytes to file: %s", len(code), filename),
			)
			if err != nil {
				operator.Fail(err)
				return
			}

			loopInfraAddFileOpSuccessTimeline(loop, loopInfraFileOpTimeline{
				Op:         "write",
				Filename:   filename,
				NewSegment: code,
				Deferred:   f.ShouldDeferDiskWrite(),
			})

			if !f.ShouldDeferDiskWrite() {
				// Verify file was written correctly
				writtenBytes, verifyErr := os.ReadFile(filename)
				if verifyErr != nil {
					runtime.AddToTimeline("write_verify_failed", fmt.Sprintf("FAILED to verify written file: %s, error: %s", filename, verifyErr.Error()))
					operator.Fail(fmt.Sprintf("file write verification failed: %v", verifyErr))
					return
				}
				runtime.AddToTimeline("write_verified", fmt.Sprintf("verified: %d bytes on disk", len(writtenBytes)))
			}

			loop.Set(f.GetFullCodeVariableName(), code)

			// Call file changed callback
			errMsg, blocking := f.OnFileChanged(code, operator)
			runBlocked := f.applySyntaxLintResult(loop, operator, blocking, f.ShouldExitAfterWrite() || f.ShouldExitWhenSyntaxClean())

			msg := utils.ShrinkTextBlock(code, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				operator.Feedback(errMsg)
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the code."
			}
			runtime.AddToTimeline("lint-message", msg)

			log.Infof("write_code done: hasBlockingErrors=%v runBlocked=%v", blocking, runBlocked)
			if !blocking && !runBlocked {
				loopInfraStatus(loop, "文件写入完成 / File Write Complete")
				loopInfraActionFinish(loop, loopInfraNodeSingleFileWrite,
					fmt.Sprintf("文件写入完成: %s (%d bytes) / File Written: %s (%d bytes)", filename, len(code), filename, len(code)),
					msg)
			}
			loop.GetEmitter().EmitPinFilename(filename)
			_, _ = f.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
				Content:       code,
				Path:          filename,
				SourceAction:  actionName,
				EventOp:       loopYaklangCodeEventOpCreate,
				EmitEvent:     true,
				DeliveryPatch: BuildYaklangPatchFull(code),
			})
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "write_code", code)
		},
	)
}

// buildModifyAction creates the modify_{suffix} action (e.g., modify_code, modify_content)
func (f *SingleFileModificationSuiteFactory) buildModifyAction() reactloops.ReActLoopOption {
	actionName := f.GetActionName("modify")
	return reactloops.WithRegisterLoopActionWithStreamField(
		actionName,
		`Modify existing code. Preferred mode:
1) Patch (default): put a Cursor-style Apply Patch in the GEN_CODE / content tag
   (*** Begin Patch ... *** End Patch). System applies it to full_code then emits the merged full file (never raw patch) to the frontend.
Legacy fallbacks when GEN_CODE is NOT a patch:
2) Snippet: old_snippet (exact text match) + optional replace_all.
3) Line range: modify_start_line + modify_end_line (1-based, inclusive).`,
		[]aitool.ToolOption{
			aitool.WithIntegerParam("modify_start_line", aitool.WithParam_Description("Legacy line-range start (optional if GEN_CODE is a patch or old_snippet is set)")),
			aitool.WithIntegerParam("modify_end_line", aitool.WithParam_Description("Legacy line-range end (optional if GEN_CODE is a patch or old_snippet is set)")),
			aitool.WithStringParam("old_snippet", aitool.WithParam_Description("Legacy exact snippet from full_code to replace when GEN_CODE is not a patch")),
			aitool.WithBoolParam("replace_all", aitool.WithParam_Description("Replace all old_snippet matches (snippet mode, default false)")),
			aitool.WithStringParam("modify_code_reason", aitool.WithParam_Description(`Fix code errors or issues, and summarize the fixing approach and lessons learned, keeping the original code content for future reference value`)),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "modify_code_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			oldSnippet := strings.TrimSpace(action.GetString("old_snippet"))
			if oldSnippet != "" {
				return nil
			}
			start := action.GetInt("modify_start_line")
			end := action.GetInt("modify_end_line")
			// Patch mode: JSON may omit line range / old_snippet; GEN_CODE is validated after WaitStream.
			if start == 0 && end == 0 {
				loopInfraStatus(l, "准备以 Patch 修改文件 / Preparing Patch Modify")
				return nil
			}
			if start <= 0 || end <= 0 || end < start {
				return utils.Error("modify_code action must have a GEN_CODE patch (*** Begin Patch), valid 'modify_start_line'/'modify_end_line', or 'old_snippet'")
			}
			loopInfraStatus(l, fmt.Sprintf("准备修改文件行 %d-%d / Preparing File Modify Lines %d-%d", start, end, start, end))
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filenameVar := f.GetFilenameVariableName()
			fullCodeVar := f.GetFullCodeVariableName()
			codeVar := f.GetCodeVariableName()
			runtime := f.GetRuntime()
			actionName := f.GetActionName("modify")

			filename := loop.Get(filenameVar)
			if filename == "" {
				op.Fail("no filename found in loop context for modify_code action")
				return
			}

			action.WaitStream(op.GetContext())

			// Preferred path: Cursor-style patch inside GEN_CODE (ignores old_snippet / line range).
			if LooksLikeCodePatch(loop.Get(codeVar)) {
				f.handleModifyByPatch(loop, action, op, actionName, filename, fullCodeVar, codeVar)
				return
			}

			if strings.TrimSpace(action.GetString("old_snippet")) != "" {
				f.handleModifyByOldSnippet(loop, action, op, actionName, filename, fullCodeVar, codeVar)
				return
			}

			start := action.GetInt("modify_start_line")
			end := action.GetInt("modify_end_line")
			if start <= 0 || end <= 0 || end < start {
				msg := fmt.Sprintf(`【modify_code 失败】GEN_CODE 不是 Patch（缺少 *** Begin Patch），且未提供 old_snippet / 有效行号。

请优先输出 Cursor 风格 Patch：
{"@action":"modify_code","modify_code_reason":"..."}
<|%s_<nonce>|>
*** Begin Patch
*** Update File: current
@@ context
-old
+new
*** End Patch
<|%s_END_<nonce>|>

或回退：old_snippet / modify_start_line+modify_end_line。`, f.aiTagName, f.aiTagName)
				runtime.AddToTimeline("modify_no_locator", msg)
				op.Feedback(msg)
				op.Continue()
				return
			}

			if loop.GetInt("modify_attempts") >= 3 {
				op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
			}

			invoker := loop.GetInvoker()

			fullCode := loop.Get(fullCodeVar)
			partialCode := loop.Get(codeVar)

			editor := memedit.NewMemEditor(fullCode)
			modifyStartLine := NormalizeActionLineNumber(loop, fullCodeVar, action.GetInt("modify_start_line"))
			modifyEndLine := NormalizeActionLineNumber(loop, fullCodeVar, action.GetInt("modify_end_line"))

			msg := fmt.Sprintf("decided to modify code file, from start_line[%v] to end_line:[%v]", modifyStartLine, modifyEndLine)
			invoker.AddToTimeline("modify_code", msg)
			reason := action.GetString("modify_code_reason")
			loopInfraActionStart(loop, loopInfraNodeSingleFileModify,
				fmt.Sprintf("修改文件行 %d-%d: %s / Modify file lines %d-%d: %s", modifyStartLine, modifyEndLine, filename, modifyStartLine, modifyEndLine, filename),
				"修改文件中 / Modifying File...")

			// Prettify the code (extract line numbers if present)
			start, end, codeSegment, fixedCode := f.PrettifyCode(partialCode)
			if fixedCode {
				start = NormalizeActionLineNumber(loop, fullCodeVar, start)
				end = NormalizeActionLineNumber(loop, fullCodeVar, end)
				lineDiffStart := start - modifyStartLine
				lineDiffEnd := end - modifyEndLine
				if start == modifyStartLine && end == modifyEndLine {
					log.Infof("use prettified code segment for 'modify_code' action, fix range %d to %d", start, end)
					partialCode = codeSegment
				} else if lineDiffStart >= -2 && lineDiffStart <= 2 && lineDiffEnd >= -2 && lineDiffEnd <= 2 {
					modifyStartLine = start
					modifyEndLine = end
					partialCode = codeSegment
					correctMsg := fmt.Sprintf("Adjusted modify range to prettified lines [%d-%d] (requested [%d-%d]).",
						start, end, action.GetInt("modify_start_line"), action.GetInt("modify_end_line"))
					runtime.AddToTimeline("modify_line_corrected", correctMsg)
					op.Feedback(correctMsg)
				} else {
					warnMsg := fmt.Sprintf(`modify_code 行号与 GEN_CODE 块不一致。

指定行号：[%d-%d]
GEN_CODE 解析行号：[%d-%d]

请使用 modify_code 的 old_snippet 精确匹配，或修正行号后重试。`, modifyStartLine, modifyEndLine, start, end)
					runtime.AddToTimeline("modify_warning", warnMsg)
					op.Feedback(warnMsg)
					op.Continue()
					return
				}
			}

			log.Infof("start to modify code lines %d to %d", modifyStartLine, modifyEndLine)
			oldSegment := loopInfraExtractLineRange(fullCode, modifyStartLine, modifyEndLine)
			err := editor.ReplaceLineRange(modifyStartLine, modifyEndLine, partialCode)
			if err != nil {
				runtime.AddToTimeline("modify_failed", "Failed to replace line range: "+err.Error())
				op.Fail("failed to replace line range: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			writeErr := f.replaceLoopFileContent(
				runtime, filename, fullCode,
				"modify_success", "modify_write_failed",
				fmt.Sprintf("SUCCESS: modified lines[%d-%d], wrote %d bytes to file: %s", modifyStartLine, modifyEndLine, len(fullCode), filename),
			)
			if writeErr != nil {
				op.Fail(fmt.Sprintf("failed to write modified content to file: %v", writeErr))
				return
			}
			loop.Set(fullCodeVar, fullCode)

			loopInfraAddFileOpSuccessTimeline(loop, loopInfraFileOpTimeline{
				Op:         "modify",
				Filename:   filename,
				OldSegment: oldSegment,
				NewSegment: partialCode,
				StartLine:  modifyStartLine,
				EndLine:    modifyEndLine,
				Deferred:   f.ShouldDeferDiskWrite(),
			})

			// Call file changed callback
			errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
			// modify 操作不自动退出：AI 可能需要多次修改，由 AI 主动调用 finish 退出。
			runBlocked := f.applySyntaxLintResult(loop, op, hasBlockingErrors, false)

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
				if hasBlockingErrors || !isSpinning {
					op.Feedback(errMsg)
				}
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the modified code segment."
			}
			runtime.AddToTimeline("code_modified", msg)
			log.Infof("modify_code done: hasBlockingErrors=%v runBlocked=%v", hasBlockingErrors, runBlocked)
			if !hasBlockingErrors && !runBlocked {
				loopInfraStatus(loop, "文件修改完成 / File Modify Complete")
				loopInfraActionFinish(loop, loopInfraNodeSingleFileModify,
					fmt.Sprintf("文件修改完成: %s lines %d-%d / File Modified: %s lines %d-%d", filename, modifyStartLine, modifyEndLine, filename, modifyStartLine, modifyEndLine),
					msg)
			}
			loop.GetEmitter().EmitPinFilename(filename)
			_, _ = f.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
				Content:       fullCode,
				Path:          filename,
				SourceAction:  actionName,
				ChangeReason:  reason,
				EventOp:       loopYaklangCodeEventOpReplace,
				EmitEvent:     true,
				DeliveryPatch: BuildYaklangPatchLineRange(partialCode, modifyStartLine, modifyEndLine, oldSegment, loop.GetInt(LoopVarCodeLineBase)),
			})
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
		},
		nil,
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			line := action.GetInt("insert_line")
			if line <= 0 {
				return utils.Error("insert_lines action must have valid 'insert_line' parameter")
			}
			loopInfraStatus(l, fmt.Sprintf("准备插入文件行 %d / Preparing File Insert Line %d", line, line))
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
			insertLine := NormalizeActionLineNumber(loop, fullCodeVar, action.GetInt("insert_line"))

			msg := fmt.Sprintf("decided to insert lines at line[%v]", insertLine)
			invoker.AddToTimeline("insert_lines", msg)
			loopInfraActionStart(loop, loopInfraNodeSingleFileInsert,
				fmt.Sprintf("插入文件行 %d: %s / Insert file line %d: %s", insertLine, filename, insertLine, filename),
				"插入文件内容 / Inserting File Content...")

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
			writeErr := f.replaceLoopFileContent(
				runtime, filename, fullCode,
				"insert_success", "insert_write_failed",
				fmt.Sprintf("SUCCESS: inserted at line[%d], wrote %d bytes to file: %s", insertLine, len(fullCode), filename),
			)
			if writeErr != nil {
				op.Fail(fmt.Sprintf("failed to write content after insert: %v", writeErr))
				return
			}
			loop.Set(fullCodeVar, fullCode)

			loopInfraAddFileOpSuccessTimeline(loop, loopInfraFileOpTimeline{
				Op:         "insert",
				Filename:   filename,
				NewSegment: partialCode,
				InsertLine: insertLine,
				Deferred:   f.ShouldDeferDiskWrite(),
			})

			// Call file changed callback
			errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
			// insert 操作不自动退出：AI 可能需要多次修改，由 AI 主动调用 finish 退出。
			runBlocked := f.applySyntaxLintResult(loop, op, hasBlockingErrors, false)
			msg = utils.ShrinkTextBlock(fmt.Sprintf("inserted at line[%v]:\n", insertLine)+partialCode, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				op.Feedback(errMsg)
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the inserted code segment."
			}
			runtime.AddToTimeline("lines_inserted", msg)
			log.Infof("insert_lines done: hasBlockingErrors=%v runBlocked=%v", hasBlockingErrors, runBlocked)
			if !hasBlockingErrors && !runBlocked {
				loopInfraStatus(loop, "文件插入完成 / File Insert Complete")
				loopInfraActionFinish(loop, loopInfraNodeSingleFileInsert,
					fmt.Sprintf("文件插入完成: %s line %d / File Inserted: %s line %d", filename, insertLine, filename, insertLine),
					msg)
			}
			loop.GetEmitter().EmitPinFilename(filename)
			_, _ = f.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
				Content:       fullCode,
				Path:          filename,
				SourceAction:  actionName,
				EventOp:       loopYaklangCodeEventOpReplace,
				EmitEvent:     true,
				DeliveryPatch: BuildYaklangPatchInsert(partialCode, insertLine, loop.GetInt(LoopVarCodeLineBase)),
			})
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
		},
		nil,
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			startLine := action.GetInt("delete_start_line")
			endLine := action.GetInt("delete_end_line")
			if startLine <= 0 {
				return utils.Error("delete_lines action must have valid 'delete_start_line' parameter")
			}
			if endLine > 0 && endLine < startLine {
				return utils.Error("delete_lines action: 'delete_end_line' must be greater than or equal to 'delete_start_line'")
			}

			if endLine > 0 {
				loopInfraStatus(l, fmt.Sprintf("准备删除文件行 %d-%d / Preparing File Delete Lines %d-%d", startLine, endLine, startLine, endLine))
			} else {
				loopInfraStatus(l, fmt.Sprintf("准备删除文件行 %d / Preparing File Delete Line %d", startLine, startLine))
			}
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
			deleteStartLine := NormalizeActionLineNumber(loop, fullCodeVar, action.GetInt("delete_start_line"))
			deleteEndLine := NormalizeActionLineNumber(loop, fullCodeVar, action.GetInt("delete_end_line"))

			var msg string
			var err error
			var deletedStart, deletedEnd int

			if deleteEndLine > 0 {
				// Delete line range
				msg = fmt.Sprintf("decided to delete code lines, from start_line[%v] to end_line:[%v]", deleteStartLine, deleteEndLine)
				log.Infof("start to delete code lines %d to %d", deleteStartLine, deleteEndLine)
				deletedStart, deletedEnd = deleteStartLine, deleteEndLine
				err = editor.DeleteLineRange(deleteStartLine, deleteEndLine)
			} else {
				// Delete single line
				msg = fmt.Sprintf("decided to delete code line[%v]", deleteStartLine)
				log.Infof("start to delete code line %d", deleteStartLine)
				deletedStart, deletedEnd = deleteStartLine, deleteStartLine
				err = editor.DeleteLine(deleteStartLine)
			}
			oldSegment := loopInfraExtractLineRange(fullCode, deletedStart, deletedEnd)

			invoker.AddToTimeline("delete_lines", msg)
			loopInfraActionStart(loop, loopInfraNodeSingleFileDelete,
				fmt.Sprintf("删除文件行: %s / Delete file lines: %s", filename, filename),
				"删除文件内容 / Deleting File Content...")

			if err != nil {
				runtime.AddToTimeline("delete_failed", "Failed to delete lines: "+err.Error())
				op.Fail("failed to delete lines: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			var successMsg string
			if deleteEndLine > 0 {
				successMsg = fmt.Sprintf("SUCCESS: deleted lines[%d-%d], wrote %d bytes to file: %s", deleteStartLine, deleteEndLine, len(fullCode), filename)
			} else {
				successMsg = fmt.Sprintf("SUCCESS: deleted line[%d], wrote %d bytes to file: %s", deleteStartLine, len(fullCode), filename)
			}
			writeErr := f.replaceLoopFileContent(
				runtime, filename, fullCode,
				"delete_success", "delete_write_failed",
				successMsg,
			)
			if writeErr != nil {
				op.Fail(fmt.Sprintf("failed to write content after delete: %v", writeErr))
				return
			}
			loop.Set(fullCodeVar, fullCode)

			loopInfraAddFileOpSuccessTimeline(loop, loopInfraFileOpTimeline{
				Op:         "delete",
				Filename:   filename,
				OldSegment: oldSegment,
				StartLine:  deletedStart,
				EndLine:    deletedEnd,
				Deferred:   f.ShouldDeferDiskWrite(),
			})

			// Call file changed callback
			errMsg, hasBlockingErrors := f.OnFileChanged(fullCode, op)
			// delete 操作不自动退出：AI 可能需要多次修改，由 AI 主动调用 finish 退出。
			runBlocked := f.applySyntaxLintResult(loop, op, hasBlockingErrors, false)

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
			log.Infof("delete_lines done: hasBlockingErrors=%v runBlocked=%v", hasBlockingErrors, runBlocked)
			if !hasBlockingErrors && !runBlocked {
				loopInfraStatus(loop, "文件删除完成 / File Delete Complete")
				loopInfraActionFinish(loop, loopInfraNodeSingleFileDelete,
					fmt.Sprintf("文件删除完成: %s / File Delete Complete: %s", filename, filename),
					msg)
			}
			loop.GetEmitter().EmitPinFilename(filename)
			_, _ = f.applyLoopYaklangCodeChange(loop, &loopYaklangCodeChange{
				Content:       fullCode,
				Path:          filename,
				SourceAction:  actionName,
				EventOp:       loopYaklangCodeEventOpReplace,
				EmitEvent:     true,
				DeliveryPatch: BuildYaklangPatchDelete(deletedStart, deletedEnd, oldSegment, loop.GetInt(LoopVarCodeLineBase)),
			})

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
