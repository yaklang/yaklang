package reactloops

const asyncDirectAnswerTimelineInstruction = `用户在后台工具调用审批中要求直接回答。

不要继续调用业务工具，也不要继续等待或执行后台任务。请依据当前 Timeline 中已经获得的信息，下一步使用 directly_answer 回答用户；已经取消或尚未完成的工作需要明确说明。`

// Session providers wake the current/next task loop; standalone loops retain a
// loop-local fallback. A background review cannot generate a concurrent answer
// or exit another ReAct session.
func (r *ReActLoop) AsyncDirectAnswerSignal() <-chan struct{} {
	if provider, ok := r.invoker.(interface {
		AsyncDirectAnswerSignal() <-chan struct{}
	}); ok {
		return provider.AsyncDirectAnswerSignal()
	}
	r.onReleaseMutex.Lock()
	defer r.onReleaseMutex.Unlock()
	if r.asyncDirectAnswer == nil {
		r.asyncDirectAnswer = make(chan struct{})
	}
	return r.asyncDirectAnswer
}
func (r *ReActLoop) RequestAsyncDirectAnswer() {
	if provider, ok := r.invoker.(interface {
		RequestAsyncDirectAnswer()
	}); ok {
		provider.RequestAsyncDirectAnswer()
		return
	}
	r.onReleaseMutex.Lock()
	if r.released || r.asyncDirectAnswerRequested {
		r.onReleaseMutex.Unlock()
		return
	}
	if r.asyncDirectAnswer == nil {
		r.asyncDirectAnswer = make(chan struct{})
	}
	r.asyncDirectAnswerRequested = true
	close(r.asyncDirectAnswer)
	r.onReleaseMutex.Unlock()
	manager := r.PeekAsyncExecutionManager()
	if manager != nil {
		manager.StopAll()
	}
}

// handleAsyncDirectAnswerSignal 消费一次用户审批直答请求并写入 Timeline。
// 主循环只在一处调用；后续轮次通过正常 Prompt 让 AI 自行选择 directly_answer。
func (r *ReActLoop) handleAsyncDirectAnswerSignal() bool {
	if r == nil {
		return false
	}
	if provider, ok := r.invoker.(interface {
		ConsumeAsyncDirectAnswer() bool
	}); ok {
		if !provider.ConsumeAsyncDirectAnswer() {
			return false
		}
		r.StopSubAgentsForUser()
		if invoker := r.GetInvoker(); invoker != nil {
			invoker.AddToTimeline("async-direct-answer-request", asyncDirectAnswerTimelineInstruction)
		}
		return true
	}
	r.onReleaseMutex.Lock()
	if !r.asyncDirectAnswerRequested || r.asyncDirectAnswerHandled {
		r.onReleaseMutex.Unlock()
		return false
	}
	r.asyncDirectAnswerHandled = true
	r.onReleaseMutex.Unlock()

	r.StopSubAgentsForUser()
	manager := r.PeekAsyncExecutionManager()
	if manager != nil {
		manager.StopAll()
	}
	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline("async-direct-answer-request", asyncDirectAnswerTimelineInstruction)
	}
	return true
}
