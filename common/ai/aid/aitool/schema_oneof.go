package aitool

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils/omap"
)

// NewOneOfObjectSchema creates a JSON schema with oneOf at the root level
// Each item in itemsOpt represents one possible schema variant
func NewOneOfObjectSchema(itemsOpt ...[]ToolOption) string {
	oneOfArray := make([]any, 0, len(itemsOpt))

	for _, itemOpt := range itemsOpt {
		temp := newTool("", itemOpt...)

		// Build the schema object for this variant
		schemaObj := omap.NewGeneralOrderedMap()
		schemaObj.Set("type", "object")

		// Add properties
		paramActually := temp.Params()
		schemaObj.Set("properties", paramActually)

		// Add required fields
		if len(temp.InputSchema.Required) > 0 {
			schemaObj.Set("required", temp.InputSchema.Required)
		}

		oneOfArray = append(oneOfArray, schemaObj)
	}

	// Build the root schema with oneOf
	baseFrame := omap.NewGeneralOrderedMap()
	baseFrame.Set("$schema", "http://json-schema.org/draft-07/schema#")
	baseFrame.Set("type", "object")
	baseFrame.Set("oneOf", oneOfArray)
	baseFrame.Set("additionalProperties", true)

	results, _ := json.MarshalIndent(baseFrame, "", "  ")
	return string(results)
}
