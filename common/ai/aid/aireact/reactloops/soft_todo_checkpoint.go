package reactloops

import "strings"

const softTodoCheckpointPrompt = `[SOFT TODO CHECKPOINT]

你准备结束当前任务。

结束前快速检查 TODO：

- 是否仍有不能忽略的开放事项；
- 是否有 TODO 已解决但尚未记录结果；
- 未完成事项是否应明确标记为 deferred。

不要为了清空列表而伪造完成。
仅在确有必要时更新 TODO；否则再次结束任务。`

// requestSoftTodoCheckpoint returns true only after the checkpoint belonging
// to the current finish flow has already been requested.
func (r *ReActLoop) requestSoftTodoCheckpoint() bool {
	r.softTodoCheckpointMu.Lock()
	defer r.softTodoCheckpointMu.Unlock()
	if r.softTodoChecked {
		return true
	}
	r.softTodoChecked = true
	r.softTodoCheckpointPending = true
	return false
}

func (r *ReActLoop) consumeSoftTodoCheckpoint() string {
	r.softTodoCheckpointMu.Lock()
	defer r.softTodoCheckpointMu.Unlock()
	if !r.softTodoCheckpointPending {
		return ""
	}
	r.softTodoCheckpointPending = false
	return softTodoCheckpointPrompt
}

func (r *ReActLoop) resetSoftTodoFinishFlow() {
	r.softTodoCheckpointMu.Lock()
	r.softTodoChecked = false
	r.softTodoCheckpointPending = false
	r.softTodoCheckpointMu.Unlock()
}

func appendSoftTodoCheckpoint(reactiveData, checkpoint string) string {
	if strings.TrimSpace(checkpoint) == "" {
		return reactiveData
	}
	if strings.TrimSpace(reactiveData) == "" {
		return checkpoint
	}
	return strings.TrimRight(reactiveData, "\n") + "\n\n" + checkpoint
}
