package reactloops

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestActionSchemaOverlappingRequiredFields(t *testing.T) {
	parameters, err := buildActionToolParameters([]*LoopAction{
		{ActionType: "find_files", Options: []aitool.ToolOption{aitool.WithStringParam("pattern", aitool.WithParam_Required(true))}},
		{ActionType: "grep_text", Options: []aitool.ToolOption{aitool.WithStringParam("pattern", aitool.WithParam_Required(true))}},
	})
	require.NoError(t, err)
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("actions.json", parameters))
	_, err = compiler.Compile("actions.json")
	require.NoError(t, err, "loop_plan shares required pattern between multiple actions")
}
