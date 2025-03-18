package taskstack

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

type Runtime struct {
	RootTask *Task
	Stack    *utils.Stack[*Task]

	statusMutex sync.Mutex
}

func CreateRuntime() *Runtime {
	return &Runtime{
		Stack: utils.NewStack[*Task](),
	}
}

func (t *Task) dumpProgress(i int, w io.Writer) {
	prefix := strings.Repeat(" ", i)

	executing := false
	finished := false
	if len(t.Subtasks) > 0 {
		allFinished := true
		haveExecutedTask := false
		for _, subtask := range t.Subtasks {
			if !subtask.executed {
				allFinished = false
			} else if !haveExecutedTask && subtask.executed {
				haveExecutedTask = true
			}
		}
		if haveExecutedTask && !allFinished {
			executing = true
		} else if allFinished {
			finished = true
		}
	} else {
		finished = t.executed
	}

	var fill = " "
	var note string
	if finished {
		fill = "x"
	} else if executing {
		fill = "~"
		note = " (部分完成)"
	}

	if t.executing {
		fill = "-"
		note = " (执行中)"
	}

	_, _ = fmt.Fprintf(w, "%s -[%v] %s %v\n", prefix, fill, strconv.Quote(t.Name), note)
	if len(t.Subtasks) > 0 {
		for _, subtask := range t.Subtasks {
			subtask.dumpProgress(i+1, w)
		}
	}
}

func (r *Runtime) Progress() string {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	if r.RootTask == nil {
		return ""
	}
	var buf bytes.Buffer
	r.RootTask.dumpProgress(0, &buf)
	return buf.String()
}

func (r *Runtime) invokeSubtask(idx int, task *Task) (aispec.ChatDetails, error) {
	r.statusMutex.Lock()
	if r.RootTask == nil {
		r.RootTask = task
	}
	task.executing = true
	r.Stack.Push(task)
	r.statusMutex.Unlock()
	defer func() {
		r.statusMutex.Lock()
		task.executed = true
		task.executing = false
		r.Stack.Pop()
		r.statusMutex.Unlock()
	}()

	allDetails := aispec.ChatDetails{}
	if len(task.Subtasks) > 0 {
		for idxRaw, subtask := range task.Subtasks {
			idx := idxRaw + 1
			details, err := r.invokeSubtask(idx, subtask)
			if err != nil {
				return nil, err
			}
			allDetails = append(allDetails, details...)
		}
		return allDetails, nil
	}

	progress := r.Progress()
	return task.executeTask(&TaskSystemContext{
		Progress:    progress,
		CurrentTask: task,
	})
}

func (r *Runtime) Invoke(task *Task) {
	r.invokeSubtask(1, task)
}
