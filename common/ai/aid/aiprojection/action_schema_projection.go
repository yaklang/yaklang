package aiprojection

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
)

const actionSchemaTagName = "FUNCTION_CALL_ACTION_SCHEMA"

// projectActionSchemaTags removes action schema blocks from the trusted schema
// slot and returns them as native tools. Any malformed block leaves the entire
// prompt untouched; a partial tool set must never be sent.
func projectActionSchemaTags(prompt string) (string, []aispec.Tool) {
	if !strings.Contains(prompt, "<|"+actionSchemaTagName+"_") || strings.Contains(prompt, "<|SCHEMA|>") {
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
		inner, err := aitag.SplitViaTAG(section.Raw, actionSchemaTagName)
		if err != nil {
			log.Warnf("action schema projection skipped: invalid schema tag: %v", err)
			return prompt, nil
		}
		for _, block := range inner.GetOrderedBlocks() {
			if !block.IsTagged() {
				cleaned.WriteString(block.Raw)
				continue
			}
			var tool aispec.Tool
			if err := json.Unmarshal([]byte(strings.TrimSpace(block.Content)), &tool); err != nil ||
				tool.Type != "function" || tool.Function.Name != block.Nonce ||
				!validActionSchemaName(block.Nonce) {
				log.Warnf("action schema projection skipped: invalid tool for action %q: %v", block.Nonce, err)
				return prompt, nil
			}
			parameters, ok := tool.Function.Parameters.(map[string]any)
			if !ok || parameters["type"] != "object" {
				log.Warnf("action schema projection skipped: invalid parameters for action %q", block.Nonce)
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
