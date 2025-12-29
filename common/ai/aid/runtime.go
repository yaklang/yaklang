package aid

import (
	"bytes"
	"fmt"
	"io"
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

func (r *runtime) Invoke(task *AiTask) error {
	if r.RootTask == nil {
		r.RootTask = task
	}
	r.updateTaskLink()
	r.cursor = 0

	// Calculate total tasks for progress display
	totalTasks := r.TaskLink.Len()
	r.config.planLoadingStatus(fmt.Sprintf("开始执行任务队列 (%d 个任务) / Starting Task Queue (%d Tasks)", totalTasks, totalTasks))

	invokeTask := func(current *AiTask) error {
		// 检查任务是否已被用户主动跳过（Skipped 状态，区别于 Aborted 失败状态）
		// 如果任务已被用户主动跳过，则直接返回 nil 继续下一个任务
		if current.GetStatus() == aicommon.AITaskState_Skipped {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
			r.config.EmitInfo("subtask %s was skipped by user, moving to next task", current.Name)
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
			return nil
		}

		// Emit current progress
		r.config.planLoadingStatus(fmt.Sprintf("执行进度: %d/%d - 当前: [%s] / Progress: %d/%d - Current: [%s]",
			r.currentProgressIndex(), totalTasks, currentTask.Index, r.currentProgressIndex(), totalTasks, currentTask.Index))

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
				return err
			}

			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 执行失败 / Task [%s] Failed", currentTask.Index, currentTask.Index))
			r.config.EmitPlanExecFail("invoke task[%s] failed: %v", currentTask.Name, err)
			r.config.EmitError("invoke subtask failed: %v", err)
			log.Errorf("invoke subtask failed: %v", err)
			return err
		}
	}
}

func (r *runtime) updateTaskLink() {
	if r.RootTask == nil {
		return
	}
	r.TaskLink = DFSOrderAiTask(r.RootTask)
}
