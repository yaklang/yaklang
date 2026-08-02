package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const currentTodoCheckpointThreshold = 25

const softTodoCheckpointPrompt = `[SOFT TODO CHECKPOINT]

你准备结束当前任务。

结束前快速检查 TODO、timeline 和最新 Observation。TODO 已清空不是任务完成的充分证据：

- 是否仍有不能忽略的开放事项；
- 是否有 TODO 已解决但尚未记录结果；
- 是否有验证型 TODO 仅凭单次阴性请求、普通扫描未命中或无明显报错就被关闭；若有，重新添加其中价值最高的一项作为有区分力的验证 TODO，并设为 CURRENT；
- timeline 或最新 Observation 是否暴露了尚未通过 todo_delta 进入 Frontier 的范围内具体入口（链接、表单 action、跳转、脚本路由、文档端点或响应字段）；
- 是否把单次工具、参数、连接、认证、空响应或 payload 失败误当成路径结束，而没有先执行有实质差异的修正或替代实验；
- 未完成事项是否应明确标记为 deferred。

还要检查是否出现了尚未进入 TODO 的高价值后续行动。只有同时满足以下条件时，它才阻止结束：

- 仍属于当前用户目标和 CURRENT-TASK；
- 由已有证据、异常或弱信号直接支持；
- 无需用户新增目标或授权，能够在当前主线之后或前置条件满足后执行；
- 预计会实质提高结论可信度、风险覆盖或影响判断。

若存在一个或多个合格分支，立即用本轮 todo_delta 先将它们全部加入或更新到 Frontier。覆盖入口写清目标、来源证据和第一步；验证型分支再写可证伪假设。随后将价值最高且最接近当前证据的一项设为 CURRENT 并继续 ReAct，不要再次 finish。
通用优化、范围外扩展、需要用户新选择、没有具体观察目标或只有空泛猜测且预期信息增益很低的想法不阻止结束，也不要为了显得主动制造 TODO。

不要为了清空列表而伪造完成。
仅在确有必要时更新 TODO；否则再次结束任务。`

const currentTodoCheckpointPrompt = `[CURRENT TODO CHECKPOINT]

当前 CURRENT TODO 已连续执行 25 个有效迭代。

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
单次失败不能 close 或 deferred。先修正调用，或改变方法、编码、参数通道、请求形态、会话、基线与观察通道。CURRENT 闭环、外部阻塞或暂时零信息时，close 或 update 写清阶段结果、已尝试变化和恢复条件；交接时在同一 todo_delta 中 close 旧项并设置下一 CURRENT。
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
