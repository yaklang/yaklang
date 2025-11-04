package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// ConvertAIToolToLoopAction converts an AI Tool to a LoopAction.
// It extracts the tool's parameters and converts them to ToolOptions for the LoopAction.
func ConvertAIToolToLoopAction(tool *aitool.Tool) *LoopAction {
	var options []aitool.ToolOption

	// Extract properties from the tool's InputSchema
	if tool.Tool.InputSchema.Properties != nil {
		// Build a set of required field names for quick lookup
		requiredSet := make(map[string]bool)
		if tool.Tool.InputSchema.Required != nil {
			for _, req := range tool.Tool.InputSchema.Required {
				requiredSet[req] = true
			}
		}

		// Convert each property to a ToolOption
		tool.Tool.InputSchema.Properties.ForEach(func(paramName string, paramSchema any) bool {
			// Skip special fields that are handled by the loop action system
			if paramName == "@action" || paramName == "human_readable_thought" {
				return true
			}

			// Convert paramSchema to map[string]interface{}
			schemaMap, ok := paramSchema.(map[string]interface{})
			if !ok {
				// Try to convert using utils if it's a different map type
				if generalMap := utils.InterfaceToGeneralMap(paramSchema); generalMap != nil {
					schemaMap = generalMap
				} else {
					// Skip if we can't convert to map
					return true
				}
			}

			// Create a copy of the schema map to avoid modifying the original
			schemaCopy := make(map[string]any)
			for k, v := range schemaMap {
				schemaCopy[k] = v
			}

			// Build PropertyOptions based on the schema
			var propertyOpts []aitool.PropertyOption

			// Check if this parameter is required
			if requiredSet[paramName] {
				propertyOpts = append(propertyOpts, aitool.WithParam_Required(true))
			}

			// Create the ToolOption using WithRawParam
			option := aitool.WithRawParam(paramName, schemaCopy, propertyOpts...)
			options = append(options, option)

			return true
		})
	}

	return &LoopAction{
		AsyncMode:      false,
		ActionType:     tool.GetName(),
		Description:    tool.GetDescription(),
		Options:        options,
		ActionVerifier: nil,
		ActionHandler:  nil,
	}
}
