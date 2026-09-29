package aicommon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	TimelinePromotedTargetSemiDynamic1 = "semi-dynamic-1"
	TimelinePromotedKindRecentTool     = "recent-tool-cache"
	TimelinePromotedOperationUpsert    = "upsert"
	TimelinePromotedOperationDelete    = "delete"
	TimelinePromotedOperationReuse     = "reuse"
)

// PromotableTimelineItem is a control-plane timeline entry. It is persisted and
// follows fork/merge/checkpoint semantics, but is deliberately excluded from the
// ordinary dump buckets, diffs and reducers. Evidence has a separate readable
// UI view. Its payload does
// participate in the prompt freeze budget.
type PromotableTimelineItem struct {
	ID            int64  `json:"id"`
	Kind          string `json:"kind"`
	TargetSection string `json:"target_section"`
	Key           string `json:"key"`
	Operation     string `json:"operation"`
	Payload       string `json:"payload,omitempty"`
	PayloadHash   string `json:"payload_hash,omitempty"`
}

func (p *PromotableTimelineItem) String() string                 { return "" }
func (p *PromotableTimelineItem) GetID() int64                   { return p.ID }
func (p *PromotableTimelineItem) GetShrinkResult() string        { return "" }
func (p *PromotableTimelineItem) GetShrinkSimilarResult() string { return "" }
func (p *PromotableTimelineItem) SetShrinkResult(string)         {}

// OpenPromptText is the exact, non-reducible payload used for open-bucket
// accounting. String remains empty for ordinary history/UI compatibility.
func (p *PromotableTimelineItem) OpenPromptText() string {
	if p != nil && p.Kind == TimelinePromotedKindRecentTool {
		return timelineToolCacheDeltaPrompt(p)
	}
	if p != nil && p.Kind == TimelinePromotedKindEvidence {
		return timelineEvidenceDeltaPrompt(p)
	}
	if p == nil {
		return ""
	}
	if p.Operation == TimelinePromotedOperationDelete {
		return fmt.Sprintf("[state %s/%s deleted]", p.Kind, p.Key)
	}
	if p.Operation == TimelinePromotedOperationReuse {
		return fmt.Sprintf("[state %s/%s reused]", p.Kind, p.Key)
	}
	return fmt.Sprintf("[state %s/%s]\n%s", p.Kind, p.Key, p.Payload)
}

type PromotedTimelineEntry struct {
	Kind          string `json:"kind"`
	TargetSection string `json:"target_section"`
	Key           string `json:"key"`
	Payload       string `json:"payload"`
	PayloadHash   string `json:"payload_hash"`
	SourceItemID  int64  `json:"source_item_id"`
	// LastUsedItemID is independent of the schema source. Older snapshots fall
	// back to SourceItemID until their journal is replayed.
	LastUsedItemID int64 `json:"last_used_item_id,omitempty"`
}

// TimelinePromotedState is the materialized, long-lived projection of sealed
// promotable entries. The journal remains in Timeline for deterministic rollback.
type TimelinePromotedState struct {
	Entries   map[string]map[string]map[string]*PromotedTimelineEntry `json:"entries,omitempty"`
	Watermark int64                                                   `json:"watermark,omitempty"`
}

func newTimelinePromotedState() *TimelinePromotedState {
	return &TimelinePromotedState{Entries: make(map[string]map[string]map[string]*PromotedTimelineEntry)}
}

func cloneTimelinePromotedState(in *TimelinePromotedState) *TimelinePromotedState {
	out := newTimelinePromotedState()
	if in == nil {
		return out
	}
	out.Watermark = in.Watermark
	for target, kinds := range in.Entries {
		out.Entries[target] = make(map[string]map[string]*PromotedTimelineEntry)
		for kind, entries := range kinds {
			out.Entries[target][kind] = make(map[string]*PromotedTimelineEntry)
			for key, entry := range entries {
				if entry == nil {
					continue
				}
				cp := *entry
				out.Entries[target][kind][key] = &cp
			}
		}
	}
	return out
}

func isPromotableTimelineItem(item *TimelineItem) bool {
	if item == nil {
		return false
	}
	_, ok := item.value.(*PromotableTimelineItem)
	return ok
}

func promotedPayloadHash(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// PushPromotable appends a prompt-state mutation to Timeline Open. Only Semi1
// is accepted in the first generation so arbitrary content cannot cross cache
// boundaries.
func (m *Timeline) PushPromotable(id int64, kind, targetSection, key, operation, payload string) bool {
	if m == nil || id <= 0 || targetSection != TimelinePromotedTargetSemiDynamic1 || strings.TrimSpace(kind) == "" || strings.TrimSpace(key) == "" {
		return false
	}
	if operation != TimelinePromotedOperationUpsert && operation != TimelinePromotedOperationDelete && operation != TimelinePromotedOperationReuse {
		return false
	}
	if operation == TimelinePromotedOperationReuse && (kind != TimelinePromotedKindRecentTool || payload != "") {
		return false
	}
	if operation == TimelinePromotedOperationDelete {
		payload = ""
	}
	now := time.Now()
	ts := now.UnixMilli()
	m.mu.Lock()
	defer m.mu.Unlock()
	// Journal entries are immutable, including tombstones retained for rollback.
	// Reusing an ID would also leave a second timestamp index pointing at it.
	if m.idToTimelineItem.Have(id) {
		return false
	}
	// Reuse is a reference, never a way to create or resurrect a cache entry.
	// The caller must submit a full upsert if this returns false.
	if operation == TimelinePromotedOperationReuse && !m.hasToolCacheBeforeLocked(id, key) {
		return false
	}
	for m.tsToTimelineItem.Have(ts) {
		ts++
	}
	m.idToTs.Set(id, ts)
	m.pushTimelineItem(ts, id, &TimelineItem{createdAt: now, value: &PromotableTimelineItem{
		ID: id, Kind: kind, TargetSection: targetSection, Key: key,
		Operation: operation, Payload: payload, PayloadHash: promotedPayloadHash(payload),
	}})
	return true
}

func (m *Timeline) rebuildPromotedStateLocked(throughID int64) {
	state := newTimelinePromotedState()
	if m == nil {
		return
	}
	if m.idToTimelineItem == nil {
		m.promotedState = state
		return
	}
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		control, ok := item.value.(*PromotableTimelineItem)
		if !ok || control == nil {
			continue
		}
		if id > throughID {
			continue
		}
		if control.TargetSection != TimelinePromotedTargetSemiDynamic1 {
			continue
		}
		if id > state.Watermark {
			state.Watermark = id
		}
		kinds := state.Entries[control.TargetSection]
		if kinds == nil {
			kinds = make(map[string]map[string]*PromotedTimelineEntry)
			state.Entries[control.TargetSection] = kinds
		}
		entries := kinds[control.Kind]
		if entries == nil {
			entries = make(map[string]*PromotedTimelineEntry)
			kinds[control.Kind] = entries
		}
		if control.Operation == TimelinePromotedOperationDelete {
			delete(entries, control.Key)
			continue
		}
		if control.Operation == TimelinePromotedOperationReuse {
			if entry := entries[control.Key]; control.Kind == TimelinePromotedKindRecentTool && entry != nil {
				entry.LastUsedItemID = id
			}
			continue
		}
		if control.Operation != TimelinePromotedOperationUpsert {
			continue
		}
		entries[control.Key] = &PromotedTimelineEntry{
			Kind: control.Kind, TargetSection: control.TargetSection, Key: control.Key,
			Payload: control.Payload, PayloadHash: control.PayloadHash, SourceItemID: id,
		}
		if control.Kind == TimelinePromotedKindRecentTool {
			entries[control.Key].LastUsedItemID = id
		}
	}
	m.promotedState = state
}

// effectivePromotedEntries overlays Open deltas on the frozen snapshot without
// freezing or rewriting either view. Returned entries are private copies.
func (m *Timeline) effectivePromotedEntries(targetSection, kind string) []*PromotedTimelineEntry {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	active := make(map[string]*PromotedTimelineEntry)
	watermark := int64(0)
	if m.promotedState != nil {
		watermark = m.promotedState.Watermark
		for key, entry := range m.promotedState.Entries[targetSection][kind] {
			if entry != nil {
				cp := *entry
				active[key] = &cp
			}
		}
	}
	ids := append([]int64(nil), m.idToTimelineItem.Keys()...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if id <= watermark {
			continue
		}
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.TargetSection != targetSection || op.Kind != kind {
			continue
		}
		switch op.Operation {
		case TimelinePromotedOperationDelete:
			delete(active, op.Key)
		case TimelinePromotedOperationReuse:
			if entry := active[op.Key]; entry != nil && kind == TimelinePromotedKindRecentTool {
				entry.LastUsedItemID = id
			}
		case TimelinePromotedOperationUpsert:
			active[op.Key] = &PromotedTimelineEntry{Kind: kind, TargetSection: targetSection, Key: op.Key,
				Payload: op.Payload, PayloadHash: op.PayloadHash, SourceItemID: id, LastUsedItemID: id}
		}
	}
	entries := make([]*PromotedTimelineEntry, 0, len(active))
	for _, entry := range active {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		left, right := promotedToolLastUsedID(entries[i]), promotedToolLastUsedID(entries[j])
		if left == right {
			return entries[i].Key < entries[j].Key
		}
		return left < right
	})
	return entries
}
