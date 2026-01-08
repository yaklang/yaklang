package loop_python_poc

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

var modifyPythonPOC = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"modify_python_poc",
		"Do NOT use this action to create new code file, ONLY use it to modify existing Python POC code. Modify the code between the specified line numbers (inclusive). The line numbers are 1-based, meaning the first line of the file is line 1. Ensure that 'modify_start_line' is less than or equal to 'modify_end_line'. IMPORTANT: You MUST wrap your Python code with <|GEN_PYTHON_POC_xxx|> and <|GEN_PYTHON_POC_END_xxx|> tags.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("modify_start_line"),
			aitool.WithIntegerParam("modify_end_line"),
			aitool.WithStringParam("modify_code_reason", aitool.WithParam_Description(`Brief explanation of why this modification is needed, e.g., fix syntax errors, add features, improve logic`)),
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
				return utils.Error("modify_python_poc action must have valid 'modify_start_line' and 'modify_end_line' parameters")
			}
			l.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing modify Python POC line:%v-%v", start, end))), l.GetCurrentTask().GetIndex())
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("No Python POC file found. Please use 'write_python_poc' first to create a file.")
				return
			}

			// Wait for stream to complete - critical for AI tag extraction
			action.WaitStream(op.GetContext())

			invoker := loop.GetInvoker()

			fullCode := loop.Get("full_code")
			if fullCode == "" {
				// Try to read from file
				content, err := os.ReadFile(filename)
				if err != nil {
					op.Fail(fmt.Sprintf("Failed to read file %s: %v", filename, err))
					return
				}
				fullCode = string(content)
				loop.Set("full_code", fullCode)
			}

			// Get the new code from AI tag field
			partialCode := loop.Get("python_poc_code")
			if partialCode == "" {
				r.AddToTimeline("error", "No code provided in modify_python_poc action. The AI must use <|GEN_PYTHON_POC_xxx|> and <|GEN_PYTHON_POC_END_xxx|> tags to wrap the code.")
				op.Fail("No code provided. Please use <|GEN_PYTHON_POC_...|> and <|GEN_PYTHON_POC_END_...|> tags to wrap your code. Do NOT use markdown code blocks.")
				return
			}

			modifyStartLine := action.GetInt("modify_start_line")
			modifyEndLine := action.GetInt("modify_end_line")

			msg := fmt.Sprintf("decided to modify Python POC code, from start_line[%v] to end_line:[%v]", modifyStartLine, modifyEndLine)
			invoker.AddToTimeline("modify_python_poc", msg)

			reason := action.GetString("modify_code_reason")
			if reason != "" {
				r.AddToTimeline("modify_reason", reason)
			}

			// Use memedit for line-based editing
			editor := memedit.NewMemEditor(fullCode)

			log.Infof("start to modify Python POC code lines %d to %d", modifyStartLine, modifyEndLine)
			err := editor.ReplaceLineRange(modifyStartLine, modifyEndLine, partialCode)
			if err != nil {
				r.AddToTimeline("modify_failed", "Failed to replace line range: "+err.Error())
				op.Fail("Failed to replace line range: " + err.Error())
				return
			}

			fullCode = editor.GetSourceCode()
			loop.Set("full_code", fullCode)

			// Write to file
			os.RemoveAll(filename)
			err = os.WriteFile(filename, []byte(fullCode), 0644)
			if err != nil {
				op.Fail(fmt.Sprintf("Failed to write modified code to file: %v", err))
				return
			}

			msg = utils.ShrinkTextBlock(fmt.Sprintf("line[%v-%v]:\n", modifyStartLine, modifyEndLine)+partialCode, 256)
			msg += "\n\n文件已更新: " + filename

			r.AddToTimeline("code_modified", msg)
			log.Infof("modify_python_poc done, file updated: %s", filename)

			// Update the complete Python POC code in timeline for report generation
			r.AddToTimeline("python_poc_code_final", "```python\n"+fullCode+"\n```")
			r.AddToTimeline("python_poc_file_path", filename)

			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "modify_python_poc", map[string]any{
				"filename": filename,
				"success":  true,
				"reason":   reason,
			})

			// Check Python availability
			pythonAvailable := loop.Get("python_available") == "true"

			if pythonAvailable {
				// Mark syntax as not yet verified - AI must verify before exiting
				loop.Set("syntax_verified", "false")

				// Disallow exit until syntax is verified
				op.DisallowNextLoopExit()

				pythonCommand := loop.Get("python_command")
				op.Feedback("代码已修改并保存到文件: " + filename + "\n\nPython 环境可用 (" + pythonCommand + ")。你需要使用 bash 工具验证代码语法，然后调用 verify_syntax 确认。")
			} else {
				// Python not available, mark as verified (skipped) and allow exit
				loop.Set("syntax_verified", "skipped")
				log.Warnf("Python not available, syntax verification skipped for modified code: %s", filename)
				r.AddToTimeline("syntax_check_skipped", "Python 环境不可用，修改后的代码语法检查已跳过。")

				op.Feedback("代码已修改并保存到文件: " + filename + "\n\n⚠️ Python 环境不可用，无法进行语法检查。你可以继续修改代码或使用 directly_answer 完成任务。")
			}
		},
	)
}
