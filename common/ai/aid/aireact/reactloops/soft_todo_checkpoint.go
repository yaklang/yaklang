package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
)

const currentTodoCheckpointThreshold = 25

var finishTodoCheckpointPrompt = promptloader.MustLoad("inline/ai/aid/aireact/reactloops/soft_todo_checkpoint/finishTodoCheckpointPrompt.txt")

var currentTodoCheckpointPrompt = promptloader.MustLoad("inline/ai/aid/aireact/reactloops/soft_todo_checkpoint/currentTodoCheckpointPrompt.txt")

type currentTodoProgress struct {
	CurrentTodoID string
	Iterations    int
	Pending       bool
}

func todoCheckpointScopeKey(scope aicommon.VerificationTodoScope) string {
	return scope.TaskID + "\x00" + scope.TaskIndex
}

// requestFinishTodoCheckpoint is called only when finish is blocked by open
// TODOs. It subsumes a pending CURRENT checkpoint for the same task scope.
func (r *ReActLoop) requestFinishTodoCheckpoint() {
	key := todoCheckpointScopeKey(aicommon.BuildVerificationTodoScope(r.GetCurrentTask()))
	r.todoCheckpointMu.Lock()
	defer r.todoCheckpointMu.Unlock()
	r.finishTodoCheckpointScope = key
	if progress := r.currentTodoProgress[key]; progress != nil {
		progress.Iterations = 0
		progress.Pending = false
	}
}

// consumeTodoCheckpoint returns at most one checkpoint. Finish has priority;
// otherwise a CURRENT checkpoint is emitted only while the same TODO remains
// current. Consumption restarts that TODO's 25-iteration window.
func (r *ReActLoop) consumeTodoCheckpoint() string {
	if r == nil || r.config == nil {
		return ""
	}
	task := r.GetCurrentTask()
	scope := aicommon.BuildVerificationTodoScope(task)
	hasOpenTodos := len(aicommon.GetBlockingVerificationTodoItems(r.config, task)) > 0
	_, current, _ := r.config.SnapshotCanonicalTodos(scope)
	key := todoCheckpointScopeKey(scope)

	r.todoCheckpointMu.Lock()
	defer r.todoCheckpointMu.Unlock()
	if r.finishTodoCheckpointScope != "" {
		pendingScope := r.finishTodoCheckpointScope
		r.finishTodoCheckpointScope = ""
		if pendingScope == key && hasOpenTodos {
			return finishTodoCheckpointPrompt
		}
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
			fmt.Sprintf("current TODO %q has occupied a long execution window; a progress checkpoint will be shown in the next context", current),
		)
	}
}
