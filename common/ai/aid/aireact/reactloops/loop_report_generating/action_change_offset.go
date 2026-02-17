package loop_report_generating

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// changeOffsetLineAction 切换报告视图偏移，用于查看大型报告的不同部分
var changeOffsetLineAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"change_view_offset",
		`Change the viewing offset for the report content. Use this when the report is large and you need to view different sections.

Common operations:
- View beginning: offset_line=1
- View next section: offset_line = current_end_line + 1
- View previous section: offset_line = current_offset_line - lines_per_page
- Jump to specific line: offset_line = target_line_number`,
		[]aitool.ToolOption{
			aitool.WithIntegerParam("offset_line", aitool.WithParam_Description("Line number to start viewing from (1-based)"), aitool.WithParam_Required(true)),
			aitool.WithIntegerParam("show_size", aitool.WithParam_Description("Maximum characters to display (default: 30000)")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			reportContent := loop.Get("full_report_code")
			if reportContent == "" {
				return utils.Error("no report content to view. Please create a report first using write_section")
			}

			offsetLine := action.GetInt("offset_line")
			if offsetLine < 1 {
				return utils.Errorf("offset_line must be >= 1, got %d", offsetLine)
			}

			totalLines := len(strings.Split(reportContent, "\n"))
			if offsetLine > totalLines {
				return utils.Errorf("offset_line %d exceeds total lines %d", offsetLine, totalLines)
			}

			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			offsetLine := action.GetInt("offset_line")
			showSize := action.GetInt("show_size")

			if showSize <= 0 {
				showSize = DefaultReportShowSize
			}

			// 更新 loop 状态
			loop.Set("offset_line", strconv.Itoa(offsetLine))
			loop.Set("report_show_size", strconv.Itoa(showSize))

			reportContent := loop.Get("full_report_code")
			lines := strings.Split(reportContent, "\n")
			totalLines := len(lines)

			log.Infof("change_view_offset: offset_line=%d, show_size=%d, total_lines=%d", offsetLine, showSize, totalLines)

			// 计算可见范围信息
			var visibleEndLine int
			var currentSize int
			for i := offsetLine - 1; i < totalLines && currentSize < showSize; i++ {
				lineSize := len(lines[i]) + 1
				if currentSize+lineSize > showSize && i > offsetLine-1 {
					break
				}
				currentSize += lineSize
				visibleEndLine = i + 1
			}

			hasMore := visibleEndLine < totalLines
			hasPrev := offsetLine > 1

			feedback := fmt.Sprintf("View offset changed to line %d. Now showing lines %d-%d of %d total lines (%d/%d bytes)",
				offsetLine, offsetLine, visibleEndLine, totalLines, currentSize, len(reportContent))

			if hasMore {
				feedback = fmt.Sprintf("%s. There is more content below (use offset_line=%d to continue).",
					feedback, visibleEndLine+1)
			}
			if hasPrev {
				feedback = fmt.Sprintf("%s. There is content above (use offset_line=1 to go back to beginning).",
					feedback)
			}

			op.Feedback(feedback)
		},
	)
}
