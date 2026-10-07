package aicommon

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestToolCaller_ExplicitParamsValidatedBeforeInvocation(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native_%v", native), func(t *testing.T) {
			var requests, invoked atomic.Int32
			cfg := NewTestConfig(context.Background(), WithEnableFunctionCallMode(native), WithAgreePolicy(AgreePolicyYOLO), WithWorkdir(t.TempDir()), WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
				requests.Add(1)
				return nil, fmt.Errorf("explicit tool calls must not request argument generation")
			}))
			tool, err := aitool.New("explicit_write", aitool.WithStringParam("file", aitool.WithParam_Required(true)), aitool.WithStringParam("content", aitool.WithParam_Required(true)), aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
				invoked.Add(1)
				require.Equal(t, "safe text", params["content"])
				return "ok", nil
			}))
			require.NoError(t, err)
			call := func(params aitool.InvokeParams) (*aitool.ToolResult, error) {
				caller, err := NewToolCaller(context.Background(), WithToolCaller_AICallerConfig(cfg), WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Task(cfg.DefaultTask), WithToolCaller_Reason("explicit write"))
				require.NoError(t, err)
				result, _, err := caller.CallToolWithExistedParams(tool, params)
				return result, err
			}
			result, err := call(nil)
			var retry *ToolCallRetryError
			require.ErrorAs(t, err, &retry)
			require.Nil(t, result)
			result, err = call(aitool.InvokeParams{"file": "/tmp/example.txt"})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.Success)
			require.Contains(t, result.Error, "content")
			require.Zero(t, invoked.Load())
			result, err = call(aitool.InvokeParams{"file": "/tmp/example.txt", "content": "safe text"})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.Success)
			require.EqualValues(t, 1, invoked.Load())
			require.Zero(t, requests.Load())
		})
	}
}

func TestToolCaller_ExplicitBusinessMetadataFieldsRemainIsolated(t *testing.T) {
	for _, reserved := range []bool{false, true} {
		t.Run(fmt.Sprintf("reserved_%v", reserved), func(t *testing.T) {
			params := aitool.InvokeParams{
				"identifier":        "business-id",
				"call_expectations": "business-expectations",
				"query":             map[string]any{"value": "original"},
			}
			if reserved {
				params[ReservedKeyIdentifier] = "artifact-id"
				params[ReservedKeyCallExpectations] = "invocation-expectations"
			}
			original := cloneEndpointParams(params)
			var received aitool.InvokeParams
			var invoked int
			tool, err := aitool.New("explicit_metadata_tool",
				aitool.WithStringParam("identifier", aitool.WithParam_Required(true)),
				aitool.WithStringParam("call_expectations", aitool.WithParam_Required(true)),
				aitool.WithStructParam("query", nil, aitool.WithStringParam("value")),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithSimpleCallback(func(args aitool.InvokeParams, _, _ io.Writer) (any, error) {
					invoked++
					received = cloneEndpointParams(args)
					args["identifier"] = "plugin-change"
					args.GetObject("query").Set("value", "plugin-change")
					return "ok", nil
				}))
			require.NoError(t, err)
			cfg := NewTestConfig(context.Background(), WithWorkdir(t.TempDir()), WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
				t.Error("explicit parameters must not request argument generation")
				return nil, fmt.Errorf("unexpected AI request")
			}))
			caller, err := NewToolCaller(context.Background(),
				WithToolCaller_AICallerConfig(cfg), WithToolCaller_Task(cfg.DefaultTask),
				WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Reason("explicit parameters"))
			require.NoError(t, err)
			result, directly, err := caller.CallToolWithExistedParams(tool, params)
			require.NoError(t, err)
			require.False(t, directly)
			require.NotNil(t, result)
			require.True(t, result.Success, "%s", result.Error)
			require.Equal(t, 1, invoked)
			require.Equal(t, "business-id", received["identifier"])
			require.Equal(t, "business-expectations", received["call_expectations"])
			require.NotContains(t, received, ReservedKeyIdentifier)
			require.NotContains(t, received, ReservedKeyCallExpectations)
			require.Equal(t, original, params, "framework and plugin changes must not mutate the source payload")
			require.Equal(t, "original", params.GetObject("query").GetString("value"))
			if reserved {
				require.Equal(t, "invocation-expectations", caller.callExpectations)
			} else {
				require.Empty(t, caller.callExpectations, "a business field is not execution metadata")
			}
		})
	}
}
