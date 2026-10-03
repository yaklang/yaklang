package aireact

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// The callback-inheritance tests use the ordinary default loop at the entry
// and the inherited coordinator/worker protocol after handing off the request.
// Model selection remains entirely under the runtime being tested.
func newNativePlanTestModel(tool string, taskCount ...int) func(aicommon.AICallerConfigIf, *aicommon.AIRequest, string) (*aicommon.AIResponse, bool, error) {
	var mu sync.Mutex
	workerSteps := map[string]int{}
	statePattern := regexp.MustCompile(`\[([^\]]+)\]: ([a-z_]+); attempt=(\d+)`)
	count := 1
	if len(taskCount) > 0 {
		count = taskCount[0]
	}
	tasks := make([]any, 0, count)
	for i := 0; i < count; i++ {
		deps := []string{}
		if i > 0 {
			deps = append(deps, fmt.Sprintf("check_%d", i-1))
		}
		tasks = append(tasks, map[string]any{"name": fmt.Sprintf("Check %d", i), "goal": "Call " + tool, "identifier": fmt.Sprintf("check_%d", i), "depends_on": deps})
	}
	return func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest, model string) (*aicommon.AIResponse, bool, error) {
		wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
		if req.GetCallerLabel() != "react-loop:coordinator" && req.GetCallerLabel() != "react-loop:pe_task" {
			return nil, false, nil
		}
		mu.Lock()
		defer mu.Unlock()
		name, args := "finish", map[string]any{}
		prompt := req.GetPrompt()
		switch {
		case strings.Contains(prompt, "执行已批准的冻结任务书。"):
			key := req.GetTaskIndex()
			if key == "" {
				if match := regexp.MustCompile(`CURRENT TASK \[task_index=([^,\]]+)`).FindStringSubmatch(prompt); len(match) > 1 {
					key = match[1]
				}
			}
			step := workerSteps[key]
			workerSteps[key]++
			if tool == "" {
				step++
			}
			switch step {
			case 0:
				name, args = "directly_call_tool", map[string]any{"directly_call_tool_name": tool, "directly_call_tool_params": map[string]any{}, "directly_call_reason": "Execute the approved deterministic check."}
			case 1:
				name, args = "submit_task_result", map[string]any{"summary": "The deterministic check completed."}
			}
		case strings.Contains(prompt, "Draft version: 0"):
			name, args = "create_plan", map[string]any{"plan": map[string]any{"name": "Callback inheritance", "goal": "Run the deterministic tool through the inherited model", "tasks": tasks}, "plan_document": "# Run the approved check"}
		case strings.Contains(prompt, "approved version: 0"):
			name, args = "submit_plan", map[string]any{"plan_version": 1}
		default:
			for _, state := range statePattern.FindAllStringSubmatch(prompt, -1) {
				switch state[2] {
				case "pending":
					name = "start_tasks"
				case "running":
					name, args = "wait_tasks", map[string]any{"timeout_seconds": 1}
				case "awaiting_review":
					attempt, _ := strconv.Atoi(state[3])
					name, args = "review_task", map[string]any{"task_id": state[1], "attempt_id": attempt, "decision": "accept", "reason": "The delivered result completes the deterministic check."}
				}
				if state[2] == "running" || state[2] == "awaiting_review" {
					break
				}
			}
		}
		native := wire.ToolCallCallback != nil && wire.FinishReasonCallback != nil
		if !native {
			args["@action"] = name
		}
		data, err := json.Marshal(args)
		if err != nil {
			return nil, true, err
		}
		rsp := c.NewAIResponse()
		if native {
			wire.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("inherit-%d", req.GetSeqId()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: string(data)}}})
			wire.FinishReasonCallback("tool_calls", nil)
		} else {
			rsp.EmitOutputStream(strings.NewReader(string(data)))
		}
		rsp.SetModelInfo("mock", model)
		rsp.Close()
		return rsp, true, nil
	}
}
