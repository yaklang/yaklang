package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Promoted items take part in compression as original journal items. Evidence
// is blacklisted by the item ID the model names. A skill is blacklisted by its
// skill name, which the parser expands to every original item of that skill.
// Tool-cache items stay visible but are not selectable; a later LRU owns them.
// The commit retires exactly the resolved IDs before promoted state is rebuilt.
// Nothing is aggregated or read from promotedState while planning.

func isCompressionPromotedItem(op *PromotableTimelineItem) bool {
	if op == nil || op.TargetSection != TimelinePromotedTargetSemiDynamic1 {
		return false
	}
	switch op.Operation {
	case TimelinePromotedOperationUpsert:
		if op.Kind == TimelinePromotedKindEvidence && !validCompressionEvidence(op) {
			return false
		}
	case TimelinePromotedOperationReuse:
		if op.Kind != TimelinePromotedKindRecentTool {
			return false
		}
	case TimelinePromotedOperationDelete:
	default:
		return false
	}
	switch op.Kind {
	case TimelinePromotedKindRecentTool, TimelinePromotedKindSkillContent, TimelinePromotedKindEvidence:
		return true
	default:
		return false
	}
}

// compressionPromotedItemKey normalizes keys for compaction and display.
// Skill parts use the range identity; other kinds use the written key.
func compressionPromotedItemKey(op *PromotableTimelineItem) string {
	if op == nil {
		return ""
	}
	if op.Kind == TimelinePromotedKindSkillContent && op.Operation == TimelinePromotedOperationUpsert {
		var payload skillContentPayload
		if json.Unmarshal([]byte(op.Payload), &payload) == nil {
			return skillPartIdentity(payload)
		}
	}
	return op.Key
}

// compressionPromotedItemLabel renders a prompt-safe title. Skill identities
// use NUL separators internally and must never reach the request verbatim.
func compressionPromotedItemLabel(op *PromotableTimelineItem) string {
	if op == nil {
		return ""
	}
	if op.Kind == TimelinePromotedKindSkillContent {
		return skillIdentityLabel(compressionPromotedItemKey(op))
	}
	return op.Key
}

// skillIdentityLabel decodes a skill part identity into a prompt-safe label.
func skillIdentityLabel(key string) string {
	if key == skillCatalogKey {
		return "catalog"
	}
	fields := strings.Split(key, "\x00")
	if len(fields) >= 2 && fields[1] == skillContentKindMetadata {
		return fields[0] + " metadata"
	}
	if len(fields) == 5 && fields[1] == skillContentKindContent {
		path := fields[2]
		if path == "" {
			path = skillMDPathPlaceholder
		}
		return fmt.Sprintf("%s:%s:%s-%s", fields[0], path, fields[3], fields[4])
	}
	if len(fields) == 2 {
		return fields[0] + ":" + fields[1]
	}
	return strings.Join(fields, ":")
}

// compressionPromotedItemText renders the complete text of one original item.
// It never returns an aggregated or projected body.
func compressionPromotedItemText(op *PromotableTimelineItem) string {
	if op == nil {
		return ""
	}
	switch op.Kind {
	case TimelinePromotedKindRecentTool:
		switch op.Operation {
		case TimelinePromotedOperationUpsert:
			return fmt.Sprintf("[UPSERT] %s\n%s", op.Key, strings.TrimSpace(op.Payload))
		case TimelinePromotedOperationReuse:
			return fmt.Sprintf("[REUSE] %s", op.Key)
		case TimelinePromotedOperationDelete:
			return fmt.Sprintf("[DELETE] %s", op.Key)
		}
	case TimelinePromotedKindSkillContent:
		if op.Operation == TimelinePromotedOperationUpsert {
			var payload skillContentPayload
			if json.Unmarshal([]byte(op.Payload), &payload) == nil {
				return skillOpenText(payload)
			}
			return strings.TrimSpace(op.Payload)
		}
		if op.Operation == TimelinePromotedOperationDelete {
			return fmt.Sprintf("[TOMBSTONE] skill part %q removed", compressionPromotedItemLabel(op))
		}
	case TimelinePromotedKindEvidence:
		if op.Operation == TimelinePromotedOperationUpsert {
			var item EvidenceItem
			if json.Unmarshal([]byte(op.Payload), &item) == nil && strings.TrimSpace(item.Content) != "" {
				return fmt.Sprintf("[UPSERT] %s\n%s", item.ID, strings.TrimSpace(item.Content))
			}
			return strings.TrimSpace(op.Payload)
		}
		if op.Operation == TimelinePromotedOperationDelete {
			return fmt.Sprintf("[DELETE] %s", op.Key)
		}
	}
	return strings.TrimSpace(op.Payload)
}

// compressionSkillRetirementName is the name the model is allowed to retire.
// It is the skill name only, never a page, line range, or internal NUL identity.
// The catalog is exposed as "catalog".
func compressionSkillRetirementName(item timelineCompressionPromotedItem) string {
	if item.Kind != TimelinePromotedKindSkillContent {
		return ""
	}
	key := item.Key
	if key == skillCatalogKey || strings.HasPrefix(key, skillCatalogKey+"\x00") {
		return "catalog"
	}
	if i := strings.IndexByte(key, 0); i >= 0 {
		return strings.TrimSpace(key[:i])
	}
	if strings.HasSuffix(key, " metadata") {
		return strings.TrimSpace(strings.TrimSuffix(key, " metadata"))
	}
	if i := strings.IndexByte(key, ':'); i > 0 {
		return strings.TrimSpace(key[:i])
	}
	return strings.TrimSpace(key)
}

func compressionPromotedDisplayName(item timelineCompressionPromotedItem) string {
	if item.Kind == TimelinePromotedKindSkillContent {
		return skillIdentityLabel(item.Key)
	}
	if name := strings.TrimSpace(item.Key); name != "" {
		return name
	}
	return item.Kind
}

func renderCompressionPromotedItems(items []timelineCompressionPromotedItem) string {
	var entries []string
	for _, item := range items {
		entries = append(entries, renderCompressionPromotedItem(item))
	}
	return strings.Join(entries, "\n")
}

func renderCompressionPromotedItem(item timelineCompressionPromotedItem) string {
	body := strings.TrimRight(item.PromptText, "\n")
	switch item.Kind {
	case TimelinePromotedKindRecentTool:
		name := compressionPromotedDisplayName(item)
		return fmt.Sprintf("========== TOOL_CACHE %s ==========\nkind=%s name=%s operation=%s\n%s\n========== END TOOL_CACHE %s ==========",
			name, item.Kind, name, item.Operation, body, name)
	case TimelinePromotedKindSkillContent:
		name := compressionSkillRetirementName(item)
		return fmt.Sprintf("========== SKILL_NAME %s ==========\nkind=%s skill_name=%s part=%s operation=%s\n%s\n========== END SKILL_NAME %s ==========",
			name, item.Kind, name, compressionPromotedDisplayName(item), item.Operation, body, name)
	default:
		return fmt.Sprintf("========== RETIRE_ID %d ==========\nkind=%s name=%s operation=%s\n%s\n========== END RETIRE_ID %d ==========",
			item.ID, item.Kind, compressionPromotedDisplayName(item), item.Operation, body, item.ID)
	}
}
