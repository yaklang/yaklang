package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const TimelinePromotedKindEvidence = "session-evidence"

// Evidence mutations use the same journal and freeze transaction as cached
// tools. The runtime view includes pending operations; the semi-dynamic view
// includes only committed operations. Neither view advances a freeze boundary.
func (m *Timeline) evidenceStoreLocked() (*EvidenceStore, bool) {
	store := NewEvidenceStore()
	found := m.evidenceInitialized
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindEvidence {
			continue
		}
		found = true
		if item.deleted {
			continue
		}
		if op.Operation == TimelinePromotedOperationDelete {
			store.ApplyOperations([]EvidenceOperation{{Op: "delete", ID: op.Key}})
			continue
		}
		var evidence EvidenceItem
		if json.Unmarshal([]byte(op.Payload), &evidence) != nil || evidence.ID != op.Key {
			continue
		}
		replaced := false
		for i := range store.Items {
			if store.Items[i].ID == evidence.ID {
				store.Items[i] = evidence
				replaced = true
				break
			}
		}
		if !replaced {
			store.Items = append(store.Items, evidence)
		}
	}
	return store, found
}

func (m *Timeline) evidenceStore() (*EvidenceStore, bool) {
	if m == nil {
		return NewEvidenceStore(), false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.evidenceStoreLocked()
}

// replaceEvidence journals a whole business mutation, including budget evictions,
// before checking the common freeze boundary. No partially applied batch is read.
func (m *Timeline) replaceEvidence(store *EvidenceStore, acquireID func() int64) error {
	if m == nil || store == nil || acquireID == nil {
		return fmt.Errorf("evidence timeline and ID allocator are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.replaceEvidenceLocked(store, acquireID)
}

// Read/apply/journal under one Timeline lock: a concurrent fork merge or rollback
// must not be overwritten by a business snapshot read before that operation.
func (m *Timeline) applyEvidenceOperations(ops []EvidenceOperation, legacyJSON string, acquireID func() int64) (*EvidenceStore, error) {
	if m == nil || acquireID == nil {
		return nil, fmt.Errorf("evidence timeline and ID allocator are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	store, found := m.evidenceStoreLocked()
	if !found {
		store = UnmarshalEvidenceStore(legacyJSON)
	}
	store.ApplyOperations(ops)
	store.ShrinkToTokenBudget(sessionEvidenceTokenBudget)
	if err := m.replaceEvidenceLocked(store, acquireID); err != nil {
		return nil, err
	}
	return store, nil
}

func (m *Timeline) replaceEvidenceLocked(store *EvidenceStore, acquireID func() int64) error {
	previous, _ := m.evidenceStoreLocked()
	next := make(map[string]EvidenceItem, len(store.Items))
	old := make(map[string]EvidenceItem, len(previous.Items))
	for _, item := range store.Items {
		next[item.ID] = item
	}
	for _, item := range previous.Items {
		old[item.ID] = item
	}
	var mutations []*PromotableTimelineItem
	appendMutation := func(key, operation, payload string) {
		mutations = append(mutations, &PromotableTimelineItem{Kind: TimelinePromotedKindEvidence, TargetSection: TimelinePromotedTargetSemiDynamic1, Key: key, Operation: operation, Payload: payload, PayloadHash: promotedPayloadHash(payload)})
	}
	changed := false
	for _, item := range previous.Items {
		if _, ok := next[item.ID]; !ok {
			appendMutation(item.ID, TimelinePromotedOperationDelete, "")
			changed = true
		}
	}
	for _, item := range store.Items {
		if prior, ok := old[item.ID]; ok && prior.Content == item.Content {
			continue
		}
		payload, _ := json.Marshal(item) // EvidenceItem contains only strings and integers.
		appendMutation(item.ID, TimelinePromotedOperationUpsert, string(payload))
		changed = true
	}
	if !changed {
		return nil
	}
	ids := m.idToTimelineItem.Keys()
	var lastID int64
	if len(ids) > 0 {
		lastID = ids[len(ids)-1]
	}
	for _, op := range mutations {
		op.ID = acquireID()
		if op.ID <= lastID {
			return fmt.Errorf("evidence ID %d must follow timeline ID %d", op.ID, lastID)
		}
		lastID = op.ID
	}
	for _, op := range mutations {
		now := time.Now()
		ts := now.UnixMilli()
		for m.tsToTimelineItem.Have(ts) {
			ts++
		}
		item := &TimelineItem{createdAt: now, value: op}
		m.idToTs.Set(op.ID, ts)
		m.OrderInsertId(op.ID, item)
		m.OrderInsertTs(ts, item)
	}
	m.evidenceInitialized = true
	for _, op := range mutations {
		if item, ok := m.idToTimelineItem.Get(op.ID); ok {
			m.emitTimelineItemAsync(item)
		}
	}
	return nil
}

func (m *Timeline) projectEvidenceLocked() string {
	var frozen []EvidenceItem
	if m.promotedState != nil {
		entries := m.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindEvidence]
		keys := make([]string, 0, len(entries))
		for key := range entries {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entry := entries[key]
			if entry == nil {
				continue
			}
			var item EvidenceItem
			if json.Unmarshal([]byte(entry.Payload), &item) == nil && item.ID == key {
				frozen = append(frozen, item)
			}
		}
	}
	return RenderSessionEvidencePromptBlock(StablePromptNonce("session-evidence-promoted"), renderEvidenceItems(frozen))
}

// timelineEvidenceDeltaPrompt renders one immutable mutation at its journal position.
// Later edits append another delta; only freeze folds operations into the semi snapshot.
func timelineEvidenceDeltaPrompt(op *PromotableTimelineItem) string {
	if op == nil || op.Kind != TimelinePromotedKindEvidence {
		return ""
	}
	if op.Operation == TimelinePromotedOperationDelete {
		return fmt.Sprintf("[evidence_delta]:\n[id: %s]\n[TOMBSTONE] 此证据已删除，忽略此前同 id 内容。", op.Key)
	}
	var item EvidenceItem
	if op.Operation != TimelinePromotedOperationUpsert || json.Unmarshal([]byte(op.Payload), &item) != nil || item.ID != op.Key {
		return fmt.Sprintf("[evidence_delta]:\n[id: %s]\n[INVALID] 无效证据变更，不改变已有证据。", op.Key)
	}
	return fmt.Sprintf("[evidence_delta]:\n[id: %s]\n[UPSERT] 以本次记录为准，覆盖此前同 id 内容。\n%s", item.ID, item.Content)
}

// Evidence has a user-visible audit view, while String/GetShrinkResult remain
// empty so promotion payloads never leak into the ordinary reducer input.
func timelineEvidenceDisplay(item *TimelineItem) (string, bool) {
	if item == nil {
		return "", false
	}
	op, ok := item.value.(*PromotableTimelineItem)
	if !ok || op == nil || op.Kind != TimelinePromotedKindEvidence {
		return "", false
	}
	if op.Operation == TimelinePromotedOperationDelete {
		return fmt.Sprintf("[evidence delete: %s]", op.Key), true
	}
	var evidence EvidenceItem
	if json.Unmarshal([]byte(op.Payload), &evidence) != nil || evidence.ID != op.Key {
		return fmt.Sprintf("[evidence invalid: %s]", op.Key), true
	}
	return fmt.Sprintf("[evidence upsert: %s]\n%s", op.Key, evidence.Content), true
}
