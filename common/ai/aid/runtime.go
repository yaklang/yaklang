package aid

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type runtime struct {
	RootTask *aiTask
	config   *Config
	Stack    *utils.Stack[*aiTask]

	statusMutex     sync.Mutex
	toolCallResults []*aitool.ToolResult
}

func (c *Coordinator) createRuntime() *runtime {
	return &runtime{
		config: c.config,
		Stack:  utils.NewStack[*aiTask](),
	}
}

func (t *aiTask) dumpProgress(i int, w io.Writer) {
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
		if t.TaskSummary != "" {
			note = fmt.Sprintf(" (Finished:%s)", t.TaskSummary)
		}
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

func (r *runtime) invokeSubtask(idx int, task *aiTask) error {
	r.statusMutex.Lock()
	if r.RootTask == nil {
		r.RootTask = task
	}
	task.executing = true
	r.config.EmitInfo("invoke subtask: %v", task.Name)

	r.Stack.Push(task)
	r.config.EmitPushTask(task)

	r.statusMutex.Unlock()
	defer func() {
		r.statusMutex.Lock()
		task.executed = true
		task.executing = false
		r.Stack.Pop()
		r.config.EmitUpdateTaskStatus(task)
		r.config.EmitPopTask(task)
		r.statusMutex.Unlock()
	}()

	if len(task.Subtasks) > 0 {
		// why not use for-range but use while-loop?
		// because subsequent subtasks may be changed during the execution
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
			if subtask.rerun {
				subtask.rerun = false
				r.config.EmitInfo("subtask %v rerun", subtask.Name)
				currentID--
				continue
			}
			r.config.EmitInfo("invoke subtask success: %v with %d tool call results", subtask.Name, len(subtask.ToolCallResults))
			r.PushToolCallResults(subtask.ToolCallResults...)
		}
		return nil
	}

	return task.executeTask(&taskContext{
		Runtime:     r,
		CurrentTask: task,
	})
}

func (r *runtime) Invoke(task *aiTask) {
	err := r.invokeSubtask(1, task)
	if err != nil {
		r.config.EmitError("invoke subtask failed: %v", err)
	}
}

func (r *runtime) PushToolCallResults(t ...*aitool.ToolResult) {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	r.toolCallResults = append(r.toolCallResults, t...)
}

func (r *runtime) PromptForToolCallResultsForLastN(n int) string {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	if len(r.toolCallResults) == 0 {
		return ""
	}

	var result = r.toolCallResults
	if len(result) > n {
		result = result[len(result)-n:]
	}
	templatedata := map[string]interface{}{
		"ToolCallResults": result,
	}
	temp, err := template.New("tool-result-history").Parse(__prompt_ToolResultHistoryPromptTemplate)
	if err != nil {
		log.Errorf("error parsing tool result history template: %v", err)
		return ""
	}
	var promptBuilder strings.Builder
	err = temp.Execute(&promptBuilder, templatedata)
	if err != nil {
		log.Errorf("error executing tool result history template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

func (r *runtime) PromptForToolCallResultsForLast5() string {
	return r.PromptForToolCallResultsForLastN(5)
}

func (r *runtime) PromptForToolCallResultsForLast10() string {
	return r.PromptForToolCallResultsForLastN(10)
}

func (r *runtime) PromptForToolCallResultsForLast20() string {
	return r.PromptForToolCallResultsForLastN(20)
}
