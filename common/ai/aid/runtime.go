package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/linktable"
)

type runtime struct {
	RootTask *AiTask
	config   *Coordinator

	currentIndex int
	TaskLink     *linktable.LinkedList[*AiTask]
	statusMutex  sync.Mutex
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
		if t.Goal != "" {
			taskNameShow = taskNameShow + "(目标:" + strconv.Quote(t.Goal) + ")"
		}
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
	defer func() {
		r.currentIndex++
	}()
	return r.TaskLink.Get(r.currentIndex)
}

func (r *runtime) Invoke(task *AiTask) {
	if r.RootTask == nil {
		r.RootTask = task
	}
	r.updateTaskLink()
	r.currentIndex = 0

	invokeTask := func(current *AiTask) error {
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
		currentTask, ok := r.NextStep()
		if !ok {
			return
		}
		if err := invokeTask(currentTask); err != nil {
			r.config.EmitError("invoke subtask failed: %v", err)
			log.Errorf("invoke subtask failed: %v", err)
			return
		}
	}
}

func (r *runtime) updateTaskLink() {
	if r.RootTask == nil {
		return
	}
	r.TaskLink = DFSOrderAiTask(r.RootTask)
}
