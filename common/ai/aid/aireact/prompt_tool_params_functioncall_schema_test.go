package aireact

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestFunctionCallParamToolsSchemaAndTagsAreStable(t *testing.T) {
	first, err := buildFunctionCallParamTools()
	require.NoError(t, err)
	second, err := buildFunctionCallParamTools()
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, first, second)
	require.Equal(t, aicommon.SubmitToolParamsFunctionName, first[0].Function.Name)

	parameters, ok := first[0].Function.Parameters.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", parameters["type"])
	require.Equal(t, false, parameters["additionalProperties"])
	require.NotContains(t, parameters, "$schema")
	require.Equal(t, []any{"params"}, parameters["required"])
	properties, ok := parameters["properties"].(map[string]any)
	require.True(t, ok)
	params, ok := properties["params"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", params["type"])
	require.Equal(t, true, params["additionalProperties"])
	require.Equal(t, "string", properties["identifier"].(map[string]any)["type"])
	require.Equal(t, "string", properties["call_expectations"].(map[string]any)["type"])

	tags, err := renderFunctionCallParamSchemaTags(first)
	require.NoError(t, err)
	again, err := renderFunctionCallParamSchemaTags(second)
	require.NoError(t, err)
	require.Equal(t, tags, again)
	blocks, err := aitag.SplitViaTAG(tags, functionCallToolParamSchemaTag)
	require.NoError(t, err)
	require.Len(t, blocks.GetTaggedBlocks(), 1)
	var decoded aispec.Tool
	require.NoError(t, json.Unmarshal([]byte(blocks.GetTaggedBlocks()[0].Content), &decoded))
	require.Equal(t, first[0], decoded)

	// The renderer already supports a future second fixed R2 function. Its
	// response handler must be added before it enters the real registry.
	extra := aispec.Tool{Type: "function", Function: aispec.ToolFunction{
		Name: "request_more_context", Parameters: map[string]any{"type": "object"},
	}}
	twoTags, err := renderFunctionCallParamSchemaTags(append(first, extra))
	require.NoError(t, err)
	twoBlocks, err := aitag.SplitViaTAG(twoTags, functionCallToolParamSchemaTag)
	require.NoError(t, err)
	require.Len(t, twoBlocks.GetTaggedBlocks(), 2)
	_, err = renderFunctionCallParamSchemaTags(append(first, first[0]))
	require.ErrorContains(t, err, "duplicate")
}
