package aicommon

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolCallerExplicitParamsPreserveBusinessMetadataFields(t *testing.T) {
	for _, reserved := range []bool{false, true} {
		t.Run(map[bool]string{false: "business_only", true: "reserved_metadata"}[reserved], func(t *testing.T) {
			params := aitool.InvokeParams{"identifier": "business-id", "call_expectations": "business-expectations"}
			if reserved {
				params[ReservedKeyIdentifier] = "artifact-id"
				params[ReservedKeyCallExpectations] = "invocation-expectations"
			}
			var received aitool.InvokeParams
			tool, err := aitool.New("explicit_metadata_tool",
				aitool.WithStringParam("identifier", aitool.WithParam_Required(true)),
				aitool.WithStringParam("call_expectations", aitool.WithParam_Required(true)),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithSimpleCallback(func(args aitool.InvokeParams, _, _ io.Writer) (any, error) {
					received = args
					return "ok", nil
				}))
			require.NoError(t, err)
			config := NewTestConfig(context.Background(), WithWorkdir(t.TempDir()))
			caller, err := NewToolCaller(context.Background(),
				WithToolCaller_AICallerConfig(config),
				WithToolCaller_Task(config.DefaultTask),
				WithToolCaller_Emitter(config.GetEmitter()),
				WithToolCaller_Reason("explicit arguments"))
			require.NoError(t, err)
			result, directly, err := caller.CallToolWithExistedParams(tool, params)
			require.NoError(t, err)
			require.False(t, directly)
			require.NotNil(t, result)
			require.Equal(t, "business-id", received.GetString("identifier"))
			require.Equal(t, "business-expectations", received.GetString("call_expectations"))
			require.NotContains(t, received, ReservedKeyIdentifier)
			require.NotContains(t, received, ReservedKeyCallExpectations)
			require.Equal(t, "business-id", params.GetString("identifier"))
			if reserved {
				require.Equal(t, "invocation-expectations", caller.callExpectations)
			} else {
				require.Empty(t, caller.callExpectations)
			}
		})
	}
}

func TestToolCallerInvalidExplicitParamsNeverGenerateOrExecute(t *testing.T) {
	aiCalls, toolCalls := 0, 0
	tool, err := aitool.New("explicit_invalid_tool",
		aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			toolCalls++
			return "must not execute", nil
		}))
	require.NoError(t, err)
	config := NewTestConfig(context.Background(), WithWorkdir(t.TempDir()),
		WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
			aiCalls++
			return nil, nil
		}))
	caller, err := NewToolCaller(context.Background(),
		WithToolCaller_AICallerConfig(config),
		WithToolCaller_Task(config.DefaultTask),
		WithToolCaller_Emitter(config.GetEmitter()),
		WithToolCaller_Reason("invalid explicit arguments"))
	require.NoError(t, err)
	result, directly, err := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"id": "invalid"})
	require.NoError(t, err)
	require.False(t, directly)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Error, "参数验证失败")
	require.Zero(t, aiCalls)
	require.Zero(t, toolCalls)
}
