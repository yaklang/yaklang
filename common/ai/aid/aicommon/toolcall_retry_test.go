package aicommon

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"io"
	"sync/atomic"
	"testing"
)

func TestToolCallerRejectedDirectPrepareNeverGeneratesOrExecutes(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native_%v", native), func(t *testing.T) {
			var requests, executions, ends int32
			cfg := NewConfig(context.Background(), WithEnableFunctionCallMode(native), WithAICallback(func(AICallerConfigIf, *AIRequest) (*AIResponse, error) {
				atomic.AddInt32(&requests, 1)
				return nil, fmt.Errorf("unexpected AI call")
			}))
			tool, err := aitool.New("retry_boundary", aitool.WithStringParam("value", aitool.WithParam_Required(true)), aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
				atomic.AddInt32(&executions, 1)
				return "bad", nil
			}))
			require.NoError(t, err)
			caller, err := NewToolCaller(context.Background(), WithToolCaller_AICallerConfig(cfg), WithToolCaller_Task(cfg.DefaultTask), WithToolCaller_Emitter(cfg.GetEmitter()), WithToolCaller_Reason("test explicit proposal"), WithToolCaller_OnEnd(func(string) { atomic.AddInt32(&ends, 1) }))
			require.NoError(t, err)
			result, direct, err := caller.DirectlyCallTool(tool, nil, func(*Action, string) (aitool.InvokeParams, bool, *aitool.Tool, error) { return nil, true, tool, nil })
			require.Nil(t, result)
			require.False(t, direct)
			var retry *ToolCallRetryError
			require.ErrorAs(t, err, &retry)
			require.Contains(t, err.Error(), "reason:")
			require.Contains(t, err.Error(), "retry:")
			require.Equal(t, tool.Name, retry.ToolName)
			require.Zero(t, atomic.LoadInt32(&requests))
			require.Zero(t, atomic.LoadInt32(&executions))
			require.EqualValues(t, 1, atomic.LoadInt32(&ends))
		})
	}
}
