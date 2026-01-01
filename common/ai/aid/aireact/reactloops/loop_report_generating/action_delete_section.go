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

// deleteSectionAction creates an action for deleting sections from the report
var deleteSectionAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"delete_section",
		"Delete content between the specified line numbers (inclusive). Use this to remove unwanted or redundant sections from the report. Line numbers are 1-based.",
		[]aitool.ToolOption{
			aitool.WithIntegerParam("delete_start_line", aitool.WithParam_Description("The starting line number to delete (1-based, inclusive)"), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("delete_end_line", aitool.WithParam_Description("The ending line number to delete (1-based, inclusive)"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("delete_reason", aitool.WithParam_Description("Reason for deleting this content")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			startLine := action.GetInt("delete_start_line")
			endLine := action.GetInt("delete_end_line")

			if startLine <= 0 || endLine <= 0 || endLine < startLine {
				return utils.Error("delete_section action must have valid 'delete_start_line' and 'delete_end_line' parameters (1-based, start <= end)")
			}

			loop.GetEmitter().EmitDefaultStreamEvent(
				"thought",
				bytes.NewReader([]byte(fmt.Sprintf("Preparing to delete lines %v-%v", startLine, endLine))),
				loop.GetCurrentTask().GetIndex(),
			)

			log.Infof("delete_section: verifying line range %d-%d for deletion", startLine, endLine)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				op.Fail("no filename found in loop context for delete_section action")
				return
			}

			invoker := loop.GetInvoker()

			fullReport := loop.Get("full_report")
			startLine := action.GetInt("delete_start_line")
			endLine := action.GetInt("delete_end_line")
			reason := action.GetString("delete_reason")

			log.Infof("delete_section: deleting lines %d-%d from %s", startLine, endLine, filename)

			// 分割内容为行
			lines := strings.Split(fullReport, "\n")

			// 验证行号范围
			if startLine > len(lines) {
				op.Fail(fmt.Sprintf("delete_start_line %d exceeds total lines %d", startLine, len(lines)))
				return
			}
			if endLine > len(lines) {
				endLine = len(lines)
				log.Warnf("delete_section: adjusted delete_end_line to %d (end of file)", endLine)
			}

			// 记录被删除的内容
			deletedLines := lines[startLine-1 : endLine]
			deletedContent := strings.Join(deletedLines, "\n")

			// 构建新内容（排除被删除的行）
			var result strings.Builder
			for i, line := range lines {
				lineNum := i + 1
				if lineNum < startLine || lineNum > endLine {
					result.WriteString(line)
					if i < len(lines)-1 {
						result.WriteString("\n")
					}
				}
			}

			fullReport = result.String()

			msg := fmt.Sprintf("decided to delete lines [%v-%v]", startLine, endLine)
			invoker.AddToTimeline("delete_section", msg)

			if reason != "" {
				r.AddToTimeline("delete_reason", reason)
			}

			r.AddToTimeline("deleted_content", utils.ShrinkTextBlock(deletedContent, 200))

			// 更新 loop 上下文
			loop.Set("full_report", fullReport)

			// 保存到文件
			os.RemoveAll(filename)
			os.WriteFile(filename, []byte(fullReport), 0644)

			// 发送编辑器事件
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "delete_section", deletedContent)

			log.Infof("delete_section: completed, deleted lines %d-%d (%d lines)", startLine, endLine, len(deletedLines))

			op.Feedback(fmt.Sprintf("Content deleted successfully. Lines %d-%d removed (%d lines).\nDeleted content preview:\n%s",
				startLine, endLine, len(deletedLines), utils.ShrinkTextBlock(deletedContent, 200)))
		},
	)
}
