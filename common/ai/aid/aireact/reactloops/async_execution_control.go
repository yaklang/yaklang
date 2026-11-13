package reactloops

import "github.com/yaklang/yaklang/common/utils"

const asyncDirectAnswerTimelineInstruction = `用户在后台工具调用审批中要求直接回答。

不要继续调用业务工具，也不要继续等待或执行后台任务。请依据当前 Timeline 中已经获得的信息，下一步使用 directly_answer 回答用户；已经取消或尚未完成的工作需要明确说明。`

// The ReAct runtime owns the direct-answer signal so it can wake the current or
// next task loop. A background review cannot exit another ReAct session.
func (r *ReActLoop) AsyncDirectAnswerSignal() <-chan struct{} {
	if r == nil || utils.IsNil(r.invoker) {
		return nil
	}
	return r.invoker.AsyncDirectAnswerSignal()
}
func (r *ReActLoop) RequestAsyncDirectAnswer() {
	if r == nil || utils.IsNil(r.invoker) {
		return
	}
	r.invoker.RequestAsyncDirectAnswer()
}

// handleAsyncDirectAnswerSignal 消费一次用户审批直答请求并写入 Timeline。
// 主循环只在一处调用；后续轮次通过正常 Prompt 让 AI 自行选择 directly_answer。
func (r *ReActLoop) handleAsyncDirectAnswerSignal() bool {
	if r == nil {
		return false
	}
	if utils.IsNil(r.invoker) {
		return false
	}
	if !r.invoker.ConsumeAsyncDirectAnswer() {
		return false
	}
	r.StopSubAgentsForUser()
	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline("async-direct-answer-request", asyncDirectAnswerTimelineInstruction)
	}
	return true
}
