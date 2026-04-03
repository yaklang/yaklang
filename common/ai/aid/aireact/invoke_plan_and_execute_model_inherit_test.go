package aireact

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_RequestPlanAndExecution_PreservesQualityModelInsideAid(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 200)

	const qualityModel = "quality-model"
	const fastModel = "fast-model"

	var qualityCallCount int32
	var fastCallCount int32

	mockTool, err := aitool.New(
		"mock_plan_exec_tool",
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			_, _ = io.WriteString(stdout, "mock tool executed")
			return map[string]any{
				"ok": true,
			}, nil
		}),
	)
	require.NoError(t, err)

	qualityCallback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		atomic.AddInt32(&qualityCallCount, 1)
		rsp := i.NewAIResponse()
		rsp.SetModelInfo("mock-provider", qualityModel)

		prompt := req.GetPrompt()
		switch {
		case utils.MatchAllOfSubString(prompt, "directly_answer", "request_plan_and_execution", "require_tool"):
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "request_plan_and_execution", "plan_request_payload": "execute mock tool in aid" },
"human_readable_thought": "delegate to plan execution", "cumulative_summary": "delegate to aid"}
`))
		case utils.MatchAllOfSubString(prompt, "FINAL_ANSWER", "answer_payload") && !utils.MatchAllOfSubString(prompt, "require_tool"):
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "outer summary"}`))
		case utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning"):
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "outer task completed"}`))
		default:
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "outer fallback"}`))
		}

		rsp.Close()
		return rsp, nil
	}

	fastCallback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		atomic.AddInt32(&fastCallCount, 1)
		rsp := i.NewAIResponse()
		rsp.SetModelInfo("mock-provider", fastModel)

		prompt := req.GetPrompt()
		switch {
		case strings.Contains(prompt, `"main_task"`) &&
			strings.Contains(prompt, `"main_task_goal"`) &&
			strings.Contains(prompt, `"subtask_name"`):
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "plan",
  "query": "execute mock tool in aid",
  "main_task": "execute mock tool in aid",
  "main_task_goal": "run the mock tool and complete the delegated task",
  "tasks": [
    {
      "subtask_name": "call mock plan exec tool",
      "subtask_goal": "invoke the mock tool once and finish the task"
    }
  ]
}`))
		case utils.MatchAllOfSubString(prompt, "Generate appropriate parameters for this tool call based on the context above"):
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "call-tool", "tool": "mock_plan_exec_tool", "params": {}}
`))
		case utils.MatchAllOfSubString(prompt, "continue-current-task", "proceed-next-task", "task-failed"):
			rsp.EmitOutputStream(bytes.NewBufferString(`{
  "@action": "continue-current-task",
  "status_summary": "mock tool call completed",
  "task_short_summary": "mock tool completed"
}`))
		case utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied", "reasoning"):
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "inner task completed"}`))
		case utils.MatchAllOfSubString(prompt, "任务执行引擎", "task_long_summary") && !utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_"):
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "summary", "status_summary": "done", "task_short_summary": "completed", "task_long_summary": "inner task completed"}`))
		case utils.MatchAllOfSubString(prompt, "FINAL_ANSWER", "answer_payload") && !utils.MatchAllOfSubString(prompt, "require_tool"):
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "inner summary"}`))
		case utils.MatchAllOfSubString(prompt, "PROGRESS_TASK_", "directly_answer", "require_tool"):
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "mock_plan_exec_tool" },
"human_readable_thought": "call delegated tool", "cumulative_summary": "call delegated tool"}
`))
		default:
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "finish", "human_readable_thought": "fast fallback"}`))
		}

		rsp.Close()
		return rsp, nil
	}

	_, err = NewTestReAct(
		aicommon.WithAICallback(fastCallback),
		aicommon.WithQualityPriorityAICallback(qualityCallback),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
		aicommon.WithAgreeYOLO(true),
		aicommon.WithEnablePlanAndExec(true),
		aicommon.WithTools(mockTool),
	)
	require.NoError(t, err)

	in <- &ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "please delegate and run the tool through aid",
	}

	var (
		mu                        sync.Mutex
		modelsBeforePlanStart     []string
		modelsAfterPlanStart      []string
		planStarted               bool
		reactTaskCompleted        bool
		sawQualityBeforePlan      bool
		sawQualityAfterPlanStart  bool
		sawFastAfterPlanStarted   bool
	)

	timeout := time.After(20 * time.Second)
LOOP:
	for {
		select {
		case e := <-out:
			switch e.Type {
			case schema.EVENT_TYPE_START_PLAN_AND_EXECUTION:
				planStarted = true
			case schema.EVENT_TYPE_AI_CALL_SUMMARY:
				var data map[string]any
				err := json.Unmarshal(e.Content, &data)
				require.NoError(t, err)
				modelName := utils.InterfaceToString(data["model_name"])
				mu.Lock()
				if planStarted {
					modelsAfterPlanStart = append(modelsAfterPlanStart, modelName)
					if modelName == qualityModel {
						sawQualityAfterPlanStart = true
					}
					if modelName == fastModel {
						sawFastAfterPlanStarted = true
					}
				} else {
					modelsBeforePlanStart = append(modelsBeforePlanStart, modelName)
					if modelName == qualityModel {
						sawQualityBeforePlan = true
					}
				}
				mu.Unlock()
			case schema.EVENT_TYPE_STRUCTURED:
				if e.NodeId == "react_task_status_changed" {
					var data map[string]any
					err := json.Unmarshal(e.Content, &data)
					require.NoError(t, err)
					if utils.InterfaceToString(data["react_task_now_status"]) == "completed" {
						reactTaskCompleted = true
						break LOOP
					}
				}
			}
		case <-timeout:
			break LOOP
		}
	}

	require.True(t, reactTaskCompleted, "expected delegated ReAct task to complete")
	require.Greater(t, atomic.LoadInt32(&qualityCallCount), int32(0), "expected outer ReAct to call quality model")
	require.True(t, sawQualityBeforePlan, "expected quality model before entering aid, got: %v", modelsBeforePlanStart)
	require.True(t, sawQualityAfterPlanStart, "expected quality model to be preserved after entering aid, got: %v", modelsAfterPlanStart)
	require.False(t, sawFastAfterPlanStarted, "did not expect fast model after entering aid, got: %v", modelsAfterPlanStart)
	_ = fastCallCount
}
