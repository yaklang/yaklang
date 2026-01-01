package loop_report_generating

import (
	"os"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// writeReportAction creates an action for writing the initial report
var writeReportAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"write_report",
		"Create the initial report content. Use this ONLY when the report file is empty. If the report already has content, use modify_section, insert_section, or delete_section instead. The content should be written using the <|GEN_REPORT_...|> and <|GEN_REPORT_END_...|> AI tag pair.",
		nil, // No additional parameters needed, content comes from AI tag
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			// 检查报告是否为空
			existingContent := loop.Get("full_report")
			if existingContent != "" {
				return utils.Error("report already has content. Use 'modify_section', 'insert_section', or 'delete_section' to modify existing content")
			}

			log.Infof("write_report: verifying report is empty for initial write")
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filename := loop.Get("filename")
			if filename == "" {
				filename = r.EmitFileArtifactWithExt("report", ".md", "")
				loop.Set("filename", filename)
			}

			log.Infof("write_report: writing initial report to file %s", filename)

			// 等待 stream 完成，确保 AI 生成的内容已经被完全接收
			log.Infof("write_report: calling WaitStream...")
			action.WaitStream(op.GetContext())
			log.Infof("write_report: WaitStream completed")

			invoker := loop.GetInvoker()

			// 获取 AI 生成的报告内容
			reportContent := loop.Get("report_content")
			log.Infof("write_report: extracted report_content length=%d, preview: %s", len(reportContent), utils.ShrinkTextBlock(reportContent, 200))
			if reportContent == "" {
				r.AddToTimeline("error", "No report content generated in write_report action. The AI must use <|GEN_REPORT_xxx|> and <|GEN_REPORT_END_xxx|> tags to wrap the content.")
				op.Fail("No report content generated. Please use <|GEN_REPORT_...|> and <|GEN_REPORT_END_...|> tags to wrap your report content. Do NOT use markdown format only.")
				return
			}

			// 保存到 loop 上下文
			loop.Set("full_report", reportContent)

			// 写入文件
			err := os.WriteFile(filename, []byte(reportContent), 0644)
			if err != nil {
				r.AddToTimeline("error", "Failed to write report to file: "+err.Error())
				op.Fail(err)
				return
			}

			// 添加到时间线
			invoker.AddToTimeline("report_created", utils.ShrinkTextBlock(reportContent, 500))

			// 发送编辑器事件
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_YAKLANG_CODE_EDITOR, "write_report", reportContent)

			log.Infof("write_report: initial report created, size=%d bytes", len(reportContent))

			op.Feedback("Report created successfully. Content preview:\n" + utils.ShrinkTextBlock(reportContent, 300))
		},
	)
}
