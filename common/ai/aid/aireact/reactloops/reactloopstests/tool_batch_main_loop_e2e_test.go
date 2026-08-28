package reactloopstests

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

// TestReActLoop_DirectToolBatchPrimaryDecisionConcurrentOrderedE2E starts at
// the same boundary as a real model response. The first primary decision is a
// directly_call_tool action containing two array items; no test calls the
// parser, verifier, handler, or ExecuteToolBatch directly.
//
// Besides checking that both plugin callbacks run, the callback barrier makes
// serial execution fail: each callback must observe the other before either is
// allowed to return. Child 1 is then forced to finish before child 0, so the
// task assertion proves that the production coordinator joins out-of-order
// workers and commits their ToolResults in model-array order.
func TestReActLoop_DirectToolBatchPrimaryDecisionConcurrentOrderedE2E(t *testing.T) {
	const (
		firstToolName  = "main_loop_batch_e2e_first"
		secondToolName = "main_loop_batch_e2e_second"
	)

	var active int32
	var maxActive int32
	var started int32
	var finished int32
	var primaryDecisions int32
	var satisfactionChecks int32

	bothCallbacksStarted := make(chan struct{})
	secondCallbackFinished := make(chan struct{})
	var closeBothStarted sync.Once
	var closeSecondFinished sync.Once
	var completionMu sync.Mutex
	completionOrder := make([]string, 0, 2)

	updateMaxActive := func(current int32) {
		for {
			old := atomic.LoadInt32(&maxActive)
			if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
				return
			}
		}
	}

	newTool := func(name string, modelIndex int) *aitool.Tool {
		tool, err := aitool.New(
			name,
			aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
				current := atomic.AddInt32(&active, 1)
				updateMaxActive(current)
				if atomic.AddInt32(&started, 1) == 2 {
					closeBothStarted.Do(func() { close(bothCallbacksStarted) })
				}

				// If production accidentally falls back to serial invocation, the
				// first callback times out instead of deadlocking the whole test.
				select {
				case <-bothCallbacksStarted:
				case <-time.After(2 * time.Second):
					atomic.AddInt32(&active, -1)
					return nil, fmt.Errorf("batch callbacks did not overlap")
				}

				if modelIndex == 0 {
					// Force worker completion to be the reverse of model-array order.
					select {
					case <-secondCallbackFinished:
					case <-time.After(2 * time.Second):
						atomic.AddInt32(&active, -1)
						return nil, fmt.Errorf("second callback did not finish first")
					}
					time.Sleep(20 * time.Millisecond)
				}

				completionMu.Lock()
				completionOrder = append(completionOrder, name)
				completionMu.Unlock()
				atomic.AddInt32(&finished, 1)
				atomic.AddInt32(&active, -1)
				if modelIndex == 1 {
					closeSecondFinished.Do(func() { close(secondCallbackFinished) })
				}
				return name + "-result", nil
			}),
		)
		require.NoError(t, err)
		return tool
	}

	firstTool := newTool(firstToolName, 0)
	secondTool := newTool(secondToolName, 1)
	var capturedTask aicommon.AIStatefulTask

	respond := func(config aicommon.AICallerConfigIf, raw string) (*aicommon.AIResponse, error) {
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(raw))
		response.Close()
		return response, nil
	}

	react, err := aireact.NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithTools(firstTool, secondTool),
		aicommon.WithAgreeYOLO(),
		aicommon.WithToolBatchInvokeConcurrency(2),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			switch {
			case aicommon.IsVerifySatisfactionPrompt(prompt):
				atomic.AddInt32(&satisfactionChecks, 1)
				if got := atomic.LoadInt32(&finished); got != 2 {
					return nil, fmt.Errorf("satisfaction ran before batch join: finished=%d", got)
				}
				return respond(config, `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"both independent calls completed"}`)

			case aicommon.IsPrimaryDecisionPrompt(prompt):
				decision := atomic.AddInt32(&primaryDecisions, 1)
				if decision == 1 {
					return respond(config, `{
  "@action": "directly_call_tool",
  "identifier": "parallel_main_loop_e2e",
  "human_readable_thought": "Run two independent tools concurrently",
  "directly_call_tool_calls": [
    {
      "tool_name": "main_loop_batch_e2e_first",
      "params": {},
      "identifier": "first_child",
      "reason": "Execute the first independent child"
    },
    {
      "tool_name": "main_loop_batch_e2e_second",
      "params": {},
      "identifier": "second_child",
      "reason": "Execute the second independent child"
    }
  ]
}`)
				}

				// This is the important join boundary: the very next primary
				// decision may only begin after both callbacks and ordered task
				// commits have settled.
				if got := atomic.LoadInt32(&finished); got != 2 {
					return nil, fmt.Errorf("primary decision %d ran before batch join: finished=%d", decision, got)
				}
				if capturedTask == nil {
					return nil, fmt.Errorf("primary decision %d has no captured task", decision)
				}
				committed := capturedTask.GetAllToolCallResults()
				if len(committed) != 2 || committed[0].Name != firstToolName || committed[1].Name != secondToolName {
					return nil, fmt.Errorf("task results were not committed in model order: %#v", committed)
				}
				if decision > 3 {
					return nil, fmt.Errorf("unexpected extra primary decision %d", decision)
				}
				// Production finish uses a two-step soft-TODO checkpoint. Decision
				// 2 is the required immediate post-join finish; decision 3 confirms it.
				return respond(config, `{"@action":"finish","identifier":"finish_after_batch"}`)

			default:
				return nil, fmt.Errorf("unexpected AI prompt in direct batch main-loop E2E: %.200s", prompt)
			}
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop(
		"direct-tool-batch-main-loop-e2e",
		react,
		reactloops.WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
			capturedTask = task
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("direct-tool-batch-main-loop-e2e-task", ctx, "run both independent checks"))

	require.Equal(t, int32(2), atomic.LoadInt32(&started))
	require.Equal(t, int32(2), atomic.LoadInt32(&finished))
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActive), "both callbacks must overlap")
	require.Equal(t, int32(3), atomic.LoadInt32(&primaryDecisions), "batch, finish request, finish confirmation")
	require.Positive(t, atomic.LoadInt32(&satisfactionChecks))

	completionMu.Lock()
	completionSnapshot := append([]string(nil), completionOrder...)
	completionMu.Unlock()
	require.Equal(t, []string{secondToolName, firstToolName}, completionSnapshot, "workers must actually settle out of order")

	require.NotNil(t, capturedTask)
	committed := capturedTask.GetAllToolCallResults()
	require.Len(t, committed, 2)
	require.Equal(t, []string{firstToolName, secondToolName}, []string{committed[0].Name, committed[1].Name})

	history := loop.GetAllExistedActionRecord()
	require.Len(t, history, 3)
	require.Equal(t, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, history[0].ActionType)
	require.Equal(t, []string{firstToolName, secondToolName}, history[0].ToolNames)
	require.Equal(t, 2, history[0].ToolCallCount)
	require.Equal(t, 2, history[0].ExecutedToolCallCount)
	require.Equal(t, "finish", history[1].ActionType)
	require.Equal(t, "finish", history[2].ActionType)
}

// TestReActLoop_RequireToolBatchPrimaryDecisionConcurrentOrderedE2E covers the
// complete production path for the require form of a native tool batch:
//
//	model response -> stream parser -> action verifier/handler -> concurrent
//	parameter transactions -> concurrent plugin callbacks -> ordered task
//	commit -> the next primary decision.
//
// The two barriers are intentional correctness assertions, not timing hints.
// A serial parameter generator or a serial plugin scheduler cannot pass them.
// The second plugin is also forced to settle first, proving that task/history
// order follows the model array rather than goroutine completion order.
func TestReActLoop_RequireToolBatchPrimaryDecisionConcurrentOrderedE2E(t *testing.T) {
	const (
		firstToolName    = "main_loop_require_batch_e2e_first"
		secondToolName   = "main_loop_require_batch_e2e_second"
		firstParamValue  = "params-belong-only-to-first-child"
		secondParamValue = "params-belong-only-to-second-child"
	)

	var paramActive int32
	var paramMaxActive int32
	var paramStarted int32
	var paramFinished int32
	var callbackActive int32
	var callbackMaxActive int32
	var callbackStarted int32
	var callbackFinished int32
	var primaryDecisions int32
	var satisfactionChecks int32

	allParamTransactionsStarted := make(chan struct{})
	allToolCallbacksStarted := make(chan struct{})
	secondToolCallbackFinished := make(chan struct{})
	var closeAllParamsStarted sync.Once
	var closeAllCallbacksStarted sync.Once
	var closeSecondCallbackFinished sync.Once

	var observedParamsMu sync.Mutex
	observedParams := make(map[string]string, 2)
	var completionMu sync.Mutex
	completionOrder := make([]string, 0, 2)

	updateMax := func(target *int32, current int32) {
		for {
			old := atomic.LoadInt32(target)
			if current <= old || atomic.CompareAndSwapInt32(target, old, current) {
				return
			}
		}
	}

	newTool := func(name string, modelIndex int, expectedParam string) *aitool.Tool {
		tool, err := aitool.New(
			name,
			aitool.WithStringParam("child_value", aitool.WithParam_Required(true)),
			aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
				current := atomic.AddInt32(&callbackActive, 1)
				updateMax(&callbackMaxActive, current)
				defer atomic.AddInt32(&callbackActive, -1)

				gotParam := params.GetString("child_value")
				observedParamsMu.Lock()
				observedParams[name] = gotParam
				observedParamsMu.Unlock()

				if atomic.AddInt32(&callbackStarted, 1) == 2 {
					closeAllCallbacksStarted.Do(func() { close(allToolCallbacksStarted) })
				}
				select {
				case <-allToolCallbacksStarted:
				case <-time.After(5 * time.Second):
					return nil, fmt.Errorf("require batch tool callbacks did not overlap")
				}

				if gotParam != expectedParam {
					return nil, fmt.Errorf("%s received another child's params: got %q, want %q", name, gotParam, expectedParam)
				}

				if modelIndex == 0 {
					select {
					case <-secondToolCallbackFinished:
					case <-time.After(5 * time.Second):
						return nil, fmt.Errorf("second require-batch callback did not finish first")
					}
				}

				completionMu.Lock()
				completionOrder = append(completionOrder, name)
				completionMu.Unlock()
				atomic.AddInt32(&callbackFinished, 1)
				if modelIndex == 1 {
					closeSecondCallbackFinished.Do(func() { close(secondToolCallbackFinished) })
				}
				return name + ":" + gotParam, nil
			}),
		)
		require.NoError(t, err)
		return tool
	}

	firstTool := newTool(firstToolName, 0, firstParamValue)
	secondTool := newTool(secondToolName, 1, secondParamValue)
	var capturedTask aicommon.AIStatefulTask

	respond := func(config aicommon.AICallerConfigIf, raw string) (*aicommon.AIResponse, error) {
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(raw))
		response.Close()
		return response, nil
	}

	assertJoinedAndCommitted := func(boundary string) error {
		if got := atomic.LoadInt32(&paramFinished); got != 2 {
			return fmt.Errorf("%s ran before both parameter transactions finished: finished=%d", boundary, got)
		}
		if got := atomic.LoadInt32(&callbackFinished); got != 2 {
			return fmt.Errorf("%s ran before both tool callbacks finished: finished=%d", boundary, got)
		}
		if capturedTask == nil {
			return fmt.Errorf("%s has no captured task", boundary)
		}
		committed := capturedTask.GetAllToolCallResults()
		if len(committed) != 2 || committed[0].Name != firstToolName || committed[1].Name != secondToolName {
			return fmt.Errorf("%s observed task results outside model order: %#v", boundary, committed)
		}
		return nil
	}

	react, err := aireact.NewTestReAct(
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithTools(firstTool, secondTool),
		aicommon.WithAgreeYOLO(),
		aicommon.WithToolBatchParamConcurrency(2),
		aicommon.WithToolBatchInvokeConcurrency(2),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			switch {
			case aicommon.IsVerifySatisfactionPrompt(prompt):
				atomic.AddInt32(&satisfactionChecks, 1)
				if joinErr := assertJoinedAndCommitted("satisfaction verification"); joinErr != nil {
					return nil, joinErr
				}
				return respond(config, `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"both generated calls completed"}`)

			case aicommon.IsToolParamGenPromptForTool(prompt, firstToolName):
				current := atomic.AddInt32(&paramActive, 1)
				updateMax(&paramMaxActive, current)
				defer atomic.AddInt32(&paramActive, -1)
				if atomic.AddInt32(&paramStarted, 1) == 2 {
					closeAllParamsStarted.Do(func() { close(allParamTransactionsStarted) })
				}
				select {
				case <-allParamTransactionsStarted:
				case <-request.GetContext().Done():
					return nil, request.GetContext().Err()
				case <-time.After(5 * time.Second):
					return nil, fmt.Errorf("require batch parameter transactions did not overlap")
				}
				atomic.AddInt32(&paramFinished, 1)
				return respond(config, `{"@action":"call-tool","params":{"child_value":"`+firstParamValue+`"}}`)

			case aicommon.IsToolParamGenPromptForTool(prompt, secondToolName):
				current := atomic.AddInt32(&paramActive, 1)
				updateMax(&paramMaxActive, current)
				defer atomic.AddInt32(&paramActive, -1)
				if atomic.AddInt32(&paramStarted, 1) == 2 {
					closeAllParamsStarted.Do(func() { close(allParamTransactionsStarted) })
				}
				select {
				case <-allParamTransactionsStarted:
				case <-request.GetContext().Done():
					return nil, request.GetContext().Err()
				case <-time.After(5 * time.Second):
					return nil, fmt.Errorf("require batch parameter transactions did not overlap")
				}
				atomic.AddInt32(&paramFinished, 1)
				return respond(config, `{"@action":"call-tool","params":{"child_value":"`+secondParamValue+`"}}`)

			case aicommon.IsPrimaryDecisionPrompt(prompt):
				decision := atomic.AddInt32(&primaryDecisions, 1)
				if decision == 1 {
					return respond(config, `{
  "@action": "require_tool",
  "identifier": "parallel_require_main_loop_e2e",
  "human_readable_thought": "Generate parameters and run two independent tools concurrently",
  "tool_require_calls": [
    {
      "tool_name": "main_loop_require_batch_e2e_first",
      "identifier": "first_require_child",
      "reason": "Generate isolated parameters for the first child"
    },
    {
      "tool_name": "main_loop_require_batch_e2e_second",
      "identifier": "second_require_child",
      "reason": "Generate isolated parameters for the second child"
    }
  ]
}`)
				}

				if joinErr := assertJoinedAndCommitted(fmt.Sprintf("primary decision %d", decision)); joinErr != nil {
					return nil, joinErr
				}
				if decision > 3 {
					return nil, fmt.Errorf("unexpected extra primary decision %d", decision)
				}
				return respond(config, `{"@action":"finish","identifier":"finish_after_require_batch"}`)

			default:
				return nil, fmt.Errorf("unexpected AI prompt in require batch main-loop E2E: %.200s", prompt)
			}
		}),
	)
	require.NoError(t, err)

	loop, err := reactloops.NewReActLoop(
		"require-tool-batch-main-loop-e2e",
		react,
		reactloops.WithOnTaskCreated(func(task aicommon.AIStatefulTask) {
			capturedTask = task
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("require-tool-batch-main-loop-e2e-task", ctx, "generate isolated params and run both independent tools"))

	require.Equal(t, int32(2), atomic.LoadInt32(&paramStarted))
	require.Equal(t, int32(2), atomic.LoadInt32(&paramFinished))
	require.Equal(t, int32(2), atomic.LoadInt32(&paramMaxActive), "both parameter AI transactions must overlap")
	require.Equal(t, int32(2), atomic.LoadInt32(&callbackStarted))
	require.Equal(t, int32(2), atomic.LoadInt32(&callbackFinished))
	require.Equal(t, int32(2), atomic.LoadInt32(&callbackMaxActive), "both plugin callbacks must overlap")
	require.Equal(t, int32(3), atomic.LoadInt32(&primaryDecisions), "batch, finish request, finish confirmation")
	require.Positive(t, atomic.LoadInt32(&satisfactionChecks))

	observedParamsMu.Lock()
	observedParamsSnapshot := make(map[string]string, len(observedParams))
	for key, value := range observedParams {
		observedParamsSnapshot[key] = value
	}
	observedParamsMu.Unlock()
	require.Equal(t, map[string]string{
		firstToolName:  firstParamValue,
		secondToolName: secondParamValue,
	}, observedParamsSnapshot, "generated parameters must stay bound to their own child")

	completionMu.Lock()
	completionSnapshot := append([]string(nil), completionOrder...)
	completionMu.Unlock()
	require.Equal(t, []string{secondToolName, firstToolName}, completionSnapshot, "workers must actually settle out of order")

	require.NotNil(t, capturedTask)
	committed := capturedTask.GetAllToolCallResults()
	require.Len(t, committed, 2)
	require.Equal(t, []string{firstToolName, secondToolName}, []string{committed[0].Name, committed[1].Name})

	history := loop.GetAllExistedActionRecord()
	require.Len(t, history, 3)
	require.Equal(t, schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL, history[0].ActionType)
	require.Equal(t, []string{firstToolName, secondToolName}, history[0].ToolNames)
	require.Equal(t, 2, history[0].ToolCallCount)
	require.Equal(t, 2, history[0].ExecutedToolCallCount)
	require.Equal(t, "finish", history[1].ActionType)
	require.Equal(t, "finish", history[2].ActionType)
}
