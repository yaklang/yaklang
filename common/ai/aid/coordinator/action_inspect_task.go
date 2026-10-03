package coordinator

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func actionInspectTask() coordinatorAction {
	return coordinatorAction{name: "inspect_task", description: "按需查看一个任务或明确指定的历史尝试；不验收、不标记已理解，不作为正常结果交接门禁。", options: []aitool.ToolOption{requiredString("task_id", "稳定任务ID。"), aitool.WithIntegerParam("attempt_id", aitool.WithParam_Description("可选历史attempt ID；省略时查看当前。")), aitool.WithBoolParam("details", aitool.WithParam_Description("默认有界状态与引用，true 请求完整实际结果。"))}, execute: func(c *Controller, l *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) (any, error) {
		return c.InspectTask(a.GetString("task_id"), uint64(a.GetInt("attempt_id")), a.GetBool("details"))
	}}
}
func (c *Controller) InspectTask(id string, attempt uint64, details bool) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.checkExecLocked(); err != nil {
		return nil, err
	}
	a, ok := c.state.Attempts[id]
	historical := false
	if attempt != 0 && (!ok || a.ID != attempt) {
		ok = false
		for _, previous := range c.state.History[id] {
			if previous.ID == attempt {
				a = previous
				ok = true
				historical = true
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("task/attempt not found")
	}
	r := clone(a.Result)
	reason := a.ReviewReason
	artifactCount, evidenceCount := len(r.Artifacts), len(r.EvidenceIDs)
	if !details {
		r.Summary = boundedText(r.Summary, 600)
		r.Error = boundedText(r.Error, 600)
		reason = boundedText(reason, 600)
		if len(r.Artifacts) > 16 {
			r.Artifacts = r.Artifacts[:16]
		}
		if len(r.EvidenceIDs) > 16 {
			r.EvidenceIDs = r.EvidenceIDs[:16]
		}
	}
	waiting := ""
	switch a.State {
	case Pending:
		waiting = "等待依赖验收或可用执行槽位"
	case Running:
		waiting = "本次冻结任务书正在执行"
	case Cancelling:
		waiting = "已请求取消，等待 worker 实际退出"
	case AwaitingReview:
		waiting = "等待协调员质量审核"
		if c.manualReview {
			waiting = "等待本任务人工审核"
		}
	case Failed, Rejected:
		waiting = "等待明确修复、调整、重试或取消"
	}
	reference := ""
	if c.resultConfig != nil && a.ID != 0 {
		reference, _ = taskResultEvidence(c.resultConfig, taskResultRecords([]Attempt{a})[0])
	}
	return map[string]any{"task_id": id, "attempt_id": a.ID, "state": a.State, "historical": historical, "waiting_reason": waiting, "review_reason": reason, "result": r, "artifact_count": artifactCount, "evidence_count": evidenceCount, "result_reference": reference}, nil
}
