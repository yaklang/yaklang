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
