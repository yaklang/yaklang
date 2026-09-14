package aicommon

import (
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestNativeParamSchemaOptionalFields(t *testing.T) {
	for _, opts := range [][]aitool.ToolOption{nil, {aitool.WithStringParam("url")}} {
		tool := aitool.NewWithoutCallback("do_http_request", opts...)
		raw, err := json.Marshal(buildNativeToolForParamGen(tool).Function.Parameters)
		require.NoError(t, err)
		var document any
		require.NoError(t, json.Unmarshal(raw, &document))
		compiler := jsonschema.NewCompiler()
		require.NoError(t, compiler.AddResource("native.json", document))
		schema, err := compiler.Compile("native.json")
		require.NoError(t, err)
		require.NoError(t, schema.Validate(map[string]any{"@action": "call-tool", "tool": tool.Name, "params": map[string]any{}}))
	}
}
