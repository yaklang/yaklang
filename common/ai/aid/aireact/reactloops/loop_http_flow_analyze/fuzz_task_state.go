package loop_http_flow_analyze

import (
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

const dispatchedFuzzTasksKey = "dispatched_fuzz_tasks"

type dispatchedFuzzTask struct {
	SubTaskID      string    `json:"sub_task_id"`
	FlowLocator    string    `json:"flow_locator"`
	FlowURL        string    `json:"flow_url"`
	VulnType       string    `json:"vuln_type"`
	TaskDesc       string    `json:"task_desc"`
	ResultSummary  string    `json:"result_summary"`
	ExecutionError string    `json:"execution_error,omitempty"`
	DispatchedAt   time.Time `json:"dispatched_at"`
}

func getDispatchedFuzzTasks(loop *reactloops.ReActLoop) []dispatchedFuzzTask {
	if loop == nil {
		return nil
	}
	raw := loop.GetVariable(dispatchedFuzzTasksKey)
	if raw == nil {
		return nil
	}
	if tasks, ok := raw.([]dispatchedFuzzTask); ok {
		result := make([]dispatchedFuzzTask, len(tasks))
		copy(result, tasks)
		return result
	}
	return nil
}

func appendDispatchedFuzzTask(loop *reactloops.ReActLoop, task dispatchedFuzzTask) {
	if loop == nil {
		return
	}
	task.DispatchedAt = time.Now()
	tasks := getDispatchedFuzzTasks(loop)
	tasks = append(tasks, task)
	loop.Set(dispatchedFuzzTasksKey, tasks)
}

// buildDispatchedFuzzTasksPrompt constructs a summary of dispatched fuzz tasks for injection into reactive_data.
func buildDispatchedFuzzTasksPrompt(loop *reactloops.ReActLoop) string {
	tasks := getDispatchedFuzzTasks(loop)
	if len(tasks) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共已派发 %d 个 fuzz 测试任务：\n\n", len(tasks)))
	for i, t := range tasks {
		sb.WriteString(fmt.Sprintf("### 任务 %d: %s\n", i+1, t.VulnType))
		sb.WriteString(fmt.Sprintf("- 目标 Flow: %s (%s)\n", utils.ShrinkString(t.FlowURL, 80), t.FlowLocator))
		sb.WriteString(fmt.Sprintf("- 测试说明: %s\n", utils.ShrinkString(t.TaskDesc, 100)))
		if t.ExecutionError != "" {
			sb.WriteString(fmt.Sprintf("- ⚠️ 执行错误: %s\n", t.ExecutionError))
		}
		if t.ResultSummary != "" {
			sb.WriteString("\n**测试摘要**:\n")
			sb.WriteString(utils.ShrinkTextBlock(t.ResultSummary, 600))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}
