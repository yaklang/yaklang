package loop_python_poc

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const pythonEnvNotAvailableComment = `# ============================================================
# WARNING: Python environment was not available during generation.
# Syntax validation could not be performed.
# Please verify this script manually before execution.
# ============================================================

`

var writePythonPOC = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"write_python_poc",
		"IMPORTANT: You MUST call 'check_python_env' FIRST before using this action. If there is NO CODE, use 'write_python_poc' to create a new Python POC file. If there is already code, use 'modify_python_poc' instead. You MUST wrap your Python code with <|GEN_PYTHON_POC_xxx|> and <|GEN_PYTHON_POC_END_xxx|> tags, do NOT use markdown code blocks.",
		nil,
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			// Check if environment has been checked
			envChecked := l.Get("env_checked")
			if envChecked != "true" {
				return utils.Error("You must call 'check_python_env' action first to check Python environment before generating code.")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				filename = r.EmitFileArtifactWithExt("python_poc", ".py", "")
				loop.Set("filename", filename)
			}

			// Wait for stream to complete - this is critical for AI tag extraction
			action.WaitStream(operator.GetContext())

			log.Infof("loop python poc start to exec writing code to file %s", filename)
			invoker := loop.GetInvoker()

			invoker.AddToTimeline("initialize", "AI decided to initialize the Python POC file: "+filename)

			// Get code from AI tag field (configured via WithAITagFieldWithAINodeId)
			code := loop.Get("python_poc_code")
			log.Infof("write_python_poc: extracted python_poc_code length=%d", len(code))

			if code == "" {
				r.AddToTimeline("error", "No code generated in write_python_poc action. The AI must use <|GEN_PYTHON_POC_xxx|> and <|GEN_PYTHON_POC_END_xxx|> tags to wrap the code.")
				operator.Fail("No code generated in 'write_python_poc' action. Please use <|GEN_PYTHON_POC_...|> and <|GEN_PYTHON_POC_END_...|> tags to wrap your code. Do NOT use markdown code blocks (```python).")
				return
			}

			// Check Python availability
			pythonAvailable := loop.Get("python_available") == "true"

			// If Python is not available, add warning comment to the code
			if !pythonAvailable {
				log.Warnf("Python environment not available, adding warning comment to code")
				// Only add comment if it doesn't already exist
				if !strings.Contains(code, "Python environment was not available") {
					code = pythonEnvNotAvailableComment + code
				}
			}

			// Store the full code
			loop.Set("full_code", code)

			// Write to file
			err := os.WriteFile(filename, []byte(code), 0644)
			if err != nil {
				r.AddToTimeline("error", "Failed to write code to file: "+err.Error())
				operator.Fail(err)
				return
			}

			msg := utils.ShrinkTextBlock(code, 256)
			msg += "\n\n文件已保存到: " + filename

			r.AddToTimeline("initial-python-poc", msg)
			log.Infof("write_python_poc done, file saved to: %s", filename)

			// Store the complete Python POC code in timeline for report generation
			r.AddToTimeline("python_poc_code_final", "```python\n"+code+"\n```")
			r.AddToTimeline("python_poc_file_path", filename)

			// Emit event for frontend
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "write_python_poc", map[string]any{
				"filename":         filename,
				"code":             code,
				"success":          true,
				"python_available": pythonAvailable,
			})

			// Pin the file for easy access
			loop.GetEmitter().EmitPinFilename(filename)

			// Handle based on Python availability
			if pythonAvailable {
				// Mark syntax as not yet verified - AI must verify before exiting
				loop.Set("syntax_verified", "false")

				// Disallow exit until syntax is verified
				operator.DisallowNextLoopExit()

				pythonCommand := loop.Get("python_command")
				operator.Feedback("代码已保存到文件: " + filename + "\n\nPython 环境可用 (" + pythonCommand + ")。你需要使用 bash 工具验证代码语法，然后调用 verify_syntax 确认。")
			} else {
				// Python not available, mark as verified (skipped) and allow exit
				loop.Set("syntax_verified", "skipped")
				log.Warnf("Python not available, syntax verification skipped for: %s", filename)
				r.AddToTimeline("syntax_check_skipped", "Python 环境不可用，语法检查已跳过。代码已添加警告注释。")

				operator.Feedback("代码已保存到文件: " + filename + "\n\n⚠️ Python 环境不可用，无法进行语法检查。代码已添加警告注释。你可以使用 directly_answer 完成任务。")
			}
		},
	)
}
