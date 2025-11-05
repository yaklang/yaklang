package aid

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

type runtime struct {
	RootTask *AiTask
	config   *Coordinator
	Stack    *utils.Stack[*AiTask]

	statusMutex sync.Mutex
}

func (c *Coordinator) createRuntime() *runtime {
	r := &runtime{
		config: c,
		Stack:  utils.NewStack[*AiTask](),
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
	if finished {
		fill = "x"
		if t.TaskSummary != "" {
			note = fmt.Sprintf(" (Finished:%s)", t.TaskSummary)
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
		taskNameShow = taskNameShow + "(" + strconv.Quote(t.Goal) + ")"
		if t.Index != "" {
			taskNameShow = t.Index + ". " + taskNameShow
		}
	}
	if strings.TrimSpace(note) == "" {
		note = "(未开始)"
	}
	_, _ = fmt.Fprintf(w, "%s -[%v] %s %v\n", prefix, fill, taskNameShow, note)
	if len(t.Subtasks) > 0 {
		for _, subtask := range t.Subtasks {
			subtask.dumpProgressEx(i+1, w, details)
		}
	}
}

func (t *AiTask) dumpProgress(i int, w io.Writer) {
	t.dumpProgressEx(i, w, false)
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

func (r *runtime) invokeSubtask(idx int, task *AiTask) error {
	r.statusMutex.Lock()
	if r.RootTask == nil {
		r.RootTask = task
	}
	r.config.EmitInfo("invoke subtask: %v", task.Name)

	r.Stack.Push(task)
	r.config.EmitPushTask(task)

	r.statusMutex.Unlock()
	defer func() {
		r.statusMutex.Lock()
		r.Stack.Pop()
		r.config.EmitUpdateTaskStatus(task)
		r.config.EmitPopTask(task)
		r.statusMutex.Unlock()
	}()

	if len(task.Subtasks) > 0 {
		return r.executeSubTask(idx, task)
	}

	return task.executeTaskPushTaskIndex()
}

func (r *runtime) executeSubTask(idx int, task *AiTask) error {
	currentID := -1
	for {
		currentID++
		if currentID >= len(task.Subtasks) {
			break
		}
		subtask := task.Subtasks[currentID]
		err := r.invokeSubtask(idx+currentID+1, subtask)
		if err != nil {
			r.config.EmitError("invoke subtask failed: %v", err)
			// invoke subtask failed
			// retry via user!
			return err
		}
		r.config.EmitInfo("invoke subtask success: %v with %d tool call results", subtask.Name, subtask.toolCallResultIds.Len())
	}
	return nil
}

func (r *runtime) Invoke(task *AiTask) {
	err := r.invokeSubtask(1, task)
	if err != nil {
		r.config.EmitError("invoke subtask failed: %v", err)
	}
}
