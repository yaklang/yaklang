package aireact

import (
	"context"
	"io"
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
			{ToolName: tool.Name, Reason: "first", Params: aitool.InvokeParams{"query": shared}},
			{ToolName: tool.Name, Reason: "second", Params: aitool.InvokeParams{"query": shared}},
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
