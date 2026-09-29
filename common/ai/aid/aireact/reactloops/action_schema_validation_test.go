package reactloops

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestActionSchemaOverlappingRequiredFields(t *testing.T) {
	tools, err := buildActionTools([]*LoopAction{
		{ActionType: "find_files", Options: []aitool.ToolOption{aitool.WithStringParam("pattern", aitool.WithParam_Required(true))}},
		{ActionType: "grep_text", Options: []aitool.ToolOption{aitool.WithStringParam("pattern", aitool.WithParam_Required(true))}},
	}, aicommon.DefaultToolBatchMaxCalls)
	require.NoError(t, err)
	for _, tool := range tools {
		compiler := jsonschema.NewCompiler()
		require.NoError(t, compiler.AddResource("action.json", tool.Function.Parameters))
		_, err = compiler.Compile("action.json")
		require.NoError(t, err, tool.Function.Name)
	}
}
