package coordinator

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionCreateReport() coordinatorAction {
	return coordinatorAction{name: "create_report", description: "任务及关键消息全部收尾后，创建 artifacts 中唯一 Markdown 报告草稿；尚未交付。", options: []aitool.ToolOption{requiredString("title", "报告标题。"), requiredString("document", "完整Markdown报告草稿：目标、方法、依据、调整、验收、失败重试、未完成范围及限制。")}, execute: func(c *Controller, l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
		r, err := c.CreateReport(op.GetContext(), a.GetString("title"), a.GetString("document"))
		if err == nil {
			l.GetEmitter().EmitPinFilename(c.Snapshot().Report.Path)
		}
		return r, err
	}}
}
