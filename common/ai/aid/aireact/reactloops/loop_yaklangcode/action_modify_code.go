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

var modifyCode = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"modify_code",
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
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for modify_code action")
				return
			}

			if loop.GetInt("modify_attempts") >= 3 {
				op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
			}

			invoker := loop.GetInvoker()

			fullCode := loop.Get("full_code")
			partialCode := loop.Get("yak_code")
			editor := memedit.NewMemEditor(fullCode)
			modifyStartLine := action.GetInt("modify_start_line")
			modifyEndLine := action.GetInt("modify_end_line")

			msg := fmt.Sprintf("decided to modify code file, from start_line[%v] to end_line:[%v]", modifyStartLine, modifyEndLine)
			invoker.AddToTimeline("modify_code", msg)

			reason := action.GetString("modify_code_reason")
			if reason != "" {
				r.AddToTimeline("modify_reason", reason)
			}

			start, end, codeSegment, fixedCode := prettifyAITagCode(partialCode)
			if fixedCode {
				if start == modifyStartLine && end == modifyEndLine {
					log.Infof("use prettified code segment for 'modify_code' action, fix range %d to %d", start, end)
					partialCode = codeSegment
				} else {
					r.AddToTimeline("modify_warning", fmt.Sprintf("The code segment line numbers [%v-%v] do not match the specified modify line numbers [%v-%v]. Using the original code segment.", start, end, modifyStartLine, modifyEndLine))
					op.Continue()
					return
				}
			}

			fmt.Println("=================================================")
			fmt.Println(string(partialCode))
			fmt.Println("=================================================")

			log.Infof("start to modify code lines %d to %d", modifyStartLine, modifyEndLine)
			err := editor.ReplaceLineRange(modifyStartLine, modifyEndLine, partialCode)
			if err != nil {
				r.AddToTimeline("modify_failed", "Failed to replace line range: "+err.Error())
				//return filename, utils.Errorf("Failed to replace line range: %v", err)
				op.Fail("failed to replace line range: " + err.Error())
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

			// 检测防空转：连续在同一区域小幅修改
			modifyRecord := ModifyRecord{
				StartLine: modifyStartLine,
				EndLine:   modifyEndLine,
			}
			isSpinning, spinReason := detectSpinning(loop, modifyRecord)
			if isSpinning {
				// 触发防空转机制
				reflectionPrompt := generateReflectionPrompt(modifyRecord, spinReason)
				op.SetReflectionLevel(reactloops.ReflectionLevel_Deep)
				op.Feedback(reflectionPrompt)
				invoker.AddToTimeline("spinning_detected", spinReason)
				log.Warnf("spinning detected in modify_code: %s", spinReason)
			}

			msg = utils.ShrinkTextBlock(fmt.Sprintf("line[%v-%v]:\n", modifyStartLine, modifyEndLine)+partialCode, 256)
			if errMsg != "" {
				msg += "\n\n--[linter]--\nWriting Code Linter Check:\n" + utils.PrefixLines(utils.ShrinkTextBlock(errMsg, 2048), "  ")
				if !isSpinning {
					// 只在没有触发防空转时才反馈错误信息，避免重复
					op.Feedback(errMsg)
				}
			} else {
				msg += "\n\n--[linter]--\nNo issues found in the modified code segment."
			}
			r.AddToTimeline("code_modified", msg)
			log.Infof("modify_code done: hasBlockingErrors=%v, will show errors in next iteration", hasBlockingErrors)
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "modify_code", partialCode)

			if errMsg != "" && !isSpinning {
				invoker.AddToTimeline("advice", "use 'grep_yaklang_samples' to find more syntax sample or docs")
			}
		},
	)
}
