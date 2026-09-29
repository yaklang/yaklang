package aiprojection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestProjectionSchemaConstructorsAndExtractionAgree(t *testing.T) {
	type objectSchema struct {
		Type string `json:"type"`
	}
	for _, tc := range []struct {
		name, functionName, toolType string
		parameters                   any
		valid                        bool
	}{
		{"map", "inspect", "function", map[string]any{"type": "object"}, true},
		{"typed_map", "inspect", "function", map[string]string{"type": "object"}, true},
		{"struct", "inspect", "function", &objectSchema{Type: "object"}, true},
		{"raw_json", "inspect", "function", json.RawMessage(`{"type":"object"}`), true},
		{"name_boundary", strings.Repeat("a", 61) + "_-9", "function", objectSchema{"object"}, true},
		{"long_name", strings.Repeat("a", 65), "function", objectSchema{"object"}, false},
		{"empty_name", "", "function", objectSchema{"object"}, false},
		{"reserved_name", "END_inspect", "function", objectSchema{"object"}, false},
		{"invalid_name", "inspect.space", "function", objectSchema{"object"}, false},
		{"unicode_name", "查询", "function", objectSchema{"object"}, false},
		{"wrong_tool_type", "inspect", "other", objectSchema{"object"}, false},
		{"nil_parameters", "inspect", "function", nil, false},
		{"typed_nil", "inspect", "function", (*objectSchema)(nil), false},
		{"array_parameters", "inspect", "function", []any{}, false},
		{"string_parameters", "inspect", "function", `{"type":"object"}`, false},
		{"missing_type", "inspect", "function", map[string]any{}, false},
		{"array_schema", "inspect", "function", objectSchema{"array"}, false},
		{"wrong_type_value", "inspect", "function", map[string]any{"type": []string{"object"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := aispec.Tool{Type: tc.toolType, Function: aispec.ToolFunction{
				Name: tc.functionName, Parameters: tc.parameters,
			}}
			encoded, err := json.Marshal(tool)
			require.NoError(t, err)
			for _, constructor := range []struct {
				tag   string
				build func(aispec.Tool) (string, error)
			}{{actionSchemaTagName, CreateActionSchema}, {toolParamSchemaTagName, CreateToolParamSchema}} {
				t.Run(constructor.tag, func(t *testing.T) {
					block, err := constructor.build(tool)
					if tc.valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						require.Empty(t, block)
					}
					// Bypass the constructor to exercise validation at the consuming boundary too.
					raw := CreateTag(constructor.tag, tc.functionName, string(encoded))
					prompt := CreateTag(tagPromptSection, "semi-dynamic-2", "before\n"+raw+"\nafter")
					prepared, ok := prepareProjection(prompt, Nonce())
					require.True(t, ok)
					cleaned, tools := projectActionSchemaTags(prepared)
					if !tc.valid {
						require.Empty(t, tools)
						require.Equal(t, prepared, cleaned)
						return
					}
					require.Equal(t, raw, block)
					require.Len(t, tools, 1)
					actual, err := json.Marshal(tools[0])
					require.NoError(t, err)
					require.JSONEq(t, string(encoded), string(actual))
					require.Contains(t, cleaned, "before\n")
					require.Contains(t, cleaned, "\nafter")
					require.NotContains(t, cleaned, constructor.tag)
				})
			}
		})
	}
}

func TestProjectionSchemaPreservesNumbers(t *testing.T) {
	for _, literal := range []string{"9007199254740993", "18446744073709551615", "0.12345678901234567890123456789", "1e400"} {
		t.Run(literal, func(t *testing.T) {
			parameters := json.RawMessage(`{"type":"object","properties":{"value":{"type":"number","const":` + literal + `,"enum":[` + literal + `]}}}`)
			for _, build := range []func(aispec.Tool) (string, error){CreateActionSchema, CreateToolParamSchema} {
				block, err := build(aispec.Tool{Type: "function", Function: aispec.ToolFunction{
					Name: "inspect", Parameters: parameters,
				}})
				require.NoError(t, err)
				result := ProjectAndObserve("schema-numbers", CreateTag(tagPromptSection, "semi-dynamic-2", block))
				require.Len(t, result.Tools, 1)
				encoded, err := json.Marshal(result.Tools[0].Function.Parameters)
				require.NoError(t, err)
				// JSONEq converts to float64 too; compare exact number tokens instead.
				require.Contains(t, string(encoded), `"const":`+literal+`,`)
				require.Contains(t, string(encoded), `"enum":[`+literal+`]`)
			}
		})
	}
}

func TestProjectionSchemaEncodingFailureDoesNotCreateTag(t *testing.T) {
	for _, parameters := range []any{make(chan int), json.RawMessage(`{"type":`)} {
		for _, build := range []func(aispec.Tool) (string, error){CreateActionSchema, CreateToolParamSchema} {
			block, err := build(aispec.Tool{Type: "function", Function: aispec.ToolFunction{
				Name: "inspect", Parameters: parameters,
			}})
			require.ErrorContains(t, err, "encode projection tool")
			require.Empty(t, block)
		}
	}
}

func TestProjectionSchemaInvalidBlockDoesNotProducePartialTools(t *testing.T) {
	const validJSON = `{"type":"function","function":{"name":"inspect","parameters":{"type":"object"}}}`
	valid := CreateTag(actionSchemaTagName, "inspect", validJSON)
	for _, tc := range []struct{ name, block string }{
		{"invalid_json", CreateTag(toolParamSchemaTagName, "broken", `{`)},
		{"trailing_json", CreateTag(toolParamSchemaTagName, "inspect", validJSON+` {}`)},
		{"trailing_garbage", CreateTag(toolParamSchemaTagName, "inspect", validJSON+` !`)},
		{"null", CreateTag(toolParamSchemaTagName, "inspect", `null`)},
		{"empty_tag_name", CreateTag(toolParamSchemaTagName, "", validJSON)},
		{"reserved_tag_name", CreateTag(toolParamSchemaTagName, "END_inspect", validJSON)},
		{"orphan_end_tag", "<|" + toolParamSchemaTagName + "_END_inspect_" + Nonce() + "|>"},
		{"mismatched_name", CreateTag(toolParamSchemaTagName, "other", validJSON)},
		{"duplicate_across_tag_types", CreateTag(toolParamSchemaTagName, "inspect", validJSON)},
		{"invalid_parameters", CreateTag(toolParamSchemaTagName, "inspect", strings.Replace(validJSON, `"object"`, `"array"`, 1))},
		{"missing_end_tag", "<|" + toolParamSchemaTagName + "_broken_" + Nonce() + "|>" + validJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, prompt := range []string{
				CreateTag(tagPromptSection, "semi-dynamic-2", "before\n"+valid+"\n"+tc.block+"\nafter"),
				CreateTag(tagPromptSection, "semi-dynamic-2", "before\n"+tc.block+"\n"+valid+"\nafter"),
				CreateTag(tagPromptSection, "semi-dynamic-2", valid) + "\n" + CreateTag(tagPromptSection, "semi-dynamic-2", tc.block),
			} {
				prepared, ok := prepareProjection(prompt, Nonce())
				require.True(t, ok)
				cleaned, tools := projectActionSchemaTags(prepared)
				require.Empty(t, tools)
				require.Equal(t, prepared, cleaned)
			}
		})
	}
}
