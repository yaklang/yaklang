package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const currentTodoCheckpointThreshold = 25

const softTodoCheckpointPrompt = `[SOFT TODO CHECKPOINT]

你准备结束当前任务。

结束前快速检查 TODO：

- 是否仍有不能忽略的开放事项；
- 是否有 TODO 已解决但尚未记录结果；
- 未完成事项是否应明确标记为 deferred。

不要为了清空列表而伪造完成。
仅在确有必要时更新 TODO；否则再次结束任务。`

const currentTodoCheckpointPrompt = `[CURRENT TODO CHECKPOINT]

当前 CURRENT TODO 已连续执行 25 个有效迭代。

在选择下一步前快速检查：

- 当前路径是否仍在产生可确认的新信息；
- 下一步是否具有与之前不同的明确验证目标；
- 当前 TODO 是否过大，需要拆分；
- 是否正在重复近似行动和近似观察；
- 是否应该继续、切换 TODO，或将当前事项标记为 deferred。

如果当前路径仍有明确进展，可以继续。
仅在必要时调整 TODO，不要为了响应检查而制造修改。
随后直接继续 ReAct。`

type currentTodoProgress struct {
	CurrentTodoID string
	Iterations    int
	Pending       bool
}

func todoCheckpointScopeKey(scope aicommon.VerificationTodoScope) string {
	return scope.TaskID + "\x00" + scope.TaskIndex
}

// requestSoftTodoCheckpoint returns true only after the checkpoint belonging
// to the current finish flow has already been requested. A finish checkpoint
// subsumes any pending CURRENT checkpoint for the same task scope.
func (r *ReActLoop) requestSoftTodoCheckpoint() bool {
	r.todoCheckpointMu.Lock()
	defer r.todoCheckpointMu.Unlock()
	if r.softTodoChecked {
		return true
	}
	r.softTodoChecked = true
	r.softTodoCheckpointPending = true
	key := todoCheckpointScopeKey(aicommon.BuildVerificationTodoScope(r.GetCurrentTask()))
	if progress := r.currentTodoProgress[key]; progress != nil {
		progress.Iterations = 0
		progress.Pending = false
	}
	return false
}

// consumeTodoCheckpoint returns at most one checkpoint. Finish has priority;
// otherwise a CURRENT checkpoint is emitted only while the same TODO remains
// current. Consumption restarts that TODO's 25-iteration window.
func (r *ReActLoop) consumeTodoCheckpoint() string {
	if r == nil || r.config == nil {
		return ""
	}
	scope := aicommon.BuildVerificationTodoScope(r.GetCurrentTask())
	_, current, _ := r.config.SnapshotCanonicalTodos(scope)
	key := todoCheckpointScopeKey(scope)

	r.todoCheckpointMu.Lock()
	defer r.todoCheckpointMu.Unlock()
	if r.softTodoCheckpointPending {
		r.softTodoCheckpointPending = false
		if progress := r.currentTodoProgress[key]; progress != nil {
			progress.Iterations = 0
			progress.Pending = false
		}
		return softTodoCheckpointPrompt
	}
	progress := r.currentTodoProgress[key]
	if progress == nil || !progress.Pending {
		return ""
	}
	if strings.TrimSpace(current) == "" || current != progress.CurrentTodoID {
		delete(r.currentTodoProgress, key)
		return ""
	}
	progress.Iterations = 0
	progress.Pending = false
	return currentTodoCheckpointPrompt
}

func (r *ReActLoop) resetSoftTodoFinishFlow() {
	r.todoCheckpointMu.Lock()
	r.softTodoChecked = false
	r.softTodoCheckpointPending = false
	r.todoCheckpointMu.Unlock()
}

// recordCurrentTodoIteration records one parsed, verified action whose handler
// has returned. Finish is excluded by the caller. The TODO snapshot is read
// after todo_delta application, so an in-turn current switch starts at one.
func (r *ReActLoop) recordCurrentTodoIteration(task aicommon.AIStatefulTask) {
	if r == nil || r.config == nil || task == nil {
		return
	}
	scope := aicommon.BuildVerificationTodoScope(task)
	_, current, _ := r.config.SnapshotCanonicalTodos(scope)
	current = strings.TrimSpace(current)
	key := todoCheckpointScopeKey(scope)

	queued := false
	r.todoCheckpointMu.Lock()
	if r.currentTodoProgress == nil {
		r.currentTodoProgress = make(map[string]*currentTodoProgress)
	}
	if current == "" {
		delete(r.currentTodoProgress, key)
	} else {
		progress := r.currentTodoProgress[key]
		if progress == nil || progress.CurrentTodoID != current {
			progress = &currentTodoProgress{CurrentTodoID: current, Iterations: 1}
			r.currentTodoProgress[key] = progress
		} else if !progress.Pending {
			progress.Iterations++
		}
		if progress.Iterations >= currentTodoCheckpointThreshold && !progress.Pending {
			progress.Pending = true
			queued = true
		}
	}
	r.todoCheckpointMu.Unlock()

	if queued && r.invoker != nil {
		r.invoker.AddToTimeline(
			"CURRENT_TODO_CHECKPOINT_REQUESTED",
			fmt.Sprintf("current TODO %q reached %d valid iterations; a soft checkpoint will be shown in the next context", current, currentTodoCheckpointThreshold),
		)
	}
}
