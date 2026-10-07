package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestExecuteToolCallGroupNestedObjectsRemainValidAndIsolated(t *testing.T) {
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
	result, err := react.ExecuteToolCallGroup(context.Background(), react.config.DefaultTask, &aicommon.ToolCallGroupRequest{
		Calls: []aicommon.ToolCallGroupCall{
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

func TestExecuteToolCallGroupExplicitParamsNeedNoAuxiliaryRequest(t *testing.T) {
	tool, err := aitool.New("explicit_group_tool", aitool.WithIntegerParam("id", aitool.WithParam_Required(true)), aitool.WithDangerousNoNeedUserReview(true), aitool.WithSimpleCallback(func(p aitool.InvokeParams, _, _ io.Writer) (any, error) { return p.GetInt("id"), nil }))
	require.NoError(t, err)
	var aiCalls int
	react := newBatchTestReAct(t, tool, func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		aiCalls++
		return nil, fmt.Errorf("unexpected auxiliary request: %s", req.GetCallerLabel())
	})
	result, err := react.ExecuteToolCallGroup(context.Background(), react.config.DefaultTask, &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
		{ToolName: tool.Name, Params: aitool.InvokeParams{"id": 404}, Reason: "request 404"},
		{ToolName: tool.Name, Params: aitool.InvokeParams{"id": 500}, Reason: "request 500"},
	}})
	require.NoError(t, err)
	require.Zero(t, aiCalls)
	require.Len(t, result.Outcomes, 2)
	for i, id := range []int64{404, 500} {
		require.Equal(t, aicommon.ToolCallStageDone, result.Outcomes[i].Stage)
		require.Equal(t, id, batchTestResultParamInt(t, result.Outcomes[i].Result, "id"))
	}
}

func TestExecuteToolCallGroupBusinessMetadataFieldsDoNotReplaceChildMetadata(t *testing.T) {
	tool, err := aitool.New("business_metadata_group_tool",
		aitool.WithStringParam("identifier", aitool.WithParam_Required(true)),
		aitool.WithStringParam("call_expectations", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			return params.GetString("identifier") + ":" + params.GetString("call_expectations"), nil
		}))
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	request := &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
		{ToolName: tool.Name, Identifier: "first_artifact", Expectations: "first invocation", Reason: "first call",
			Params: aitool.InvokeParams{"identifier": "first-business", "call_expectations": "first business"}},
		{ToolName: tool.Name, Identifier: "second_artifact", Expectations: "second invocation", Reason: "second call",
			Params: aitool.InvokeParams{"identifier": "second-business", "call_expectations": "second business"}},
	}}
	result, err := react.ExecuteToolCallGroup(context.Background(), react.config.DefaultTask, request)
	require.NoError(t, err)
	require.Len(t, result.Outcomes, 2)
	for i, outcome := range result.Outcomes {
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
		require.NotNil(t, outcome.Result)
		require.True(t, outcome.Result.Success, "%s", outcome.Result.Error)
		encoded, err := json.Marshal(outcome.Result.Param)
		require.NoError(t, err)
		var params aitool.InvokeParams
		require.NoError(t, json.Unmarshal(encoded, &params))
		require.Equal(t, request.Calls[i].Params, params)
	}
	dirs := batchArtifactDirs(t, react)
	require.Len(t, dirs, 2)
	identifiers := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		identifiers = append(identifiers, readBatchArtifactManifest(t, dir).Identifier)
	}
	require.ElementsMatch(t, []string{"first_artifact", "second_artifact"}, identifiers)
}

func TestToolCallGroupRestoredNameOnlyRequestCannotGenerateArguments(t *testing.T) {
	var invoked, requests int
	tool, err := aitool.New("restored_name_only", aitool.WithDangerousNoNeedUserReview(true), aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) { invoked++; return "bad", nil }))
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		requests++
		return nil, fmt.Errorf("unexpected argument generation")
	})
	var request aicommon.ToolCallGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"calls":[{"mode":"require","tool_name":"restored_name_only"},{"mode":"require","tool_name":"restored_name_only"}]}`), &request))
	result, err := react.ExecuteToolCallGroup(context.Background(), react.config.DefaultTask, &request)
	require.NoError(t, err)
	require.Len(t, result.Outcomes, 2)
	for _, outcome := range result.Outcomes {
		require.Equal(t, aicommon.ToolCallStageValidationFailed, outcome.Stage)
		require.ErrorContains(t, outcome.Err, "reason:")
		require.ErrorContains(t, outcome.Err, "retry:")
		require.Nil(t, outcome.Result)
	}
	require.Zero(t, invoked)
	require.Zero(t, requests)
}

func TestToolCallGroupExplicitEmptyParamsSurviveRequestRestore(t *testing.T) {
	var invoked atomic.Int32
	tool, err := aitool.New("restored_empty_params", aitool.WithDangerousNoNeedUserReview(true), aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		invoked.Add(1)
		return "ok", nil
	}))
	require.NoError(t, err)
	react := newBatchTestReAct(t, tool, nil)
	request := aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
		{ToolName: tool.Name, Params: aitool.InvokeParams{}, Reason: "first explicit empty arguments"},
		{ToolName: tool.Name, Params: aitool.InvokeParams{}, Reason: "second explicit empty arguments"},
	}}
	data, err := json.Marshal(request)
	require.NoError(t, err)
	var restored aicommon.ToolCallGroupRequest
	require.NoError(t, json.Unmarshal(data, &restored))
	for _, call := range restored.Calls {
		require.NotNil(t, call.Params)
	}
	result, err := react.ExecuteToolCallGroup(context.Background(), react.config.DefaultTask, &restored)
	require.NoError(t, err)
	require.EqualValues(t, 2, invoked.Load())
	for _, outcome := range result.Outcomes {
		require.Equal(t, aicommon.ToolCallStageDone, outcome.Stage)
		require.True(t, outcome.Result.Success)
	}
}
