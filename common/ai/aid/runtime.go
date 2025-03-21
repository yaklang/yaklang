package aid

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"io"
	"strconv"
	"strings"
	"sync"
	"text/template"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"
)

type runtime struct {
	RootTask *aiTask
	Stack    *utils.Stack[*aiTask]

	statusMutex     sync.Mutex
	toolCallResults []*aitool.ToolResult
}

func createRuntime() *runtime {
	return &runtime{
		Stack: utils.NewStack[*aiTask](),
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
	r.Stack.Push(task)
	r.statusMutex.Unlock()
	defer func() {
		r.statusMutex.Lock()
		task.executed = true
		task.executing = false
		r.Stack.Pop()
		r.statusMutex.Unlock()
	}()

	if len(task.Subtasks) > 0 {
		for idxRaw, subtask := range task.Subtasks {
			idx := idxRaw + 1
			err := r.invokeSubtask(idx, subtask)
			if err != nil {
				// invoke subtask failed
				// retry via user!
				return err
			}
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
	r.invokeSubtask(1, task)
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
