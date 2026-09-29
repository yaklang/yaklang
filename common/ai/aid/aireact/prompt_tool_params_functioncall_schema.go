package aireact

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

const functionCallToolParamSchemaTag = "FUNCTION_CALL_TOOL_PARAM_SCHEMA"

// buildFunctionCallParamTools is the fixed R2 tool registry. Keep its order and
// schemas independent of the selected business tool so repeated calls have a
// stable native tool prefix. A new entry also needs a response handler in R2.
func buildFunctionCallParamTools() ([]aispec.Tool, error) {
	parameters, err := buildSubmitToolParamsSchema()
	if err != nil {
		return nil, err
	}
	return []aispec.Tool{{
		Type: "function",
		Function: aispec.ToolFunction{
			Name:        aicommon.SubmitToolParamsFunctionName,
			Description: "Submit parameters for the single business tool selected by the runtime.",
			Parameters:  parameters,
		},
	}}, nil
}

func buildSubmitToolParamsSchema() (map[string]any, error) {
	// The inner params object stays open: its concrete schema is shown in the
	// selected-tool section and checked by tool.ValidateParams after the call.
	schemaText := aitool.NewObjectSchema(
		aitool.WithRawParam("params", map[string]any{
			"type": "object", "additionalProperties": true,
		}, aitool.WithParam_Required(true),
			aitool.WithParam_Description("Complete parameters for the selected business tool")),
		aitool.WithStringParam("identifier",
			aitool.WithParam_Description("Optional short identifier for this invocation")),
		aitool.WithStringParam("call_expectations",
			aitool.WithParam_Description("Optional success criteria for this invocation")),
	)
	var parameters map[string]any
	if err := json.Unmarshal([]byte(schemaText), &parameters); err != nil {
		return nil, fmt.Errorf("decode submit_tool_params schema: %w", err)
	}
	delete(parameters, "$schema")
	parameters["additionalProperties"] = false
	return parameters, nil
}

func renderFunctionCallParamSchemaTags(tools []aispec.Tool) (string, error) {
	if len(tools) == 0 {
		return "", fmt.Errorf("R2 native tool registry is empty")
	}
	var out strings.Builder
	seen := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		name := tool.Function.Name
		if name == "" || strings.HasPrefix(name, "END_") || len(name) > 64 {
			return "", fmt.Errorf("invalid R2 native tool name %q", name)
		}
		for _, r := range name {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
				continue
			}
			return "", fmt.Errorf("invalid R2 native tool name %q", name)
		}
		if _, exists := seen[name]; exists {
			return "", fmt.Errorf("duplicate R2 native tool name %q", name)
		}
		seen[name] = struct{}{}
		if tool.Type != "function" {
			return "", fmt.Errorf("R2 native tool %q must be a function", name)
		}
		block, err := aiprojection.CreateToolParamSchema(tool)
		if err != nil {
			return "", err
		}
		out.WriteString(block)
		out.WriteByte('\n')
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}
