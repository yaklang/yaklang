package reactloops

import "github.com/yaklang/yaklang/common/ai/aid/aicommon"

// This signal belongs to the loop, never to a returned action operator. A
// background review cannot generate a concurrent answer or exit another agent.
func (r *ReActLoop) AsyncDirectAnswerSignal() <-chan struct{} {
	r.onReleaseMutex.Lock()
	defer r.onReleaseMutex.Unlock()
	if r.asyncDirectAnswer == nil {
		r.asyncDirectAnswer = make(chan struct{})
	}
	return r.asyncDirectAnswer
}
func (r *ReActLoop) RequestAsyncDirectAnswer() {
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
	manager := r.asyncExecutionManager
	r.onReleaseMutex.Unlock()
	if manager != nil {
		manager.Close()
	}
}
func (r *ReActLoop) asyncReviewRequiresAnswer() bool {
	select {
	case <-r.AsyncDirectAnswerSignal():
		return true
	default:
		return false
	}
}
func (r *ReActLoop) answerAsyncReview(task aicommon.AIStatefulTask) error {
	r.StopSubAgentsForUser()
	r.onReleaseMutex.Lock()
	manager := r.asyncExecutionManager
	r.onReleaseMutex.Unlock()
	if manager != nil {
		manager.Close()
	}
	answer, err := r.invoker.DirectlyAnswer(task.GetContext(), "用户在后台工具调用审批中要求直接回答。停止继续调用工具，根据已有信息回答用户。", nil)
	if err == nil {
		r.invoker.AddToTimeline("directly-answer", answer)
		// This host-directed answer bypasses execOneCall. Record its delivery
		// so ordinary finalizers do not see the preceding tool action and
		// generate a second answer. It executes no tool callbacks.
		r.actionHistoryMutex.Lock()
		r.actionHistory = append(r.actionHistory, &ActionRecord{
			ActionType:     loopAction_DirectlyAnswer.ActionType,
			ActionName:     loopAction_DirectlyAnswer.ActionType,
			ActionParams:   map[string]interface{}{"answer_payload": answer},
			IterationIndex: r.currentIterationIndex,
		})
		r.actionHistoryMutex.Unlock()
	}
	return err
}
