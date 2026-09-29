package reactloops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestFunctionCallActionVariantSelection(t *testing.T) {
	native := &LoopAction{ActionType: "protocol_probe", Description: "load only", Options: []aitool.ToolOption{aitool.WithStringParam("load_name")}}
	legacy := &LoopAction{ActionType: native.ActionType, Description: "generate and execute", FunctionCallAction: native}
	loop := makeSchemaStabilityTestLoop(aicommon.NewConfig(context.Background()))
	loop.actions.Set(legacy.ActionType, legacy)
	for _, enabled := range []bool{false, true} {
		loop.useFunctionCallActionVariants = enabled
		for _, mode := range []bool{false, true} {
			loop.functionCallMode = mode
			expected := legacy
			if enabled && mode {
				expected = native
			}
			handler, err := loop.GetActionHandler(legacy.ActionType)
			require.NoError(t, err)
			require.Same(t, expected, handler)
			for _, action := range loop.GetAllActions() {
				if action.ActionType == legacy.ActionType {
					require.Same(t, expected, action)
				}
			}
		}
	}
	require.Equal(t, "generate and execute", legacy.Description)
	tools, err := buildActionTools([]*LoopAction{native}, aicommon.DefaultToolBatchMaxCalls)
	require.NoError(t, err)
	params := tools[0].Function.Parameters.(map[string]any)
	require.NotContains(t, params["properties"], "human_readable_thought")
	require.Contains(t, params["properties"], "identifier")
	var plain map[string]any
	require.NoError(t, json.Unmarshal([]byte(buildSchema(legacy)), &plain))
	require.Contains(t, plain["properties"], "human_readable_thought")
}
