package reactloops

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

// buildActionTools compiles each action into a native function. The function
// name selects the action, so @action is intentionally absent from parameters.
func buildActionTools(actions []*LoopAction, maxBatchCalls int) ([]aispec.Tool, error) {
	tools := make([]aispec.Tool, 0, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if action == nil {
			continue
		}
		if !validActionToolName(action.ActionType) {
			return nil, utils.Errorf("invalid native action tool name %q", action.ActionType)
		}
		if _, exists := seen[action.ActionType]; exists {
			return nil, utils.Errorf("duplicate native action tool name %q", action.ActionType)
		}
		seen[action.ActionType] = struct{}{}
		opts := make([]any, 0, len(action.Options)+3)
		for _, opt := range commonActionSchemaOptions(true) {
			opts = append(opts, opt)
		}
		actionOptions := action.Options
		if action.NativeOptions != nil {
			actionOptions = action.NativeOptions
		}
		for _, opt := range actionOptions {
			opts = append(opts, opt)
		}
		schemaText, err := applyToolBatchSchemaMaxItems(aitool.NewObjectSchema(opts...), maxBatchCalls)
		if err != nil {
			return nil, utils.Wrapf(err, "build schema for action %q", action.ActionType)
		}
		var parameters map[string]any
		if err := json.Unmarshal([]byte(schemaText), &parameters); err != nil {
			return nil, utils.Wrapf(err, "decode schema for action %q", action.ActionType)
		}
		// Tool APIs expect the schema object itself, not a draft-07 document.
		delete(parameters, "$schema")
		if properties, ok := parameters["properties"].(map[string]any); ok {
			delete(properties, "@action")
		}
		if required, ok := parameters["required"].([]any); ok {
			filtered := required[:0]
			for _, field := range required {
				if field != "@action" {
					filtered = append(filtered, field)
				}
			}
			parameters["required"] = filtered
		}
		description := nativeActionDescription(action)
		tools = append(tools, aispec.Tool{
			Type: "function",
			Function: aispec.ToolFunction{
				Name:        action.ActionType,
				Description: description,
				Parameters:  parameters,
			},
		})
	}
	return tools, nil
}

func validActionToolName(name string) bool {
	if len(name) == 0 || len(name) > 64 || strings.HasPrefix(name, "END_") {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// renderFunctionCallSchemaTags places one AITAG per action in place of the text
// SCHEMA slot. Projection into provider tools is a separate, later step.
func renderFunctionCallSchemaTags(tools []aispec.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	var out strings.Builder
	for _, tool := range tools {
		if !validActionToolName(tool.Function.Name) {
			return "", utils.Errorf("invalid action tool name %q", tool.Function.Name)
		}
		encoded, err := json.Marshal(tool)
		if err != nil {
			return "", utils.Wrapf(err, "encode action tool %q", tool.Function.Name)
		}
		out.WriteString("<|FUNCTION_CALL_ACTION_SCHEMA_")
		out.WriteString(tool.Function.Name)
		out.WriteString("|>\n")
		out.Write(encoded)
		out.WriteString("\n<|FUNCTION_CALL_ACTION_SCHEMA_END_")
		out.WriteString(tool.Function.Name)
		out.WriteString("|>\n")
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}
