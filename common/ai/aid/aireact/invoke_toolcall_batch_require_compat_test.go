package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

var requireBatchContentNoncePattern = regexp.MustCompile(`<\|TOOL_PARAM_content_([A-Za-z0-9]+)\|>`)

func requireBatchContentNonce(prompt string) (string, error) {
	matched := requireBatchContentNoncePattern.FindStringSubmatch(prompt)
	if len(matched) != 2 {
		return "", fmt.Errorf("TOOL_PARAM_content nonce not found in parameter prompt")
	}
	return matched[1], nil
}

func requireBatchParamResponse(
	config aicommon.AICallerConfigIf,
	nonce string,
	jsonValue string,
	aitagValue string,
) (*aicommon.AIResponse, error) {
	raw := fmt.Sprintf(`{"@action":"call-tool","params":{"content":%q}}
<|TOOL_PARAM_content_%s|>
%s
<|TOOL_PARAM_content_END_%s|>`, jsonValue, nonce, aitagValue, nonce)
	response := config.NewAIResponse()
	response.EmitOutputStream(bytes.NewBufferString(raw))
	response.Close()
	return response, nil
}

func requireBatchResultParams(t *testing.T, result *aitool.ToolResult) aitool.InvokeParams {
	t.Helper()
	require.NotNil(t, result)
	switch params := result.Param.(type) {
	case aitool.InvokeParams:
		return params
	case map[string]any:
		return aitool.InvokeParams(params)
	default:
		t.Fatalf("unexpected result param type %T", result.Param)
		return nil
	}
}

func TestExecuteToolBatch_RequireParamTransactionsRunConcurrentlyWithIsolatedAITAG(t *testing.T) {
	const (
		firstValue  = "first child AITAG\nkeeps its own multiline body"
		secondValue = "second child AITAG\nkeeps its own multiline body"
	)

	firstTool, err := aitool.New(
		"batch_require_aitag_first_tool",
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return params.GetString("content"), nil
		}),
	)
	require.NoError(t, err)
	secondTool, err := aitool.New(
		"batch_require_aitag_second_tool",
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			return params.GetString("content"), nil
		}),
	)
	require.NoError(t, err)

	var arrived int32
	var active int32
	var maxActive int32
	allArrived := make(chan struct{})
	var nonceMu sync.Mutex
	nonces := make(map[string]string, 2)

	callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := request.GetPrompt()
		toolName := ""
		value := ""
		switch {
		case isToolParamGenerationPrompt(prompt, firstTool.Name):
			toolName = firstTool.Name
			value = firstValue
		case isToolParamGenerationPrompt(prompt, secondTool.Name):
			toolName = secondTool.Name
			value = secondValue
		default:
			return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
		}

		nonce, nonceErr := requireBatchContentNonce(prompt)
		if nonceErr != nil {
			return nil, nonceErr
		}
		nonceMu.Lock()
		nonces[toolName] = nonce
		nonceMu.Unlock()

		current := atomic.AddInt32(&active, 1)
		defer atomic.AddInt32(&active, -1)
		for {
			old := atomic.LoadInt32(&maxActive)
			if current <= old || atomic.CompareAndSwapInt32(&maxActive, old, current) {
				break
			}
		}
		if atomic.AddInt32(&arrived, 1) == 2 {
			close(allArrived)
		}
		select {
		case <-allArrived:
		case <-request.GetContext().Done():
			return nil, request.GetContext().Err()
		case <-time.After(5 * time.Second):
			return nil, fmt.Errorf("parameter transactions did not overlap")
		}

		return requireBatchParamResponse(config, nonce, "JSON value must be overridden", value)
	}

	react, err := NewTestReAct(
		aicommon.WithAICallback(callback),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithTools(firstTool, secondTool),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true),
	)
	require.NoError(t, err)
	react.config.SetConfig(aicommon.ConfigKeyToolBatchParamConcurrency, 2)

	result, execErr := react.ExecuteToolBatch(
		context.Background(),
		react.config.DefaultTask,
		&aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: firstTool.Name, Reason: "generate first child params"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: secondTool.Name, Reason: "generate second child params"},
		}},
	)
	require.NoError(t, execErr)
	require.Len(t, result.Outcomes, 2)
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActive), "both parameter AI transactions must be in flight together")
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[0].Stage)
	require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[1].Stage)
	require.Equal(t, firstValue, requireBatchResultParams(t, result.Outcomes[0].Result).GetString("content"))
	require.Equal(t, secondValue, requireBatchResultParams(t, result.Outcomes[1].Result).GetString("content"))
	firstData := fmt.Sprint(result.Outcomes[0].Result.Data)
	secondData := fmt.Sprint(result.Outcomes[1].Result.Data)
	require.Contains(t, firstData, firstValue)
	require.NotContains(t, firstData, secondValue)
	require.Contains(t, secondData, secondValue)
	require.NotContains(t, secondData, firstValue)

	nonceMu.Lock()
	firstNonce := nonces[firstTool.Name]
	secondNonce := nonces[secondTool.Name]
	nonceMu.Unlock()
	require.NotEmpty(t, firstNonce)
	require.NotEmpty(t, secondNonce)
	require.NotEqual(t, firstNonce, secondNonce, "each concurrent parameter parser must use its own prompt nonce")
}

func TestExecuteToolBatch_RequireFreshRuntimeReplaysParamAndToolCheckpoints(t *testing.T) {
	var aiCalls int32
	var firstInvokes int32
	var secondInvokes int32

	firstTool, err := aitool.New(
		"batch_require_replay_first_tool",
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&firstInvokes, 1)
			return params.GetString("content"), nil
		}),
	)
	require.NoError(t, err)
	secondTool, err := aitool.New(
		"batch_require_replay_second_tool",
		aitool.WithStringParam("content", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			atomic.AddInt32(&secondInvokes, 1)
			return params.GetString("content"), nil
		}),
	)
	require.NoError(t, err)

	callback := func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := request.GetPrompt()
		value := ""
		switch {
		case isToolParamGenerationPrompt(prompt, firstTool.Name):
			value = "checkpointed first AITAG body"
		case isToolParamGenerationPrompt(prompt, secondTool.Name):
			value = "checkpointed second AITAG body"
		default:
			return nil, fmt.Errorf("unexpected AI call: caller=%s", request.GetCallerLabel())
		}
		nonce, nonceErr := requireBatchContentNonce(prompt)
		if nonceErr != nil {
			return nil, nonceErr
		}
		atomic.AddInt32(&aiCalls, 1)
		// Keep the JSON slot empty so a fresh runtime can safely recover the one
		// checkpointed AITAG block even though its newly generated prompt has a
		// different nonce. A stale-nonce AITAG is intentionally not allowed to
		// overwrite a non-empty JSON value: that conservative rule prevents an old
		// block from contaminating a newly generated proposal.
		return requireBatchParamResponse(config, nonce, "", value)
	}

	runtimeID := "batch-require-replay-" + ksuid.New().String()
	workdir := t.TempDir()
	const sequenceStart int64 = 12100
	newRuntime := func() *ReAct {
		react, runtimeErr := NewTestReAct(
			aicommon.WithID(runtimeID),
			aicommon.WithSequence(sequenceStart),
			aicommon.WithAICallback(callback),
			aicommon.WithAITransactionAutoRetry(1),
			aicommon.WithTools(firstTool, secondTool),
			aicommon.WithWorkdir(workdir),
			aicommon.WithAgreeYOLO(),
			aicommon.WithDisableToolCallerIntervalReview(true),
		)
		require.NoError(t, runtimeErr)
		return react
	}
	newRequest := func() *aicommon.ToolBatchRequest {
		return &aicommon.ToolBatchRequest{Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: firstTool.Name, Reason: "checkpoint first require child"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: secondTool.Name, Reason: "checkpoint second require child"},
		}}
	}

	firstRuntime := newRuntime()
	firstRequest := newRequest()
	firstResult, firstErr := firstRuntime.ExecuteToolBatch(context.Background(), firstRuntime.config.DefaultTask, firstRequest)
	require.NoError(t, firstErr)
	require.Len(t, firstResult.Outcomes, 2)
	require.Equal(t, int32(2), atomic.LoadInt32(&aiCalls), "the first run generates params once per require child")
	require.Equal(t, int32(1), atomic.LoadInt32(&firstInvokes))
	require.Equal(t, int32(1), atomic.LoadInt32(&secondInvokes))
	require.Equal(t, "checkpointed first AITAG body", requireBatchResultParams(t, firstResult.Outcomes[0].Result).GetString("content"))
	require.Equal(t, "checkpointed second AITAG body", requireBatchResultParams(t, firstResult.Outcomes[1].Result).GetString("content"))
	firstData := []any{firstResult.Outcomes[0].Result.Data, firstResult.Outcomes[1].Result.Data}
	firstBatchID := firstRequest.BatchID
	firstCallIDs := []string{firstRequest.Calls[0].ExecutionCallID, firstRequest.Calls[1].ExecutionCallID}

	// Rebuild both the runtime and request from scratch. Parameter prompts get new
	// random nonces, while the reserved transaction/tool sequences stay stable.
	// The saved AITAG response is reparsed from the AI checkpoint and each finished
	// tool checkpoint suppresses the corresponding plugin callback.
	secondRuntime := newRuntime()
	secondRequest := newRequest()
	secondResult, secondErr := secondRuntime.ExecuteToolBatch(context.Background(), secondRuntime.config.DefaultTask, secondRequest)
	require.NoError(t, secondErr)
	require.Len(t, secondResult.Outcomes, 2)
	require.Equal(t, int32(2), atomic.LoadInt32(&aiCalls), "fresh-runtime replay must not call the parameter-generation AI again")
	require.Equal(t, int32(1), atomic.LoadInt32(&firstInvokes), "fresh-runtime replay must not invoke the first plugin again")
	require.Equal(t, int32(1), atomic.LoadInt32(&secondInvokes), "fresh-runtime replay must not invoke the second plugin again")
	require.Equal(t, firstBatchID, secondRequest.BatchID)
	require.Equal(t, firstCallIDs, []string{secondRequest.Calls[0].ExecutionCallID, secondRequest.Calls[1].ExecutionCallID})
	for index, expected := range []string{"checkpointed first AITAG body", "checkpointed second AITAG body"} {
		require.Equalf(t, aicommon.ToolCallStageDone, secondResult.Outcomes[index].Stage,
			"replayed outcome[%d]: result=%+v err=%v", index, secondResult.Outcomes[index].Result, secondResult.Outcomes[index].Err)
		require.NotNil(t, secondResult.Outcomes[index].Result)
		require.True(t, secondResult.Outcomes[index].Result.Success)
		require.Equal(t, expected, requireBatchResultParams(t, secondResult.Outcomes[index].Result).GetString("content"))
		require.Equal(t, firstData[index], secondResult.Outcomes[index].Result.Data,
			"tool checkpoint replay must preserve the first run's finalized artifact-backed result")
	}
}
