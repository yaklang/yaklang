package aid

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

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

	completedCount atomic.Int32
	inFlightCount  atomic.Int32
}

func (r *runtime) currentIndex() int {
	if r.cursor <= 0 {
		return 0
	}
	return r.cursor - 1
}

// currentProgressIndex returns the current progress index (1-based)
func (r *runtime) currentProgressIndex() int {
	if r == nil {
		return 0
	}
	progress := int(r.completedCount.Load()) + int(r.inFlightCount.Load())
	if progress > 0 {
		return progress
	}
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

func (r *runtime) locateStartTaskCursor(startTaskIndex string) error {
	startTaskIndex = strings.TrimSpace(startTaskIndex)
	if startTaskIndex == "" {
		r.cursor = 0
		return nil
	}
	for i := 0; i < r.TaskLink.Len(); i++ {
		task, ok := r.TaskLink.Get(i)
		if !ok || task == nil {
			continue
		}
		if task.Index == startTaskIndex {
			r.cursor = i
			return nil
		}
	}
	return utils.Errorf("runtime: start task %q not found", startTaskIndex)
}

func (r *runtime) Invoke(task *AiTask, startTaskIndex string) (retErr error) {
	if r.RootTask == nil {
		r.RootTask = task
	}
	r.updateTaskLink()
	if err := r.locateStartTaskCursor(startTaskIndex); err != nil {
		return err
	}

	leafTotal := len(collectLeafTasks(r.RootTask))
	r.config.planLoadingStatus(fmt.Sprintf("开始执行任务队列 (%d 个叶子任务, DAG 并发) / Starting Task Queue (%d Leaf Tasks, DAG Concurrent)", leafTotal, leafTotal))

	var currentTask *AiTask
	phase := Phase_NotCompleted
	defer func() {
		r.config.savePlanAndExecState(phase, currentTask)
	}()

	ctx := r.config.GetContext()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	recoveryStart := startTaskIndex
	for {
		if r.RootTask != nil {
			r.config.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
				"root_task": r.RootTask,
			})
		}

		pending := r.collectSchedulableLeaves(recoveryStart)
		if len(pending) == 0 {
			r.config.planLoadingStatus("所有任务执行完成 / All Tasks Completed")
			phase = Phase_Completed
			currentTask = nil
			return nil
		}

		currentTask = pending[0]
		r.config.planLoadingStatus(fmt.Sprintf("执行进度: %d/%d - 待执行: %d / Progress: %d/%d - Pending: %d",
			r.currentProgressIndex(), leafTotal, len(pending), r.currentProgressIndex(), leafTotal, len(pending)))
		r.config.savePlanAndExecState(Phase_NotCompleted, currentTask)

		if err := r.executePlanTaskDAG(ctx, recoveryStart); err != nil {
			if r.config.IsCtxDone() {
				r.config.planLoadingStatus("用户终止执行 / User Terminated Execution")
				r.config.EmitInfo("coordinator context cancelled, stopping execution")
				return err
			}
			r.config.planLoadingStatus(fmt.Sprintf("任务执行失败 / Task Execution Failed"))
			r.config.EmitPlanExecFail("invoke plan task dag failed: %v", err)
			r.config.EmitError("invoke plan task dag failed: %v", err)
			log.Errorf("invoke plan task dag failed: %v", err)
			cancel()
			return err
		}

		recoveryStart = ""
		r.updateTaskLink()
		leafTotal = len(collectLeafTasks(r.RootTask))
	}
}

func (r *runtime) updateTaskLink() {
	if r.RootTask == nil {
		return
	}
	r.TaskLink = DFSOrderAiTask(r.RootTask)
}
