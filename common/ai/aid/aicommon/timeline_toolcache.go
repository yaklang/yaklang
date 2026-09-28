package aicommon

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
)

// This ephemeral type labels cache entries in the prompt without changing the
// persisted journal. Its informational tags have no projection authority.
type timelineToolCachePromptItem struct{ TextTimelineItem }

// timelineToolCacheDeltaPrompt renders one immutable event at its journal
// position. The stable tag suffix is informational, not a projection nonce:
// these schemas describe business-tool params and never declare native tools.
func timelineToolCacheDeltaPrompt(item *PromotableTimelineItem) string {
	if item == nil || item.Kind != TimelinePromotedKindRecentTool {
		return ""
	}
	var body string
	switch item.Operation {
	case TimelinePromotedOperationUpsert:
		body = fmt.Sprintf("[UPSERT] %s\n%s", item.Key, strings.TrimSpace(item.Payload))
	case TimelinePromotedOperationReuse:
		body = fmt.Sprintf("[REUSE] %s", item.Key)
	case TimelinePromotedOperationDelete:
		body = fmt.Sprintf("[DELETE] %s", item.Key)
	default:
		return ""
	}
	return "<|CACHE_TOOL_CALL_" + buildinaitools.RecentToolCacheStableNonce + "|>\n" + body + "\n<|CACHE_TOOL_CALL_END_" + buildinaitools.RecentToolCacheStableNonce + "|>"
}

// promotedToolLastUsedID accepts snapshots written before reuse was recorded.
func promotedToolLastUsedID(entry *PromotedTimelineEntry) int64 {
	if entry.LastUsedItemID > 0 {
		return entry.LastUsedItemID
	}
	return entry.SourceItemID
}

// hasToolCacheBeforeLocked resolves a reuse reference against the journal at
// that item's position. A later upsert must not authorize an earlier reuse.
// Caller holds Timeline.mu. Frozen entries remain in the journal for rollback.
func (m *Timeline) hasToolCacheBeforeLocked(id int64, key string) bool {
	var latestID int64
	var exists bool
	for _, candidateID := range m.idToTimelineItem.Keys() {
		if candidateID >= id || candidateID <= latestID {
			continue
		}
		item, ok := m.idToTimelineItem.Get(candidateID)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindRecentTool ||
			op.TargetSection != TimelinePromotedTargetSemiDynamic1 || op.Key != key {
			continue
		}
		switch op.Operation {
		case TimelinePromotedOperationUpsert:
			latestID, exists = candidateID, true
		case TimelinePromotedOperationDelete:
			latestID, exists = candidateID, false
		}
	}
	return exists
}

func renderPromotedRecentTools(state *TimelinePromotedState) string {
	if state == nil {
		return ""
	}
	entries := state.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindRecentTool]
	if len(entries) == 0 {
		return ""
	}
	keys := make([]string, 0, len(entries))
	for key, entry := range entries {
		if entry != nil {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := promotedToolLastUsedID(entries[keys[i]]), promotedToolLastUsedID(entries[keys[j]])
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right // Most recently used tools appear last, after freeze.
	})
	var out strings.Builder
	out.WriteString("<|CACHE_TOOL_CALL_" + buildinaitools.RecentToolCacheStableNonce + "|>\n")
	out.WriteString("# Recently Used Tools (available for directly_call_tool)\n\n")
	for _, key := range keys {
		if entry := entries[key]; entry != nil {
			out.WriteString(strings.TrimSpace(entry.Payload))
			out.WriteString("\n\n")
		}
	}
	out.WriteString("\n<|CACHE_TOOL_CALL_END_" + buildinaitools.RecentToolCacheStableNonce + "|>")
	return strings.TrimSpace(out.String())
}
