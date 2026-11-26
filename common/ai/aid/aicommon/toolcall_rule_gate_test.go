package aicommon

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestToolCallRuleGateApply(t *testing.T) {
	tool, err := aitool.New("bash", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
		return nil, nil
	}))
	require.NoError(t, err)
	params := aitool.InvokeParams{"command": "rm -rf /"}

	t.Run("returns-decision", func(t *testing.T) {
		caller := &ToolCaller{ctx: context.Background()}
		caller.ruleGate = func(_ context.Context, toolName string, got aitool.InvokeParams) *ToolCallRuleDecision {
			require.Equal(t, "bash", toolName)
			require.Equal(t, "rm -rf /", got["command"])
			return &ToolCallRuleDecision{
				Action: schema.SFR_ACTION_DENY,
			}
		}
		decision := caller.applyToolCallRuleGate(tool, params)
		require.NotNil(t, decision)
		require.Equal(t, schema.SFR_ACTION_DENY, decision.Action)
	})

	t.Run("panic-fails-open", func(t *testing.T) {
		caller := &ToolCaller{ctx: context.Background()}
		caller.ruleGate = func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
			panic("broken analyzer")
		}
		require.Nil(t, caller.applyToolCallRuleGate(tool, params))
	})

	t.Run("no-gate", func(t *testing.T) {
		caller := &ToolCaller{ctx: context.Background()}
		require.Nil(t, caller.applyToolCallRuleGate(tool, params))
	})

	t.Run("nil-tool", func(t *testing.T) {
		caller := &ToolCaller{ctx: context.Background()}
		caller.ruleGate = func(context.Context, string, aitool.InvokeParams) *ToolCallRuleDecision {
			t.Fatal("gate must not run without a tool")
			return nil
		}
		require.Nil(t, caller.applyToolCallRuleGate(nil, params))
	})
}
