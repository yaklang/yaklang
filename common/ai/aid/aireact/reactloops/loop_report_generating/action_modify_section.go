package loop_report_generating

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

// modifySectionAction creates an action for modifying report sections
var modifySectionAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"modify_section",
		"Modify the content between the specified line numbers (inclusive). Line numbers are 1-based. Use this to update or rewrite existing sections of the report. The new content should be written using the <|GEN_REPORT_...|> and <|GEN_REPORT_END_...|> AI tag pair.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("modify_start_line", aitool.WithParam_Description("The starting line number (1-based, inclusive)"), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("modify_end_line", aitool.WithParam_Description("The ending line number (1-based, inclusive)"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("modify_reason", aitool.WithParam_Description("Reason for the modification")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "modify_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			startLine := action.GetInt("modify_start_line")
			endLine := action.GetInt("modify_end_line")

			if startLine <= 0 || endLine <= 0 || endLine < startLine {
				return utils.Error("modify_section action must have valid 'modify_start_line' and 'modify_end_line' parameters (1-based, start <= end)")
			}

			loop.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing to modify lines %v-%v", startLine, endLine))),
				loop.GetCurrentTask().GetIndex(),
			)

			log.Infof("modify_section: verifying line range %d-%d", startLine, endLine)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for modify_section action")
				return
			}

			// 等待 stream 完成，确保 AI 生成的内容已经被完全接收
			action.WaitStream(op.GetContext())

			invoker := loop.GetInvoker()

			fullReport := loop.Get("full_report")
			newContent := loop.Get("report_content")
			startLine := action.GetInt("modify_start_line")
			endLine := action.GetInt("modify_end_line")
			reason := action.GetString("modify_reason")

			log.Infof("modify_section: modifying lines %d-%d in %s", startLine, endLine, filename)

			if newContent == "" {
				op.Fail("No new content provided. Please use the <|GEN_REPORT_...|> and <|GEN_REPORT_END_...|> tag pair to provide replacement content.")
				return
			}

			// 使用 memedit 进行编辑
			editor := memedit.NewMemEditor(fullReport)

			msg := fmt.Sprintf("decided to modify report, from start_line[%v] to end_line:[%v]", startLine, endLine)
			invoker.AddToTimeline("modify_section", msg)

			if reason != "" {
				r.AddToTimeline("modify_reason", reason)
			}

			// 替换行范围
			err := editor.ReplaceLineRange(startLine, endLine, newContent)
			if err != nil {
				r.AddToTimeline("modify_failed", "Failed to replace line range: "+err.Error())
				op.Fail("failed to replace line range: " + err.Error())
				return
			}

			fullReport = editor.GetSourceCode()
			loop.Set("full_report", fullReport)

			// 保存到文件
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullReport), 0644)

			// 发送文件产物事件（确保 CI/客户端能正确获取更新后的文件）
			loop.GetEmitter().EmitPinFilename(filename)

			// 构建反馈消息
			msg = fmt.Sprintf("Modified lines [%v-%v]:\n%s", startLine, endLine, utils.ShrinkTextBlock(newContent, 256))
			r.AddToTimeline("section_modified", msg)

			// 发送编辑器事件
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "modify_section", newContent)

			log.Infof("modify_section: completed, modified lines %d-%d", startLine, endLine)

			op.Feedback(fmt.Sprintf("Section modified successfully. Lines %d-%d replaced.\nNew content preview:\n%s",
				startLine, endLine, utils.ShrinkTextBlock(newContent, 200)))
		},
	)
}
