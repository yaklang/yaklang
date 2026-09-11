package aireact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestExecuteToolBatchNestedObjectsRemainValidAndIsolated(t *testing.T) {
	tool, err := aitool.New("nested_batch_tool",
		aitool.WithStructParam("query", nil, aitool.WithStringParam("value")),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			params.GetObject("query").Set("value", "changed")
			return "ok", nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	shared := map[string]any{"value": "original"}
	result, err := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "first", Params: aitool.InvokeParams{"query": shared}},
			{Mode: aicommon.ToolCallModeDirect, ToolName: tool.Name, Reason: "second", Params: aitool.InvokeParams{"query": shared}},
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Outcomes, 2)
	for _, outcome := range result.Outcomes {
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage, "%+v", outcome)
		require.True(t, outcome.Result.Success)
	}
	require.Equal(t, "original", shared["value"], "batch children must not mutate the source payload")
}

func TestExecuteToolBatchParamPromptIncludesIndividualIntent(t *testing.T) {
	tool, err := aitool.New("intent_batch_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			return params.GetInt("id"), nil
		}),
	)
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		id := 0
		for _, candidate := range []int{404, 500} {
			if strings.Contains(req.GetPrompt(), fmt.Sprintf("Only request status %d", candidate)) &&
				strings.Contains(req.GetPrompt(), fmt.Sprintf("request_%d", candidate)) &&
				strings.Contains(req.GetPrompt(), fmt.Sprintf("Expect HTTP %d", candidate)) {
				id = candidate
			}
		}
		response := config.NewAIResponse()
		response.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`{"@action":"call-tool","params":{"id":%d}}`, id)))
		response.Close()
		return response, nil
	})
	result, err := react.ExecuteToolBatch(context.Background(), react.config.DefaultTask, &aicommon.ToolBatchRequest{
		Calls: []aicommon.ToolBatchCall{
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "Only request status 404", Identifier: "request_404", Expectations: "Expect HTTP 404"},
			{Mode: aicommon.ToolCallModeRequire, ToolName: tool.Name, Reason: "Only request status 500", Identifier: "request_500", Expectations: "Expect HTTP 500"},
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Outcomes, 2)
	for i, id := range []int64{404, 500} {
		require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[i].Stage)
		require.Equal(t, id, batchTestResultParamInt(t, result.Outcomes[i].Result, "id"))
	}
}
