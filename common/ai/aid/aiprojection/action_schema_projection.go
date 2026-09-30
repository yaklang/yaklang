package aiprojection

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

const actionSchemaTagName = "FUNCTION_CALL_ACTION_SCHEMA"
const toolParamSchemaTagName = "FUNCTION_CALL_TOOL_PARAM_SCHEMA"

// projectActionSchemaTags removes action schema blocks from the trusted schema
// slot and returns them as native tools. Any malformed block leaves the entire
// prompt untouched; a partial tool set must never be sent.
func projectActionSchemaTags(prompt string) (string, []aispec.Tool) {
	if !containsActionSchemaTag(prompt) || strings.Contains(prompt, "<|SCHEMA|>") {
		return prompt, nil
	}
	outer, err := aitag.SplitViaTAG(prompt, acceptedTagNames...)
	if err != nil {
		log.Warnf("action schema projection skipped: invalid outer prompt: %v", err)
		return prompt, nil
	}

	var cleaned strings.Builder
	var tools []aispec.Tool
	seen := make(map[string]struct{})
	for _, section := range outer.GetOrderedBlocks() {
		if !section.IsTagged() || section.TagName != tagPromptSection || section.Nonce != "semi-dynamic-2" {
			cleaned.WriteString(section.Raw)
			continue
		}
		inner, err := aitag.SplitViaTAG(section.Raw, actionSchemaTagName, toolParamSchemaTagName)
		if err != nil {
			log.Warnf("action schema projection skipped: invalid schema tag: %v", err)
			return prompt, nil
		}
		for _, block := range inner.GetOrderedBlocks() {
			if !block.IsTagged() {
				// The generic splitter treats reserved names and stray closing tags
				// as text. They still invalidate this entire trusted schema slot.
				// Unsigned tag-looking data was masked by prepareProjection already.
				if containsActionSchemaTag(block.Raw) {
					log.Warnf("action schema projection skipped: malformed schema tag")
					return prompt, nil
				}
				cleaned.WriteString(block.Raw)
				continue
			}
			tool, err := decodeProjectionTool(block.Content)
			if err != nil {
				log.Warnf("action schema projection skipped: invalid tool for action %q: %v", block.Nonce, err)
				return prompt, nil
			}
			if tool.Function.Name != block.Nonce {
				log.Warnf("action schema projection skipped: mismatched action name %q", block.Nonce)
				return prompt, nil
			}
			if _, duplicate := seen[block.Nonce]; duplicate {
				log.Warnf("action schema projection skipped: duplicate action %q", block.Nonce)
				return prompt, nil
			}
			seen[block.Nonce] = struct{}{}
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return prompt, nil
	}
	return cleaned.String(), tools
}

func containsActionSchemaTag(text string) bool {
	return strings.Contains(text, "<|"+actionSchemaTagName) || strings.Contains(text, "<|"+toolParamSchemaTagName)
}

// Decode and validate the wire representation at both boundaries. UseNumber
// preserves exact schema bounds, defaults and enum values through projection.
func decodeProjectionTool(encoded string) (aispec.Tool, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(encoded)))
	decoder.UseNumber()
	var tool aispec.Tool
	if err := decoder.Decode(&tool); err != nil {
		return aispec.Tool{}, fmt.Errorf("decode projection tool: %w", err)
	}
	// Unlike Unmarshal, Decode alone would accept a second JSON value or junk.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return aispec.Tool{}, fmt.Errorf("projection tool must contain exactly one JSON value")
	}
	if err := validateProjectionTool(tool); err != nil {
		return aispec.Tool{}, err
	}
	return tool, nil
}

func validateProjectionTool(tool aispec.Tool) error {
	if tool.Type != "function" {
		return fmt.Errorf("projection tool type must be function")
	}
	if !validActionSchemaName(tool.Function.Name) {
		return fmt.Errorf("invalid projection tool function name")
	}
	parameters, ok := tool.Function.Parameters.(map[string]any)
	if !ok || parameters["type"] != "object" {
		return fmt.Errorf("projection tool parameters must be an object schema")
	}
	return nil
}

func validActionSchemaName(name string) bool {
	if len(name) == 0 || len(name) > 64 || strings.HasPrefix(name, "END_") {
		return false
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}
