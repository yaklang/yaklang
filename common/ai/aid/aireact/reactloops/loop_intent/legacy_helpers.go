// Package loop_intent legacy_helpers.go retains helper functions that are still
// referenced by tests but were part of the old action-based loop.
// These functions delegate to the shared reactloops implementations where possible.
package loop_intent

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

// capabilityDetail is the local mirror of reactloops.CapabilityDetail, retained
// for test compatibility.
type capabilityDetail struct {
	CapabilityName string `json:"capability_name"`
	CapabilityType string `json:"capability_type"`
	Description    string `json:"description"`
}

var capabilityTypeUsageGuides = map[string]string{
	"tool":       "通过 `require_tool` 调用指定工具执行任务。/ Use `require_tool` to invoke the tool.",
	"mcp-tool":   "通过 `require_tool` 调用指定 MCP 工具执行任务（工具名以 mcp_ 开头）。/ Use `require_tool` with the mcp_ prefixed name to invoke the MCP tool.",
	"forge":      "通过 `require_ai_blueprint` 调用蓝图，由蓝图系统负责自动化执行编排。/ Use `require_ai_blueprint` to execute the blueprint workflow.",
	"skill":      "技能会被自动加载到上下文中，提供特定领域的知识和方法指引。/ Skills are auto-loaded into context.",
	"focus_mode": "通过 `enter_focus_mode` 进入专注模式，在独立的执行环境中完成特定任务。/ Use `enter_focus_mode` to enter focus mode.",
}

var capabilityTypeLabels = map[string]string{
	"tool":       "Tools (工具)",
	"mcp-tool":   "MCP Tools (外部 MCP 工具)",
	"forge":      "Forges / Blueprints (AI 蓝图)",
	"skill":      "Skills (技能)",
	"focus_mode": "Focus Modes (专注模式)",
}

var capabilityTypeOrder = []string{"tool", "mcp-tool", "forge", "skill", "focus_mode"}

func appendCapDetail(details *[]capabilityDetail, name, capType, desc string) {
	*details = append(*details, capabilityDetail{
		CapabilityName: name,
		CapabilityType: capType,
		Description:    desc,
	})
}

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
		md.WriteString("#### " + label + "\n")
		if guide, ok := capabilityTypeUsageGuides[capType]; ok {
			md.WriteString(guide)
			md.WriteString("\n\n")
		}
		for _, cap := range caps {
			md.WriteString("- **" + cap.CapabilityName + "**: " + cap.Description + "\n")
		}
		md.WriteString("\n")
	}
	if !hasContent {
		return ""
	}
	return md.String()
}

func searchLoopMetadata(query string) []*reactloops.LoopMetadata {
	allMeta := reactloops.GetAllLoopMetadata()
	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)
	var matched []*reactloops.LoopMetadata

	for _, meta := range allMeta {
		if meta.IsHidden {
			continue
		}
		searchText := strings.ToLower(meta.Name + " " + meta.Description + " " + meta.UsagePrompt)
		if strings.Contains(searchText, queryLower) {
			matched = append(matched, meta)
			continue
		}
		if len(queryTokens) > 1 {
			meaningfulTokens := 0
			matchCount := 0
			for _, token := range queryTokens {
				if len(token) < 2 {
					continue
				}
				meaningfulTokens++
				if strings.Contains(searchText, token) {
					matchCount++
				}
			}
			if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
				matched = append(matched, meta)
			}
		}
	}
	return matched
}

func intentSummaryStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	content, err := io.ReadAll(fieldReader)
	if err != nil {
		return
	}
	_, _ = emitWriter.Write([]byte(reactloops.CompactIntentSummary(string(content))))
}

// compactIntentSummary is retained for test compatibility.
func compactIntentSummary(summary string) string {
	return reactloops.CompactIntentSummary(summary)
}
