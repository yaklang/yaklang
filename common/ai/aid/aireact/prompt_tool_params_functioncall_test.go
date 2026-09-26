package aireact

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestFunctionCallToolParamsPromptKeepsNativeToolStable(t *testing.T) {
	react, err := NewTestReAct()
	require.NoError(t, err)
	tools := []*aitool.Tool{
		aitool.NewWithoutCallback("read_file", aitool.WithStringParam("path", aitool.WithParam_Required(true))),
		aitool.NewWithoutCallback("search_web", aitool.WithStringParam("query", aitool.WithParam_Required(true))),
	}
	var nativeSchema []byte
	var systemContent any
	for _, tool := range tools {
		prompt, err := react.promptManager.GenerateFunctionCallToolParamsPromptForTask(nil, tool)
		require.NoError(t, err)
		require.Contains(t, prompt, "<|AI_CACHE_SYSTEM_high-static|>")
		require.Contains(t, prompt, "<|FUNCTION_CALL_TOOL_PARAM_SCHEMA_submit_tool_params|>")
		require.Contains(t, prompt, "# 本工具参数 Schema")
		require.Contains(t, prompt, tool.Name)
		require.NotContains(t, prompt, "<|TOOL_PARAM_")
		require.NotContains(t, prompt, `"@action":"call-tool"`)
		projected := aiprojection.ProjectAndObserve("r2-test-model", prompt)
		require.NotNil(t, projected)
		require.True(t, projected.IsHijacked)
		require.Len(t, projected.Tools, 1)
		require.Equal(t, "submit_tool_params", projected.Tools[0].Function.Name)
		require.NotEmpty(t, projected.Messages)
		currentSchema, err := json.Marshal(projected.Tools[0])
		require.NoError(t, err)
		var parameters any
		parameterJSON, err := json.Marshal(projected.Tools[0].Function.Parameters)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(parameterJSON, &parameters))
		compiler := jsonschema.NewCompiler()
		require.NoError(t, compiler.AddResource("submit.json", parameters))
		compiled, err := compiler.Compile("submit.json")
		require.NoError(t, err)
		require.NoError(t, compiled.Validate(map[string]any{"params": map[string]any{}}))
		require.Error(t, compiled.Validate(map[string]any{"identifier": "missing_params"}))
		if nativeSchema == nil {
			nativeSchema = currentSchema
			systemContent = projected.Messages[0].Content
		} else {
			require.Equal(t, nativeSchema, currentSchema)
			require.Equal(t, systemContent, projected.Messages[0].Content)
		}
	}
}
