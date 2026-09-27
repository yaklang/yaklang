package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/log"
)

// RecordRecentlyUsedTool is the session write path for successful calls and
// explicit preloads. Runtime LRU changes immediately; prompt state changes only
// through immutable Open events, then joins the next Timeline freeze.
func (c *Config) RecordRecentlyUsedTool(tool *aitool.Tool) buildinaitools.RecentToolCacheMutation {
	if c == nil || tool == nil || c.GetTimeline() == nil || c.GetAiToolManager() == nil {
		return buildinaitools.RecentToolCacheMutation{}
	}
	tl := c.GetTimeline()
	tl.toolCacheMu.Lock()
	defer tl.toolCacheMu.Unlock()
	mutation := c.GetAiToolManager().AddRecentlyUsedTool(tool)
	entry := mutation.Upsert
	if entry == nil {
		entry = mutation.Reuse
	}
	changed := false
	if entry != nil {
		payload := buildinaitools.RenderRecentToolEntryForPromotion(entry)
		fullPayload := payload
		operation := TimelinePromotedOperationUpsert
		// Compare with this Timeline, not just the runtime LRU. A fork, restored
		// session or pre-seeded manager may have no corresponding journal entry.
		for _, prior := range tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool) {
			if prior.Key == entry.Name && prior.Payload == payload {
				operation, payload = TimelinePromotedOperationReuse, ""
				break
			}
		}
		changed = c.appendToolCacheDelta(entry.Name, operation, payload)
		if !changed && operation == TimelinePromotedOperationReuse {
			// A concurrent explicit rollback can remove the referenced schema.
			// Recover with a complete delta instead of silently dropping the use.
			changed = c.appendToolCacheDelta(entry.Name, TimelinePromotedOperationUpsert, fullPayload)
		}
	}
	for _, deleted := range mutation.Deleted {
		if deleted != nil {
			changed = c.appendToolCacheDelta(deleted.Name, TimelinePromotedOperationDelete, "") || changed
		}
	}
	c.saveToolCacheDeltas(changed)
	return mutation
}

func (c *Config) appendToolCacheDelta(name, operation, payload string) bool {
	return c.GetTimeline().PushPromotable(c.AcquireId(), TimelinePromotedKindRecentTool,
		TimelinePromotedTargetSemiDynamic1, name, operation, payload)
}

func (c *Config) saveToolCacheDeltas(changed bool) {
	if changed && c.PersistentSessionId != "" && c.GetDB() != nil {
		c.GetTimeline().Save(c.GetDB(), c.PersistentSessionId)
	}
}

// restoreRecentToolsFromTimeline restores both frozen and pending membership,
// including reuse order and deletes. It never infers cache entries from tool
// result text. Missing tools, schema changes and budget evictions become new
// Open deltas; existing frozen schemas remain byte-stable until the next freeze.
func (c *Config) restoreRecentToolsFromTimeline() {
	if c == nil || c.GetTimeline() == nil || c.GetAiToolManager() == nil {
		return
	}
	tl, manager := c.GetTimeline(), c.GetAiToolManager()
	tl.toolCacheMu.Lock()
	defer tl.toolCacheMu.Unlock()
	previous := tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool)
	var tools []*aitool.Tool
	byName := make(map[string]*aitool.Tool, len(previous))
	for _, entry := range previous {
		tool, err := manager.GetToolByName(entry.Key)
		if err != nil || tool == nil {
			log.Warnf("cannot restore cached tool [%s] for session [%s]: %v", entry.Key, c.PersistentSessionId, err)
			continue
		}
		tools = append(tools, tool)
		byName[entry.Key] = tool
	}
	current := manager.RestoreRecentlyUsedTools(tools)
	payloads := make(map[string]string, len(current))
	for _, entry := range current {
		payloads[entry.Name] = buildinaitools.RenderRecentToolEntryForPromotion(entry)
	}
	changed := false
	for _, prior := range previous {
		payload, exists := payloads[prior.Key]
		if !exists {
			changed = c.appendToolCacheDelta(prior.Key, TimelinePromotedOperationDelete, "") || changed
		} else if payload != prior.Payload {
			changed = c.appendToolCacheDelta(prior.Key, TimelinePromotedOperationUpsert, payload) || changed
		}
	}
	if changed {
		// Schema corrections are new journal entries, so their recency must
		// agree with the runtime order too. Membership already fits the budget.
		tools = nil
		for _, entry := range tl.effectivePromotedEntries(TimelinePromotedTargetSemiDynamic1, TimelinePromotedKindRecentTool) {
			tools = append(tools, byName[entry.Key])
		}
		manager.RestoreRecentlyUsedTools(tools)
	}
	c.saveToolCacheDeltas(changed)
}
