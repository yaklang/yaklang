package coordinator

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func actionSubmitReport() coordinatorAction {
	return coordinatorAction{name: "submit_report", description: "发布实际保存的最新报告；宿主复查任务、消息与用户要求后才能正常结束，不额外请求报告审批。", options: []aitool.ToolOption{requiredString("summary", "供界面展示的交付摘要。")}, execute: func(c *Controller, l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
		if len(aicommon.GetBlockingVerificationTodoItems(l.GetConfig(), op.GetTask())) > 0 {
			return nil, fmt.Errorf("协调员仍有未处理的微观 TODO，不能提交报告")
		}
		user, _ := l.GetVariable("coordinator_observed_user_revision").(uint64)
		messages, _ := l.GetVariable("coordinator_message_cursor").(uint64)
		r, err := c.SubmitReport(user, messages)
		if err != nil {
			return nil, err
		}
		c.publish()
		receipt := map[string]any{"report_path": r.Path, "title": r.Title, "summary_markdown": a.GetString("summary")}
		l.GetEmitter().EmitJSON(schema.EVENT_TYPE_REPORT_FINISH, "report-finish", receipt)
		return receipt, nil
	}}
}
