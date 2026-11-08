package aireact

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
)

// Run real default-loop transports, tool execution and finish admission with a
// poisoned verification callback. Legacy config values must not re-enable reviews,
// even when tool results grow the prompt beyond the retired token thresholds.
func TestDefaultLoopDoesNotAutomaticallyVerifySatisfaction(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, route := range []string{"scalar", "batch", "tool_action", "pending_todo"} {
			for _, interval := range []int{0, aicommon.DefaultPeriodicVerificationInterval} {
				t.Run(fmt.Sprintf("native=%t/%s/legacy_interval=%d", native, route, interval), func(t *testing.T) {
					runDefaultWithoutVerification(t, native, route, interval)
				})
			}
		}
	}
}

func runDefaultWithoutVerification(t *testing.T, native bool, route string, interval int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	var loop *reactloops.ReActLoop
	var toolsExecuted, activityMissing, auxiliaryCalls atomic.Int32
	tool, err := aitool.New("satisfaction_removal_step",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithIntegerParam("step", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			if !loop.HasInflightToolActivity() {
				activityMissing.Add(1)
			}
			step := int(params.GetInt("step"))
			if step < 1 || step > 6 {
				return nil, fmt.Errorf("unexpected step %d", step)
			}
			if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("step-%d.txt", step)), []byte("executed"), 0600); err != nil {
				return nil, err
			}
			toolsExecuted.Add(1)
			var output strings.Builder
			for row := 0; row < 230; row++ {
				fmt.Fprintf(&output, "step=%d row=%04d source_id=S%d-%04d status=processed amount=%d currency=CNY note=independent source observation; retain audit provenance.\n", step, row, step, row, step+row)
			}
			return output.String(), nil
		}))
	require.NoError(t, err)
	steps := make([]guiReplayStep, 0, 10)
	if route == "pending_todo" {
		steps = append(steps, guiReplayStep{"finish", aitool.InvokeParams{}})
	}
	for step := 1; step <= 6; step++ {
		params := aitool.InvokeParams{"step": step}
		switch route {
		case "tool_action":
			steps = append(steps, guiReplayStep{tool.Name, params})
		case "batch":
			steps = append(steps, guiReplayStep{"directly_call_tool", aitool.InvokeParams{
				"directly_call_tool_params_group": []any{
					aitool.InvokeParams{"tool_name": tool.Name, "params": params},
					aitool.InvokeParams{"tool_name": tool.Name, "params": aitool.InvokeParams{"step": step + 1}},
				},
			}})
			step++
		default:
			steps = append(steps, guiReplayStep{"directly_call_tool", aitool.InvokeParams{
				"directly_call_tool_name": tool.Name, "directly_call_tool_params": params,
				"directly_call_reason": "Execute the required local source processing step.",
			}})
		}
	}
	if route == "pending_todo" {
		delta := aitool.InvokeParams{"close": []any{aitool.InvokeParams{
			"id": "required-work", "outcome": "resolved", "reason": "All six local operations have completed.",
			"refs": []string{"step-6.txt"},
		}}}
		if native {
			steps = append(steps, guiReplayStep{"adjust_todolist", aitool.InvokeParams{"todo_delta": delta}})
		} else {
			steps = append(steps, guiReplayStep{"save_evidence", aitool.InvokeParams{
				"evidence_id": "completed-work", "evidence_content": "All six local operations completed; step-1.txt through step-6.txt exist.", "todo_delta": delta,
			}})
		}
	}
	steps = append(steps, guiReplayStep{"finish", aitool.InvokeParams{}})
	decisions, firstPromptTokens, maxPromptTokens := 0, 0, 0
	runtime, err := NewTestReAct(
		aicommon.WithContext(ctx), aicommon.WithWorkdir(root), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tool)),
		aicommon.WithEnableFunctionCallMode(native), aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithPeriodicVerificationInterval(int64(interval)),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() == "directly-answer" {
				return guiRespond(c, req, false, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": "All six local operations completed."}})
			}
			if !strings.HasPrefix(req.GetCallerLabel(), "react-loop:") || decisions >= len(steps) {
				return nil, fmt.Errorf("unexpected model request %q at decision %d", req.GetCallerLabel(), decisions)
			}
			tokens := aicommon.MeasureTokens(req.GetPrompt())
			if decisions == 0 {
				firstPromptTokens = tokens
			}
			if tokens > maxPromptTokens {
				maxPromptTokens = tokens
			}
			step := steps[decisions]
			decisions++
			return guiRespond(c, req, native, step)
		}),
		aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if req.GetCallerLabel() == "liteforge[tool-call-reason]" {
				return guiRespond(c, req, false, guiReplayStep{"tool-call-reason", aitool.InvokeParams{"reason": "Process source step"}})
			}
			auxiliaryCalls.Add(1)
			return nil, fmt.Errorf("unexpected auxiliary call: %s", req.GetCallerLabel())
		}),
	)
	require.NoError(t, err)
	defer func() { cancel(); runtime.WaitLifecycleStopped() }()
	runtime.config.RecordRecentlyUsedTool(tool)
	options := append(reactloops.BasicAICommonConfigOption(runtime.config),
		reactloops.WithPeriodicVerificationInterval(interval), reactloops.WithDisablePeriodicVerification(false),
		reactloops.WithDisableLoopPerception(true), reactloops.WithDisableIncreaseIteration(true), reactloops.WithMaxIterations(16))
	if route == "tool_action" {
		options = append(options, reactloops.WithRegisterLoopActionFromTool(tool))
	}
	loop, err = reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_DEFAULT, runtime, options...)
	require.NoError(t, err)
	task := aicommon.NewStatefulTaskBase("satisfaction-removal", "Complete all six local source processing steps before finishing.", ctx, runtime.Emitter, true)
	if route == "pending_todo" {
		results := runtime.config.ApplyTodoDelta(aicommon.BuildVerificationTodoScope(task), &aicommon.TodoDelta{
			Add: []aicommon.TodoAdd{{ID: "required-work", Text: "Execute all six source processing steps."}},
		})
		require.Empty(t, aicommon.FormatVerificationTodoApplyErrors(results))
	}
	require.NoError(t, loop.ExecuteWithExistedTask(task))
	require.True(t, task.IsFinished())
	require.Equal(t, len(steps), decisions)
	require.EqualValues(t, 6, toolsExecuted.Load())
	require.Zero(t, auxiliaryCalls.Load())
	require.Zero(t, activityMissing.Load(), "stall monitoring must see every active tool")
	require.False(t, loop.HasInflightToolActivity())
	require.Nil(t, loop.GetLastSatisfactionRecordFull())
	require.Greater(t, maxPromptTokens-firstPromptTokens, 20*1024)
	for step := 1; step <= 6; step++ {
		body, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("step-%d.txt", step)))
		require.NoError(t, err)
		require.Equal(t, "executed", string(body))
	}
	require.Equal(t, route == "pending_todo", strings.Contains(runtime.config.Timeline.String(), "FINISH_BLOCKED_BY_TODO"))
}
