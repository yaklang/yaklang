package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	stageSummaryRequestPendingKey      = "__stage_summary_request_pending__"
	directAnswerDelegationAllowedKey   = "__direct_answer_delegation_allowed__"
	TimelineEntryStageSummaryRequest   = "stage_summary_request"
	TimelineEntryStageSummaryDelivered = "stage_summary_delivered"
)

// SetDirectlyAnswerDelegationAllowed limits the compatibility bridge to the
// live decision/action phase and normal finish finalization. Hard-error and
// initialization finalizers continue to use the standalone fallback.
func (r *ReActLoop) SetDirectlyAnswerDelegationAllowed(allowed bool) {
	if r == nil {
		return
	}
	r.Set(directAnswerDelegationAllowedKey, allowed)
}

func (r *ReActLoop) IsDirectlyAnswerDelegationAllowed() bool {
	if r == nil {
		return false
	}
	return utils.InterfaceToBoolean(r.GetVariable(directAnswerDelegationAllowedKey))
}

func (r *ReActLoop) HasPendingStageSummaryRequest() bool {
	if r == nil {
		return false
	}
	return utils.InterfaceToBoolean(r.GetVariable(stageSummaryRequestPendingKey))
}

// RequestStageSummary records one bounded, action-oriented request in Timeline.
// Returning false means an earlier request is still pending; callers should use
// their compatibility fallback instead of repeatedly resuming the loop.
func (r *ReActLoop) RequestStageSummary(query, referenceMaterial string) bool {
	if r == nil || r.HasPendingStageSummaryRequest() {
		return false
	}
	r.Set(stageSummaryRequestPendingKey, true)

	var body strings.Builder
	body.WriteString("当前进行阶段性小总结。请回到主循环，根据 Timeline 中已经存在的事实和工具结果执行下一步：\n")
	body.WriteString("1. 使用主循环的 directly_answer Action 向用户输出当前结论，不要启动独立回答调用；\n")
	body.WriteString("2. 明确列出关键证据及其来源，不要把推测写成已验证事实；\n")
	body.WriteString("3. 标注哪些 TODO/子任务已经完成，并通过 next_movements 更新对应状态；\n")
	body.WriteString("4. 若仍有未完成事项，阶段总结后继续执行；若全部完成，再显式调用 finish。")
	if query = strings.TrimSpace(query); query != "" {
		body.WriteString("\n\n本次总结关注：\n")
		body.WriteString(utils.ShrinkTextBlock(query, 4*1024))
	}
	if referenceMaterial = strings.TrimSpace(referenceMaterial); referenceMaterial != "" {
		body.WriteString("\n\n本次总结补充证据材料：\n")
		body.WriteString(utils.ShrinkTextBlock(referenceMaterial, 12*1024))
	}
	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline(TimelineEntryStageSummaryRequest, body.String())
	}
	return true
}

// MarkStageSummaryDelivered is called by the normal directly_answer action path
// (including focus-loop overrides through DirectlyAnswerContinue).
func (r *ReActLoop) MarkStageSummaryDelivered() {
	if r == nil || !r.HasPendingStageSummaryRequest() {
		return
	}
	r.Set(stageSummaryRequestPendingKey, false)
	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline(TimelineEntryStageSummaryDelivered,
			"阶段性总结已由主循环 directly_answer Action 输出；继续执行剩余事项，或在 TODO 全部关闭后调用 finish。")
	}
}
