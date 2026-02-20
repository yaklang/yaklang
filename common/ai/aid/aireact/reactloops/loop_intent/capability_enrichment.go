package loop_intent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

type capabilityDetail struct {
	CapabilityName string `json:"capability_name"`
	CapabilityType string `json:"capability_type"`
	Description    string `json:"description"`
}

var capabilityTypeUsageGuides = map[string]string{
	"tool":       "通过 `require_tool` 调用指定工具执行任务。/ Use `require_tool` to invoke the tool.",
	"forge":      "通过 `tool_compose` 调用蓝图编排执行多步骤自动化流程。/ Use `tool_compose` to execute the blueprint.",
	"skill":      "技能会被自动加载到上下文中，提供特定领域的知识和方法指引。/ Skills are auto-loaded into context.",
	"focus_mode": "通过 `enter_focus_mode` 进入专注模式，在独立的执行环境中完成特定任务。/ Use `enter_focus_mode` to enter focus mode.",
}

var capabilityTypeLabels = map[string]string{
	"tool":       "Tools (工具)",
	"forge":      "Forges / Blueprints (AI 蓝图)",
	"skill":      "Skills (技能)",
	"focus_mode": "Focus Modes (专注模式)",
}

var capabilityTypeOrder = []string{"tool", "forge", "skill", "focus_mode"}

func parseCapabilityDetails(jsonStr string) []capabilityDetail {
	if jsonStr == "" {
		return nil
	}
	var details []capabilityDetail
	if err := json.Unmarshal([]byte(jsonStr), &details); err != nil {
		log.Warnf("intent loop: failed to parse matched_capabilities_details: %v", err)
		return nil
	}
	return details
}

func marshalCapabilityDetails(details []capabilityDetail) string {
	if len(details) == 0 {
		return ""
	}
	data, err := json.Marshal(details)
	if err != nil {
		log.Warnf("intent loop: failed to marshal capability details: %v", err)
		return ""
	}
	return string(data)
}

// buildCapabilityEnrichmentMarkdown constructs structured Markdown from capability details,
// grouped by type with usage guidance for each type.
// When recommendedNames is non-empty, only matching capabilities are included.
func buildCapabilityEnrichmentMarkdown(details []capabilityDetail, recommendedNames map[string]bool) string {
	if len(details) == 0 {
		return ""
	}

	grouped := make(map[string][]capabilityDetail)
	for _, d := range details {
		if len(recommendedNames) > 0 && !recommendedNames[d.CapabilityName] {
			continue
		}
		grouped[d.CapabilityType] = append(grouped[d.CapabilityType], d)
	}

	var md strings.Builder
	md.WriteString("### Recommended Capabilities / 推荐能力\n\n")

	hasContent := false
	for _, capType := range capabilityTypeOrder {
		caps, ok := grouped[capType]
		if !ok || len(caps) == 0 {
			continue
		}
		hasContent = true

		label := capabilityTypeLabels[capType]
		if label == "" {
			label = capType
		}
		md.WriteString(fmt.Sprintf("#### %s\n", label))

		if guide, ok := capabilityTypeUsageGuides[capType]; ok {
			md.WriteString(guide)
			md.WriteString("\n\n")
		}

		for _, cap := range caps {
			md.WriteString(fmt.Sprintf("- **%s**: %s\n", cap.CapabilityName, cap.Description))
		}
		md.WriteString("\n")
	}

	if !hasContent {
		return ""
	}
	return md.String()
}
