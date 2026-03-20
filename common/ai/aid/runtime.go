package aid

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

type runtime struct {
	RootTask *AiTask
	config   *Coordinator

	cursor      int
	TaskLink    *linktable.LinkedList[*AiTask]
	statusMutex sync.Mutex
}

func (r *runtime) currentIndex() int {
	if r.cursor <= 0 {
		return 0
	}
	return r.cursor - 1
}

// currentProgressIndex returns the current progress index (1-based)
func (r *runtime) currentProgressIndex() int {
	return r.cursor
}

func (c *Coordinator) createRuntime() *runtime {
	r := &runtime{
		config:   c,
		TaskLink: linktable.New[*AiTask](),
	}
	return r
}

func (t *AiTask) dumpProgressEx(i int, w io.Writer, details bool) {
	prefix := strings.Repeat(" ", i)

	executing := false
	finished := false
	if len(t.Subtasks) > 0 {
		allFinished := true
		haveExecutedTask := false
		for _, subtask := range t.Subtasks {
			if !subtask.executed() {
				allFinished = false
			} else if !haveExecutedTask && subtask.executed() {
				haveExecutedTask = true
			}
		}
		if haveExecutedTask && !allFinished {
			executing = true
		} else if allFinished {
			finished = true
		}
	} else {
		finished = t.executed()
	}

	var fill = " "
	var note string
	var finishedSummary string // separate line for finished summary
	if finished {
		fill = "x"
		// Get the best available summary for finished tasks
		if t.LongSummary != "" {
			finishedSummary = t.LongSummary
		} else if t.TaskSummary != "" {
			finishedSummary = t.TaskSummary
		} else if t.ShortSummary != "" {
			finishedSummary = t.ShortSummary
		}
	} else if executing {
		fill = "~"
		note = " (部分完成)"
	}

	if t.executing() {
		fill = "-"
		note = " (执行中)"
		if ret := t.SingleLineStatusSummary(); ret != "" {
			note += fmt.Sprintf(" (status:%s)", ret)
		}
	}

	taskNameShow := strconv.Quote(t.Name)
	if details {
		if t.Goal != "" {
			taskNameShow = taskNameShow + "(目标:" + strconv.Quote(t.Goal) + ")"
		}
		if t.Index != "" {
			taskNameShow = t.Index + ". " + taskNameShow
		}
	}
	if strings.TrimSpace(note) == "" && finishedSummary == "" {
		note = "(未开始)"
	}
	_, _ = fmt.Fprintf(w, "%s-[%v] %s%v\n", prefix, fill, taskNameShow, note)

	// Output finished summary on a new indented line if available
	if finishedSummary != "" {
		summaryPrefix := strings.Repeat(" ", i+1) // extra indent for summary
		_, _ = fmt.Fprintf(w, "%sFinished: %s\n", summaryPrefix, finishedSummary)
	}

	if len(t.Subtasks) > 0 {
		for _, subtask := range t.Subtasks {
			subtask.dumpProgressEx(i+1, w, details)
		}
	}
}

func (t *AiTask) dumpProgress(i int, w io.Writer) {
	t.dumpProgressEx(i, w, true)
}

func (t *AiTask) Progress() string {
	if t == nil {
		return ""
	}
	var buf bytes.Buffer
	t.dumpProgress(0, &buf)
	return buf.String()
}

func (t *AiTask) ProgressWithDetail() string {
	if t == nil {
		return ""
	}
	var buf bytes.Buffer
	t.dumpProgressEx(0, &buf, true)
	return buf.String()
}

func (r *runtime) Progress() string {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	if r.RootTask == nil {
		return ""
	}
	var buf bytes.Buffer
	r.RootTask.dumpProgress(0, &buf)
	return buf.String()
}

func (r *runtime) NextStep() (*AiTask, bool) {
	task, ok := r.TaskLink.Get(r.cursor)
	if ok {
		r.cursor++
		return task, true
	}
	return task, false
}

func (r *runtime) Invoke(task *AiTask) (retErr error) {
	if r.RootTask == nil {
		r.RootTask = task
	}
	r.updateTaskLink()
	r.cursor = 0

	// Calculate total tasks for progress display
	totalTasks := r.TaskLink.Len()
	r.config.planLoadingStatus(fmt.Sprintf("开始执行任务队列 (%d 个任务) / Starting Task Queue (%d Tasks)", totalTasks, totalTasks))

	var currentTask *AiTask
	phase := "executing"
	defer func() {
		r.config.savePlanAndExecState(phase, currentTask)
	}()

	invokeTask := func(current *AiTask) error {
		// 检查任务是否已被用户主动跳过（Skipped 状态，区别于 Aborted 失败状态）
		// 如果任务已被用户主动跳过，则直接返回 nil 继续下一个任务
		if current.GetStatus() == aicommon.AITaskState_Skipped {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
			r.config.EmitInfo("subtask %s was skipped by user, moving to next task", current.Name)
			return nil
		}
		// 恢复执行时，如果任务已完成，直接跳过
		if current.executed() {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已完成 / Task [%s] Completed", current.Index, current.Index))
			r.config.EmitInfo("subtask %s already completed, moving to next task", current.Name)
			return nil
		}
		// 检查全局 context 是否被取消（用户终止整个任务）
		if r.config.IsCtxDone() {
			r.config.planLoadingStatus("执行已取消 / Execution Cancelled")
			return utils.Errorf("coordinator context is done")
		}

		// 检查任务自身的 context（可能被 skiped/redo 重置）
		if current.IsCtxDone() {
			// 再次检查状态，如果是 Skipped，说明是被用户主动跳过的
			if current.GetStatus() == aicommon.AITaskState_Skipped {
				r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
				r.config.EmitInfo("subtask %s context cancelled (skipped), moving to next task", current.Name)
				return nil
			}
			return utils.Errorf("task context is done")
		}

		// Emit task start status with progress info
		r.config.planLoadingStatus(fmt.Sprintf("准备执行任务 [%s]: %s / Preparing Task [%s]: %s",
			current.Index, current.Name, current.Index, current.Name))

		r.config.EmitInfo("invoke subtask: %v", current.Name)
		if len(current.Subtasks) == 0 {
			current.SetStatus(aicommon.AITaskState_Processing) // 设置为执行中
		}
		r.config.EmitPushTask(current)
		defer func() {
			r.config.EmitUpdateTaskStatus(current)
			r.config.EmitPopTask(current)
		}()

		if len(current.Subtasks) == 0 {
			return current.executeTaskPushTaskIndex()
		}
		return nil
	}

	for {
		// 每次开始任务之前，先 emit 任务树，更新任务树进度
		if r.RootTask != nil {
			r.config.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
				"root_task": r.RootTask,
			})
		}

		currentTask, ok := r.NextStep()
		if !ok {
			r.config.planLoadingStatus("所有任务执行完成 / All Tasks Completed")
			phase = "completed"
			currentTask = nil
			return nil
		}

		// Emit current progress
		r.config.planLoadingStatus(fmt.Sprintf("执行进度: %d/%d - 当前: [%s] / Progress: %d/%d - Current: [%s]",
			r.currentProgressIndex(), totalTasks, currentTask.Index, r.currentProgressIndex(), totalTasks, currentTask.Index))
		r.config.savePlanAndExecState("executing", currentTask)

		if err := invokeTask(currentTask); err != nil {
			// 检查是否是任务被用户主动跳过导致的错误
			// 1. 检查任务状态是否为 Skipped（用户主动跳过）
			// 2. 检查错误是否包含 context canceled（任务执行中被中断）
			isSkipped := currentTask.GetStatus() == aicommon.AITaskState_Skipped
			isContextCanceled := strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context done")

			if isSkipped || (isContextCanceled && currentTask.GetStatus() == aicommon.AITaskState_Skipped) {
				r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 用户跳过,继续下一个 / Task [%s] User Skipped, Continuing", currentTask.Index, currentTask.Index))
				r.config.EmitInfo("task %s was skipped by user, continuing to next task", currentTask.Name)
				continue
			}

			// 检查全局 context 是否被取消（用户终止整个任务）
			if r.config.IsCtxDone() {
				r.config.planLoadingStatus("用户终止执行 / User Terminated Execution")
				r.config.EmitInfo("coordinator context cancelled, stopping execution")
				phase = "cancelled"
				return err
			}

			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 执行失败 / Task [%s] Failed", currentTask.Index, currentTask.Index))
			r.config.EmitPlanExecFail("invoke task[%s] failed: %v", currentTask.Name, err)
			r.config.EmitError("invoke subtask failed: %v", err)
			log.Errorf("invoke subtask failed: %v", err)
			phase = "failed"
			return err
		}
	}
}

func (r *runtime) updateTaskLink() {
	if r.RootTask == nil {
		return
	}
	r.TaskLink = topologicalDFSOrderAiTask(r.RootTask)
}

// topologicalDFSOrderAiTask returns all tasks in a deterministic order that
// respects the DependsOn relationships declared on each AiTask.
//
// Algorithm:
//  1. The root node itself is placed first (it is a container, not executed directly).
//  2. For every node that has immediate subtasks, those subtasks are sorted using
//     Kahn's topological-sort before being appended, so that each subtask appears
//     after every task it explicitly depends on.
//  3. Nested subtasks (grandchildren, etc.) are recursively handled the same way.
//  4. If a cycle or unresolvable dependency is detected the remaining tasks are
//     appended in their original order so execution is never silently lost.
func topologicalDFSOrderAiTask(root *AiTask) *linktable.LinkedList[*AiTask] {
	result := linktable.New[*AiTask]()

	var visit func(task *AiTask)
	visit = func(task *AiTask) {
		result.PushBack(task)

		if len(task.Subtasks) == 0 {
			return
		}

		// Sort the immediate children so that declared dependencies are
		// executed before the tasks that depend on them.
		sorted := topologicalSortSubtasks(task.Subtasks)
		for _, child := range sorted {
			visit(child)
		}
	}

	visit(root)
	return result
}

// topologicalSortSubtasks reorders tasks so that every task appears after all
// tasks it depends on (via the DependsOn field).  Dependencies that do not
// correspond to any task in the provided slice are simply ignored.
//
// If the dependency graph contains a cycle the cyclic tasks are appended in
// their original order at the end so that no task is dropped.
func topologicalSortSubtasks(tasks []*AiTask) []*AiTask {
	if len(tasks) <= 1 {
		return tasks
	}

	// Build a name-to-position map; first occurrence wins if names collide.
	nameToTask := make(map[string]*AiTask, len(tasks))
	for _, t := range tasks {
		if _, exists := nameToTask[t.Name]; !exists {
			nameToTask[t.Name] = t
		}
	}

	// Kahn's algorithm -------------------------------------------------
	// inDegree[name] = number of unresolved dependencies within this slice.
	inDegree := make(map[string]int, len(tasks))
	// dependents[x] = names of tasks in this slice that directly depend on x.
	dependents := make(map[string][]string, len(tasks))

	for _, t := range tasks {
		if _, ok := inDegree[t.Name]; !ok {
			inDegree[t.Name] = 0
		}
		for _, dep := range t.DependsOn {
			if _, exists := nameToTask[dep]; exists {
				inDegree[t.Name]++
				dependents[dep] = append(dependents[dep], t.Name)
			}
			// Dependencies outside the slice are ignored (already satisfied).
		}
	}

	// Seed the queue with tasks that have no in-slice dependencies.
	// Preserve original relative order for determinism.
	var queue []*AiTask
	for _, t := range tasks {
		if inDegree[t.Name] == 0 {
			queue = append(queue, t)
		}
	}

	// Build a name-to-original-index map for stable ordering of newly-ready tasks.
	// Built once here and reused inside the main loop.
	origIdx := make(map[string]int, len(tasks))
	for i, t := range tasks {
		origIdx[t.Name] = i
	}

	result := make([]*AiTask, 0, len(tasks))
	processed := make(map[string]bool, len(tasks))

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if processed[cur.Name] {
			continue
		}
		processed[cur.Name] = true
		result = append(result, cur)

		// Unlock tasks that depended on this one.
		// Append newly-ready tasks preserving their original relative order.
		var newlyReady []*AiTask
		for _, depName := range dependents[cur.Name] {
			inDegree[depName]--
			if inDegree[depName] == 0 {
				if dt, ok := nameToTask[depName]; ok && !processed[depName] {
					newlyReady = append(newlyReady, dt)
				}
			}
		}
		// Keep original relative order among newly-ready tasks.
		if len(newlyReady) > 1 {
			sort.Slice(newlyReady, func(i, j int) bool {
				return origIdx[newlyReady[i].Name] < origIdx[newlyReady[j].Name]
			})
		}
		queue = append(queue, newlyReady...)
	}

	// Append any remaining tasks (cycles or duplicate names) in original order.
	if len(result) < len(tasks) {
		for _, t := range tasks {
			if !processed[t.Name] {
				result = append(result, t)
			}
		}
	}

	return result
}
