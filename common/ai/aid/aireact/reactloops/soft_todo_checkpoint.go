package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const currentTodoCheckpointThreshold = 25

const finishTodoCheckpointPrompt = `[FINISH BLOCKED BY TODO]

你请求 finish，但当前任务仍有未完成 TODO。

继续执行当前 TODO；没有 CURRENT 时，选择一个可执行的开放项并用 todo_delta 设为 CURRENT。
仅当已有 Observation 或交付物支持该项结论时，才用 todo_delta.close 写明 outcome、reason 和 refs。
不要为结束而批量关闭、降级或 deferred，也不要重复答复或反复 finish。
处理完剩余事项后再审计验收覆盖、新发现和终态理由；无开放 TODO 仍不能证明完成。`

const currentTodoCheckpointPrompt = `[CURRENT TODO CHECKPOINT]

当前 CURRENT TODO 已持续经过多次相似操作，系统请求一次进展审计。

在选择下一步前快速检查：

- 当前 CURRENT 是否仍是阻塞用户验收、且最接近最新证据的主要矛盾；其它开放 TODO 是否完整保存了待返回的 Frontier；
- 当前路径是否仍在产生可确认的新信息；
- 下一步是否具有与之前不同的明确验证目标；
- 当前 TODO 是否过大，需要拆分；
- 当前 Observation 是否产生了尚未进入 Frontier 的同级有效分支；
- 是否正在重复近似行动和近似观察；
- 当前事项是否已确认、被有区分力地排除、被外部前置条件阻塞，或暂时没有信息增益但 Frontier 仍有可执行项；
- 最近的失败是否只来自一种工具调用、参数形态、连接、认证上下文、观察通道或 payload。

如果当前路径仍有新的可控变量和明确进展，改变实验设计并沿 CURRENT 继续向深处执行；新出现的同级具体入口先用 todo_delta 保存到 Frontier，不得漏记。
单次失败不能 close 或 deferred。先修正调用，或改变方法、编码、参数通道、请求形态、会话、基线与观察通道。CURRENT 有证据闭环或真实外部阻塞时才 close 并设置下一 CURRENT；暂时零信息时用 update 保存阶段结果、已尝试变化与不确定性, 保持开放并切换 current, 不得为切换焦点而关闭。
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
