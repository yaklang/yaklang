package coordinator

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func actionWriteReport() coordinatorAction {
	return coordinatorAction{
		name: "write_report", description: "在当前协调循环写入 Markdown 报告产物，并发布兼容的 report_finish 事件。", options: []aitool.ToolOption{requiredString("title", "报告标题。"), requiredString("markdown", "完整报告正文。"), requiredString("summary", "供现有界面展示的报告摘要。")},
		execute: func(c *Controller, loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
			if strings.TrimSpace(a.GetString("markdown")) == "" {
				return nil, fmt.Errorf("report content is empty")
			}
			path := loop.GetInvoker().EmitFileArtifactWithExt("coordinator-report", ".md", a.GetString("markdown"))
			if path == "" {
				return nil, fmt.Errorf("report artifact could not be written")
			}
			loop.Set("coordinator_report_path", path)
			receipt := map[string]any{"report_path": path, "title": a.GetString("title"), "summary_markdown": a.GetString("summary")}
			loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, "report-finish", receipt)
			return receipt, nil
		},
	}
}
