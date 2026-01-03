package loop_report_generating

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// insertSectionAction creates an action for inserting new content into the report
var insertSectionAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"insert_section",
		"Insert new content after the specified line number. Use this to add new sections or paragraphs to the report. The new content should be written using the <|GEN_REPORT_...|> and <|GEN_REPORT_END_...|> AI tag pair.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("after_line", aitool.WithParam_Description("The line number after which to insert new content (1-based). Use 0 to insert at the beginning of the file."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("insert_reason", aitool.WithParam_Description("Reason for inserting this content")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "insert_reason",
				AINodeId:  "re-act-loop-thought",
			},
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			afterLine := action.GetInt("after_line")

			if afterLine < 0 {
				return utils.Error("after_line must be >= 0 (use 0 to insert at the beginning)")
			}

			loop.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing to insert content after line %v", afterLine))),
				loop.GetCurrentTask().GetIndex(),
			)

			log.Infof("insert_section: verifying insert position at line %d", afterLine)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for insert_section action")
				return
			}

			// 等待 stream 完成，确保 AI 生成的内容已经被完全接收
			action.WaitStream(op.GetContext())

			invoker := loop.GetInvoker()

			fullReport := loop.Get("full_report")
			newContent := loop.Get("report_content")
			afterLine := action.GetInt("after_line")
			reason := action.GetString("insert_reason")

			log.Infof("insert_section: inserting content after line %d in %s", afterLine, filename)

			if newContent == "" {
				op.Fail("No new content provided. Please use the <|GEN_REPORT_...|> and <|GEN_REPORT_END_...|> tag pair to provide content to insert.")
				return
			}

			// 分割现有内容为行
			lines := strings.Split(fullReport, "\n")

			// 验证行号
			if afterLine > len(lines) {
				afterLine = len(lines)
				log.Warnf("insert_section: adjusted after_line to %d (end of file)", afterLine)
			}

			// 构建新内容
			var result strings.Builder

			if afterLine == 0 {
				// 插入到开头
				result.WriteString(newContent)
				if !strings.HasSuffix(newContent, "\n") {
					result.WriteString("\n")
				}
				result.WriteString(fullReport)
			} else {
				// 插入到指定行之后
				for i, line := range lines {
					result.WriteString(line)
					if i < len(lines)-1 || strings.HasSuffix(fullReport, "\n") {
						result.WriteString("\n")
					}

					if i+1 == afterLine {
						// 在这一行之后插入新内容
						if !strings.HasSuffix(result.String(), "\n") {
							result.WriteString("\n")
						}
						result.WriteString(newContent)
						if !strings.HasSuffix(newContent, "\n") && i < len(lines)-1 {
							result.WriteString("\n")
						}
					}
				}
			}

			fullReport = result.String()

			msg := fmt.Sprintf("decided to insert content after line[%v]", afterLine)
			invoker.AddToTimeline("insert_section", msg)

			if reason != "" {
				r.AddToTimeline("insert_reason", reason)
			}

			// 更新 loop 上下文
			loop.Set("full_report", fullReport)

			// 保存到文件
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullReport), 0644)

			// 发送文件产物事件（确保 CI/客户端能正确获取更新后的文件）
			loop.GetEmitter().EmitPinFilename(filename)

			// 构建反馈消息
			newLines := len(strings.Split(newContent, "\n"))
			msg = fmt.Sprintf("Inserted %d lines after line %d:\n%s", newLines, afterLine, utils.ShrinkTextBlock(newContent, 256))
			r.AddToTimeline("section_inserted", msg)

			// 发送编辑器事件
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "insert_section", newContent)

			log.Infof("insert_section: completed, inserted %d lines after line %d", newLines, afterLine)

			op.Feedback(fmt.Sprintf("Content inserted successfully after line %d.\n%d new lines added.\nNew content preview:\n%s",
				afterLine, newLines, utils.ShrinkTextBlock(newContent, 200)))
		},
	)
}
