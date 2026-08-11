package aireact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newBatchTestReAct(t *testing.T, tool *aitool.Tool, callback aicommon.AICallbackType) *ReAct {
	t.Helper()
	if callback == nil {
		callback = func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}
	}
	react, err := NewTestReAct(
		aicommon.WithAICallback(callback),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)
	return react
}

func batchTestResultParamInt(t *testing.T, result *aitool.ToolResult, key string) int64 {
	t.Helper()
	require.NotNil(t, result)
	switch params := result.Param.(type) {
	case aitool.InvokeParams:
		return params.GetInt(key)
	case map[string]any:
		return aitool.InvokeParams(params).GetInt(key)
	default:
		t.Fatalf("unexpected result param type %T", result.Param)
		return 0
	}
}

func TestExecuteToolBatch_RejectsScalarRequest(t *testing.T) {
	tool, err := aitool.New(
		"batch_min_items_tool",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)

	result, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name}},
	})
	require.Nil(t, result)
	require.ErrorContains(t, execErr, "at least 2")
}

func TestExecuteToolBatch_DirectBoundedConcurrencyAndOrderedCommit(t *testing.T) {
	var active int32
	var maxActive int32
	var completionMu sync.Mutex
	var completion []int

	tool, err := aitool.New(
		"batch_order_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("delay_ms", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			current := atomic.AddInt32(&active, 1)
			for {
				old := atomic.LoadInt32(&maxActive)
				if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
					break
				}
			}
			time.Sleep(time.Duration(params.GetInt("delay_ms")) * time.Millisecond)
			atomic.AddInt32(&active, -1)
			id := int(params.GetInt("id"))
			completionMu.Lock()
			completion = append(completion, id)
			completionMu.Unlock()
			return id, nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 2)

	request := &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "test call 0", Params: aitool.InvokeParams{"id": 0, "delay_ms": 180}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "test call 1", Params: aitool.InvokeParams{"id": 1, "delay_ms": 20}},
		{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "test call 2", Params: aitool.InvokeParams{"id": 2, "delay_ms": 20}},
	}}

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActive), "invoke concurrency must use its own configured bound")
	require.NotEqual(t, []int{0, 1, 2}, completion, "test must observe out-of-order worker completion")

	for i, outcome := range batchResult.Outcomes {
		require.Equal(t, i, outcome.Index)
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
		require.NotNil(t, outcome.Result)
		require.Equal(t, int64(i), batchTestResultParamInt(t, outcome.Result, "id"))
	}
	committed := react.config.DefaultTask.GetAllToolCallResults()
	require.Len(t, committed, 3)
	for i, item := range committed {
		require.Equal(t, int64(i), batchTestResultParamInt(t, item, "id"), "task commit order must follow the model array")
	}
}

func TestExecuteToolBatch_DirectAdmissionFailureStartsNothing(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_validate_tool",
		aitool.WithStringParam("required_value", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "valid test call", Params: aitool.InvokeParams{"required_value": "valid"}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "invalid test call", Params: aitool.InvokeParams{}},
		},
	})
	require.NoError(t, execErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&invoked))
	require.Equal(t, aicommon.ToolCallStageCancelled, batchResult.Outcomes[0].Stage)
	require.Equal(t, aicommon.ToolCallStageValidationFailed, batchResult.Outcomes[1].Stage)
}

func TestExecuteToolBatch_FreshRequestReplaysStableCheckpointIdentity(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_checkpoint_replay_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "executed", nil
		}),
	)
	require.NoError(t, err)
	runtimeID := "batch-replay-" + ksuid.New().String()
	const sequenceStart int64 = 9100
	newRuntime := func() *ReAct {
		react, runtimeErr := NewTestReAct(
			aicommon.WithID(runtimeID),
			aicommon.WithSequence(sequenceStart),
			aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				return nil, fmt.Errorf("unexpected AI call")
			}),
			aicommon.WithTools(tool),
			aicommon.WithWorkdir(t.TempDir()),
			aicommon.WithAgreeYOLO(),
			aicommon.WithDisableToolCallerIntervalReview(true),
		)
		require.NoError(t, runtimeErr)
		return react
	}
	newRequest := func() *aicommon.ToolBatchRequest {
		return &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "replay call 0", Params: aitool.InvokeParams{"id": 0}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "replay call 1", Params: aitool.InvokeParams{"id": 1}},
		}}
	}

	firstRuntime := newRuntime()
	firstRequest := newRequest()
	firstResult, firstErr := firstRuntime.ExecuteToolBatch(context.Background(), firstRuntime.config.DefaultTask, firstRequest)
	require.NoError(t, firstErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked))
	require.Len(t, firstResult.Outcomes, 2)
	firstBatchID := firstRequest.BatchID
	firstCallIDs := []string{firstRequest.Calls[0].ExecutionCallID, firstRequest.Calls[1].ExecutionCallID}

	// Simulate recovery by constructing a new runtime and a newly parsed request:
	// neither carries IDs from the first in-memory objects.
	secondRuntime := newRuntime()
	secondRequest := newRequest()
	secondResult, secondErr := secondRuntime.ExecuteToolBatch(context.Background(), secondRuntime.config.DefaultTask, secondRequest)
	require.NoError(t, secondErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked), "finished checkpoints must replay without invoking plugins again")
	require.Equal(t, firstBatchID, secondRequest.BatchID)
	require.Equal(t, firstCallIDs, []string{secondRequest.Calls[0].ExecutionCallID, secondRequest.Calls[1].ExecutionCallID})
	for _, outcome := range secondResult.Outcomes {
		require.NotNil(t, outcome.Result)
		require.True(t, outcome.Result.Success, "replayed result: %+v; outcome error: %v", outcome.Result, outcome.Err)
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
	}
}

func TestExecuteToolBatch_ReviewCardsFollowModelArrayOrder(t *testing.T) {
	var invoked int32
	var reviewMu sync.Mutex
	var reviewedIDs []int
	input := make(chan *ypb.AIInputEvent, 4)
	tool, err := aitool.New(
		"batch_ordered_review_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	type reviewMaterial struct {
		ID     string `json:"id"`
		Params struct {
			ID int `json:"id"`
		} `json:"params"`
	}
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventInputChan(input),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				return
			}
			var material reviewMaterial
			if json.Unmarshal(event.Content, &material) != nil {
				return
			}
			reviewMu.Lock()
			reviewedIDs = append(reviewedIDs, material.Params.ID)
			reviewMu.Unlock()
			input <- &ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        material.ID,
				InteractiveJSONInput: `{"suggestion":"continue"}`,
			}
		}),
	)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batchResult, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "review call 0", Params: aitool.InvokeParams{"id": 0}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "review call 1", Params: aitool.InvokeParams{"id": 1}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "review call 2", Params: aitool.InvokeParams{"id": 2}},
		},
	})
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(3), atomic.LoadInt32(&invoked))
	reviewMu.Lock()
	require.Equal(t, []int{0, 1, 2}, reviewedIDs)
	reviewMu.Unlock()
}

func TestExecuteToolBatch_ReviewCheckpointIdentityMismatchIsRejected(t *testing.T) {
	var invoked int32
	tool, err := aitool.New(
		"batch_review_checkpoint_tool",
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	runtimeID := "batch-review-checkpoint-" + ksuid.New().String()
	const sequenceStart int64 = 9600
	react, err := NewTestReAct(
		aicommon.WithID(runtimeID),
		aicommon.WithSequence(sequenceStart),
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)
	// Allocation layout is batch seed, then param/review/tool/watcher/result for
	// each child. Seed a conflicting review at the reserved child-0 review seq.
	reviewCheckpoint := react.config.CreateReviewCheckpoint(sequenceStart + 2)
	require.NoError(t, react.config.SubmitCheckpointRequest(reviewCheckpoint, map[string]any{
		"batch_id":     "wrong-batch",
		"call_index":   0,
		"call_tool_id": "wrong-call",
		"tool":         tool.Name,
		"params":       aitool.InvokeParams{},
	}))
	reviewCheckpoint2 := react.config.CreateReviewCheckpoint(sequenceStart + 7)
	require.NoError(t, react.config.SubmitCheckpointRequest(reviewCheckpoint2, map[string]any{
		"batch_id":     "wrong-batch",
		"call_index":   1,
		"call_tool_id": "wrong-call-2",
		"tool":         tool.Name,
		"params":       aitool.InvokeParams{},
	}))

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "identity mismatch 0", Params: aitool.InvokeParams{}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "identity mismatch 1", Params: aitool.InvokeParams{}},
		},
	})
	require.NoError(t, execErr)
	require.Equal(t, int32(0), atomic.LoadInt32(&invoked))
	require.Equal(t, aicommon.ToolCallStagePrepareFailed, batchResult.Outcomes[0].Stage)
	require.ErrorContains(t, batchResult.Outcomes[0].Err, "review checkpoint identity mismatch")
}

func TestExecuteToolBatch_RequireBoundsParamGenerationSeparately(t *testing.T) {
	var activeAI int32
	var maxActiveAI int32
	var invoked int32
	twoAIActive := make(chan struct{})
	var releaseAI sync.Once
	type childContextMarker struct{}
	tool, err := aitool.New(
		"batch_require_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)

	var missingChildContext int32
	callback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if req.GetContext() == nil || req.GetContext().Value(childContextMarker{}) != "batch-child" {
			atomic.StoreInt32(&missingChildContext, 1)
		}
		current := atomic.AddInt32(&activeAI, 1)
		for {
			old := atomic.LoadInt32(&maxActiveAI)
			if current <= old || atomic.CompareAndSwapInt32(&maxActiveAI, old, current) {
				break
			}
		}
		if current >= 2 {
			releaseAI.Do(func() { close(twoAIActive) })
		}
		select {
		case <-twoAIActive:
		case <-time.After(3 * time.Second):
		}
		atomic.AddInt32(&activeAI, -1)
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(`{"@action":"call-tool","identifier":"batch","params":{"id":1}}`))
		response.Close()
		return response, nil
	}
	react := newBatchTestReAct(t, tool, callback)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 2)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 3)

	batchCtx := context.WithValue(context.Background(), childContextMarker{}, "batch-child")
	batchResult, execErr := react.ExecuteToolBatch(batchCtx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "first"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "second"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "third"},
		},
	})
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActiveAI))
	require.Equal(t, int32(0), atomic.LoadInt32(&missingChildContext), "parameter AI requests must carry the child context")
	require.Equal(t, int32(3), atomic.LoadInt32(&invoked))
	for _, outcome := range batchResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
	}
}

func TestExecuteToolBatch_RequireParamFailureIsAllSettled(t *testing.T) {
	var aiCalls int32
	var invoked int32
	tool, err := aitool.New(
		"batch_require_all_settled_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	callback := func(config aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if atomic.AddInt32(&aiCalls, 1) == 1 {
			return nil, fmt.Errorf("synthetic parameter generation failure")
		}
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(`{"@action":"call-tool","params":{"id":1}}`))
		response.Close()
		return response, nil
	}
	react := newBatchTestReAct(t, tool, callback)
	react.config.AiAutoRetry = 1
	react.config.AiTransactionAutoRetry = 1
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 3)

	batchResult, execErr := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "first"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "second"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "third"},
		},
	})
	require.NoError(t, execErr)
	require.Len(t, batchResult.Outcomes, 3)
	require.Equal(t, int32(2), atomic.LoadInt32(&invoked), "one prepare failure must not cancel successful siblings")
	var preparedFailed, done int
	for _, outcome := range batchResult.Outcomes {
		switch outcome.Stage {
		case aicommon.ToolCallStagePrepareFailed:
			preparedFailed++
		case aicommon.ToolCallStageDone:
			done++
		}
	}
	require.Equal(t, 1, preparedFailed)
	require.Equal(t, 2, done)
}

func TestExecuteToolBatch_RequireParamCancellationDoesNotRetry(t *testing.T) {
	var aiCalls int32
	started := make(chan struct{})
	tool, err := aitool.New(
		"batch_require_cancel_params_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return "must not invoke", nil
		}),
	)
	require.NoError(t, err)
	callback := func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		if atomic.AddInt32(&aiCalls, 1) == 1 {
			close(started)
		}
		requestCtx := req.GetContext()
		if requestCtx == nil {
			return nil, fmt.Errorf("missing request context")
		}
		<-requestCtx.Done()
		return nil, requestCtx.Err()
	}
	react := newBatchTestReAct(t, tool, callback)
	react.config.AiAutoRetry = 5
	react.config.AiTransactionAutoRetry = 5
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var batchResult *aicommon.ToolBatchResult
	var execErr error
	go func() {
		batchResult, execErr = react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "cancel while generating params 0"},
				{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "cancel while generating params 1"},
			},
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("parameter AI callback did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("parameter transaction did not stop on child cancellation")
	}
	require.ErrorIs(t, execErr, context.Canceled)
	require.Equal(t, int32(1), atomic.LoadInt32(&aiCalls), "cancellation must not enter gateway or transaction retries")
	require.NotNil(t, batchResult)
	require.Equal(t, aicommon.ToolCallStageCancelled, batchResult.Outcomes[0].Stage)
}

func TestExecuteToolBatch_DirectAnswerCancelsWholeBatchBeforeAnyInvoke(t *testing.T) {
	var invoked int32
	var reviewCount int32
	input := make(chan *ypb.AIInputEvent, 4)
	tool, err := aitool.New(
		"batch_direct_answer_tool",
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&invoked, 1)
			return "must not run", nil
		}),
	)
	require.NoError(t, err)
	react, err := NewTestReAct(
		aicommon.WithAICallback(func(_ aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, fmt.Errorf("unexpected AI call")
		}),
		aicommon.WithTools(tool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithEventInputChan(input),
		aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			if event == nil || event.Type != schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
				return
			}
			atomic.AddInt32(&reviewCount, 1)
			input <- &ypb.AIInputEvent{
				IsInteractiveMessage: true,
				InteractiveId:        event.GetContentJSONPath("$.id"),
				InteractiveJSONInput: `{"suggestion":"direct_answer"}`,
			}
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	batchResult, execErr := react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "direct answer call 0", Params: aitool.InvokeParams{}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "direct answer call 1", Params: aitool.InvokeParams{}},
		},
	})
	require.NoError(t, execErr)
	require.True(t, batchResult.DirectlyAnswer)
	require.GreaterOrEqual(t, atomic.LoadInt32(&reviewCount), int32(1))
	require.Equal(t, int32(0), atomic.LoadInt32(&invoked), "the final barrier must keep every plugin callback at zero")
	for _, outcome := range batchResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageCancelled, outcome.Stage)
	}
}

func TestExecuteToolBatch_CancellationReachesRunningChildren(t *testing.T) {
	started := make(chan struct{}, 2)
	tool, err := aitool.New(
		"batch_cancel_tool",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, _ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var batchResult *aicommon.ToolBatchResult
	var execErr error
	go func() {
		batchResult, execErr = react.ExecuteToolBatch(ctx, react.config.DefaultTask, &aicommon.ToolBatchRequest{
			Calls: []aicommon.ToolBatchCall{
				{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "cancel call 0", Params: aitool.InvokeParams{}},
				{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "cancel call 1", Params: aitool.InvokeParams{}},
			},
		})
		close(done)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("tool child did not start")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("batch did not stop after cancellation")
	}
	require.ErrorIs(t, execErr, context.Canceled)
	require.NotNil(t, batchResult)
	for _, outcome := range batchResult.Outcomes {
		require.Equal(t, aicommon.ToolCallStageCancelled, outcome.Stage)
		if outcome.Result != nil {
			require.False(t, outcome.Result.Success)
		}
	}
	require.Empty(t, react.config.DefaultTask.GetAllToolCallResults(), "cancelled children must not be committed to task state")
	var finishedToolCheckpoints int
	require.NoError(t, react.config.GetDB().Model(&schema.AiCheckpoint{}).
		Where("coordinator_uuid = ? AND type = ? AND finished = ?", react.config.GetRuntimeId(), schema.AiCheckpointType_ToolCall, true).
		Count(&finishedToolCheckpoints).Error)
	require.Zero(t, finishedToolCheckpoints, "cancelled plugin executions must leave replayable unfinished checkpoints")
}

func TestToolBatchBarrier_DirectAnswerAbortsReadySibling(t *testing.T) {
	barrier := newToolBatchBarrier(2)
	invokeAcquired := int32(0)
	invokeGate := func(context.Context) (func(), error) {
		atomic.AddInt32(&invokeAcquired, 1)
		return func() {}, nil
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := barrier.wait(context.Background(), 0, invokeGate)
		errCh <- err
	}()
	barrier.arrive(1, true)
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, errToolBatchDirectAnswer)
	case <-time.After(time.Second):
		t.Fatal("ready sibling remained blocked")
	}
	require.Equal(t, int32(0), atomic.LoadInt32(&invokeAcquired))
}
