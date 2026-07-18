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
)

// PromotableTimelineItem is a control-plane timeline entry. It is persisted and
// follows fork/merge/checkpoint semantics, but is deliberately excluded from the
// user timeline, ordinary buckets, diffs and reducers.
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

type PromotedTimelineEntry struct {
	Kind          string `json:"kind"`
	TargetSection string `json:"target_section"`
	Key           string `json:"key"`
	Payload       string `json:"payload"`
	PayloadHash   string `json:"payload_hash"`
	SourceItemID  int64  `json:"source_item_id"`
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
	if operation != TimelinePromotedOperationUpsert && operation != TimelinePromotedOperationDelete {
		return false
	}
	if operation == TimelinePromotedOperationDelete {
		payload = ""
	}
	now := time.Now()
	ts := now.UnixMilli()
	m.mu.Lock()
	defer m.mu.Unlock()
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

func (m *Timeline) rebuildPromotedStateLocked(sealedBeforeID int64, forceAll bool) {
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
		if !forceAll && (sealedBeforeID <= 0 || id >= sealedBeforeID) {
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
		entries[control.Key] = &PromotedTimelineEntry{
			Kind: control.Kind, TargetSection: control.TargetSection, Key: control.Key,
			Payload: control.Payload, PayloadHash: control.PayloadHash, SourceItemID: id,
		}
	}
	m.promotedState = state
}

func (m *Timeline) forcePromoteAllLocked() {
	m.rebuildPromotedStateLocked(0, true)
}

func (m *Timeline) ForcePromoteAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.forcePromoteAllLocked()
}

func (m *Timeline) HasPromotableKind(kind string) bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		if control, ok := item.value.(*PromotableTimelineItem); ok && control != nil && control.Kind == kind {
			return true
		}
	}
	return false
}

// effectivePromotedKeys returns the current materialized membership for one
// promotion namespace, including mutations that are still in Timeline Open.
// It is deliberately read-only: session restore must not seal buckets or move
// the promotion watermark merely to rebuild execution-side authorization.
//
// Ordering follows the latest mutation source ID. Reuse of an unchanged tool
// does not create a prompt mutation, so exact execution-side LRU touches are
// intentionally not persisted across process restarts.
func (m *Timeline) effectivePromotedKeys(targetSection, kind string) []string {
	if m == nil || strings.TrimSpace(targetSection) == "" || strings.TrimSpace(kind) == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	type activePromotion struct {
		key      string
		sourceID int64
	}
	active := make(map[string]activePromotion)
	watermark := int64(0)
	if m.promotedState != nil {
		watermark = m.promotedState.Watermark
		if kinds := m.promotedState.Entries[targetSection]; kinds != nil {
			for key, entry := range kinds[kind] {
				if entry == nil {
					continue
				}
				active[key] = activePromotion{key: key, sourceID: entry.SourceItemID}
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
		control, ok := item.value.(*PromotableTimelineItem)
		if !ok || control == nil || control.TargetSection != targetSection || control.Kind != kind {
			continue
		}
		if control.Operation == TimelinePromotedOperationDelete {
			delete(active, control.Key)
			continue
		}
		active[control.Key] = activePromotion{key: control.Key, sourceID: id}
	}

	ordered := make([]activePromotion, 0, len(active))
	for _, entry := range active {
		ordered = append(ordered, entry)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].sourceID == ordered[j].sourceID {
			return ordered[i].key < ordered[j].key
		}
		return ordered[i].sourceID < ordered[j].sourceID
	})
	keys := make([]string, 0, len(ordered))
	for _, entry := range ordered {
		keys = append(keys, entry.key)
	}
	return keys
}

func (m *Timeline) projectPromoted(sealedBeforeID int64) (string, string) {
	if m == nil {
		return "", ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// A previously materialized watermark never moves backwards during ordinary
	// rendering. This also preserves force-promotion performed before compression
	// and one-time legacy bootstrap.
	effectiveLimit := sealedBeforeID
	if m.promotedState != nil && m.promotedState.Watermark > 0 && m.promotedState.Watermark+1 > effectiveLimit {
		effectiveLimit = m.promotedState.Watermark + 1
	}
	m.rebuildPromotedStateLocked(effectiveLimit, false)
	semi := renderPromotedRecentTools(m.promotedState)
	var pending []*PromotableTimelineItem
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		control, ok := item.value.(*PromotableTimelineItem)
		if !ok || control == nil || id <= m.promotedState.Watermark {
			continue
		}
		pending = append(pending, control)
	}
	return semi, renderPromotableOpenDeltas(pending, semi == "")
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
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	out.WriteString("<|CACHE_TOOL_CALL_[current-nonce]|>\n")
	out.WriteString("# Recently Used Tools (available for directly_call_tool)\n\n")
	for _, key := range keys {
		if entry := entries[key]; entry != nil {
			out.WriteString(strings.TrimSpace(entry.Payload))
			out.WriteString("\n\n")
		}
	}
	out.WriteString(recentToolRoutingInstructions)
	out.WriteString("\n<|CACHE_TOOL_CALL_END_[current-nonce]|>")
	return strings.TrimSpace(out.String())
}

func renderPromotableOpenDeltas(items []*PromotableTimelineItem, includeInstructions bool) string {
	if len(items) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("<|CACHE_TOOL_CALL_[current-nonce]|>\n")
	out.WriteString("# Prompt State Updates (pending Timeline seal)\n\n")
	for _, item := range items {
		if item == nil || item.Kind != TimelinePromotedKindRecentTool {
			continue
		}
		if item.Operation == TimelinePromotedOperationDelete {
			fmt.Fprintf(&out, "- invalidated recent tool: %s\n", item.Key)
			continue
		}
		out.WriteString(strings.TrimSpace(item.Payload))
		out.WriteString("\n\n")
	}
	if includeInstructions {
		out.WriteString(recentToolRoutingInstructions)
	}
	out.WriteString("\n<|CACHE_TOOL_CALL_END_[current-nonce]|>")
	return strings.TrimSpace(out.String())
}

const recentToolRoutingInstructions = `## How to use directly_call_tool

If the exact tool you need is already listed above, prefer directly_call_tool for faster execution.
The schemas above are params-only shapes. Pass one directly as directly_call_tool_params; do not wrap it with @action, tool, or params.
For multiline values, TOOL_PARAM_{param_name}_[current-nonce] AITAG blocks may be used. AITAG values override same-named JSON params.`
