package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionModifyReport() coordinatorAction {
	options := []aitool.ToolOption{aitool.WithStringParam("document", aitool.WithParam_Description("完整报告正文，与document_patch互斥。")), aitool.WithStringParam("document_patch", aitool.WithParam_Description("仅针对coordinator-report.md的严格unified diff；不修改其他artifact，无fuzz。"))}
	return coordinatorAction{name: "modify_report", description: "原子修改唯一当前报告，正文覆盖或严格单文件diff互斥，保存草稿而不发布交付。", options: options, execute: func(c *Controller, l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
		r, err := c.ModifyReport(op.GetContext(), modifyArguments(a, options))
		if err == nil && r.Status != "unchanged" {
			l.GetEmitter().EmitPinFilename(c.Snapshot().Report.Path)
		}
		return r, err
	}}
}
