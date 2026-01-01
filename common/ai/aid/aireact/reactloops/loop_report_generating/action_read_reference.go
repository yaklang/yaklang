package loop_report_generating

import (
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// readReferenceFileAction creates an action for reading reference files
var readReferenceFileAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"read_reference_file",
		"Read the content of a reference file. Use this to collect information before writing the report. You can optionally specify line range to read partial content.",
		[]aitool.ToolOption{
			aitool.WithStringParam("file_path", aitool.WithParam_Description("Path to the reference file to read"), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("start_line", aitool.WithParam_Description("Optional: start line number (1-based). If not specified, reads from the beginning.")),
			aitool.WithIntegerParam("end_line", aitool.WithParam_Description("Optional: end line number (1-based). If not specified, reads to the end.")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			filePath := action.GetString("file_path")
			if filePath == "" {
				return utils.Error("file_path is required")
			}

			// 检查文件是否存在
			if !utils.FileExists(filePath) {
				return utils.Errorf("file not found: %s", filePath)
			}

			log.Infof("read_reference_file: verifying file %s", filePath)
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			filePath := action.GetString("file_path")
			startLine := action.GetInt("start_line")
			endLine := action.GetInt("end_line")

			log.Infof("read_reference_file: reading file %s (lines %d-%d)", filePath, startLine, endLine)

			// 读取文件内容
			content, err := os.ReadFile(filePath)
			if err != nil {
				log.Errorf("read_reference_file: failed to read file: %v", err)
				op.Fail(fmt.Sprintf("failed to read file: %v", err))
				return
			}

			var resultContent string
			lines := strings.Split(string(content), "\n")

			// 处理行范围
			if startLine > 0 || endLine > 0 {
				if startLine < 1 {
					startLine = 1
				}
				if endLine < 1 || endLine > len(lines) {
					endLine = len(lines)
				}
				if startLine > len(lines) {
					startLine = len(lines)
				}
				if startLine > endLine {
					startLine = endLine
				}

				selectedLines := lines[startLine-1 : endLine]
				resultContent = strings.Join(selectedLines, "\n")
				log.Infof("read_reference_file: read lines %d-%d, content size=%d bytes", startLine, endLine, len(resultContent))
			} else {
				resultContent = string(content)
				log.Infof("read_reference_file: read entire file, content size=%d bytes", len(resultContent))
			}

			// 限制内容大小，避免过大
			const maxContentSize = 50 * 1024 // 50KB
			if len(resultContent) > maxContentSize {
				resultContent = resultContent[:maxContentSize] + "\n\n[... content truncated, file too large ...]"
				log.Warnf("read_reference_file: content truncated to %d bytes", maxContentSize)
			}

			// 将读取的内容添加到已收集的参考资料中
			existingRefs := loop.Get("collected_references")
			newRef := fmt.Sprintf("\n=== Reference from: %s ===\n%s\n", filePath, resultContent)
			loop.Set("collected_references", existingRefs+newRef)

			// 添加到时间线
			invoker := loop.GetInvoker()
			invoker.AddToTimeline("reference_read", fmt.Sprintf("Read reference file: %s (%d bytes)", filePath, len(resultContent)))

			// 反馈结果
			summary := fmt.Sprintf("Successfully read file: %s\nContent size: %d bytes\nLines: %d", filePath, len(resultContent), len(lines))
			if len(resultContent) > 500 {
				summary += fmt.Sprintf("\n\nPreview:\n%s\n...", resultContent[:500])
			} else {
				summary += fmt.Sprintf("\n\nContent:\n%s", resultContent)
			}
			op.Feedback(summary)

			log.Infof("read_reference_file: completed, added to collected references")
		},
	)
}
