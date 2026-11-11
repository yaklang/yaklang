package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const TimelinePromotedKindEvidence = "session-evidence"

// evidenceOpToItem 把证据业务操作转换为独立的 Timeline item。
// item 的数字 ID 保持为空，由各个 Timeline 接收后自行分配。
func evidenceOpToItem(operation EvidenceOperation) (*TimelineItem, error) {
	targetSection := TimelinePromotedTargetSemiDynamic1
	key := strings.TrimSpace(operation.ID)
	switch strings.ToLower(strings.TrimSpace(operation.Op)) {
	case "add", "update", "upsert":
		normalized, err := BuildSessionEvidenceUpsert(key, operation.Content)
		if err != nil {
			return nil, err
		}
		now := time.Now().Unix()
		payload, err := json.Marshal(EvidenceItem{
			ID:          normalized.ID,
			Content:     normalized.Content,
			CreatedUnix: now,
			UpdatedUnix: now,
		})
		if err != nil {
			return nil, fmt.Errorf("encode session evidence %q: %w", normalized.ID, err)
		}
		return NewTimelineItem(&PromotableTimelineItem{
			Kind:          TimelinePromotedKindEvidence,
			TargetSection: targetSection,
			Key:           normalized.ID,
			Operation:     TimelinePromotedOperationUpsert,
			Payload:       string(payload),
			PayloadHash:   promotedPayloadHash(string(payload)),
		}), nil
	case "delete":
		if !sessionEvidenceIDPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid session evidence id %q", operation.ID)
		}
		if strings.TrimSpace(operation.Content) != "" {
			return nil, fmt.Errorf("session evidence delete %q must not carry content", key)
		}
		return NewTimelineItem(&PromotableTimelineItem{
			Kind:          TimelinePromotedKindEvidence,
			TargetSection: targetSection,
			Key:           key,
			Operation:     TimelinePromotedOperationDelete,
			PayloadHash:   promotedPayloadHash(""),
		}), nil
	default:
		return nil, fmt.Errorf("unsupported session evidence operation %q", operation.Op)
	}
}

// itemToEvidenceOp 把证据 Timeline item 还原为 EvidenceStore 可消费的操作。
// EvidenceStore 用 add 表示幂等更新，delete 保持显式删除语义。
func itemToEvidenceOp(item *TimelineItem) (EvidenceOperation, error) {
	if item == nil || item.value == nil {
		return EvidenceOperation{}, fmt.Errorf("session evidence Timeline item is required")
	}
	if item.deleted {
		return EvidenceOperation{}, fmt.Errorf("session evidence Timeline item must be live")
	}
	promoted, ok := item.value.(*PromotableTimelineItem)
	if !ok || promoted == nil || promoted.Kind != TimelinePromotedKindEvidence {
		return EvidenceOperation{}, fmt.Errorf("Timeline item %T is not session evidence", item.value)
	}
	if promoted.TargetSection != TimelinePromotedTargetSemiDynamic1 {
		return EvidenceOperation{}, fmt.Errorf("unsupported evidence target section %q", promoted.TargetSection)
	}
	key := strings.TrimSpace(promoted.Key)
	if key != promoted.Key || !sessionEvidenceIDPattern.MatchString(key) {
		return EvidenceOperation{}, fmt.Errorf("invalid session evidence id %q", promoted.Key)
	}
	if promoted.PayloadHash != "" && promoted.PayloadHash != promotedPayloadHash(promoted.Payload) {
		return EvidenceOperation{}, fmt.Errorf("session evidence %q payload hash does not match", key)
	}

	switch promoted.Operation {
	case TimelinePromotedOperationDelete:
		if promoted.Payload != "" {
			return EvidenceOperation{}, fmt.Errorf("session evidence delete %q must not carry a payload", key)
		}
		return EvidenceOperation{ID: key, Op: "delete"}, nil
	case TimelinePromotedOperationUpsert:
		var evidence EvidenceItem
		if err := json.Unmarshal([]byte(promoted.Payload), &evidence); err != nil {
			return EvidenceOperation{}, fmt.Errorf("decode session evidence %q: %w", key, err)
		}
		if evidence.ID != key {
			return EvidenceOperation{}, fmt.Errorf("session evidence payload id %q does not match key %q", evidence.ID, key)
		}
		if strings.TrimSpace(evidence.Content) == "" {
			return EvidenceOperation{}, fmt.Errorf("session evidence %q content is required", key)
		}
		return EvidenceOperation{ID: key, Op: "add", Content: evidence.Content}, nil
	default:
		return EvidenceOperation{}, fmt.Errorf("unsupported session evidence operation %q", promoted.Operation)
	}
}

// isEvidenceOpNeededLocked 判断操作是否会改变当前 Timeline 的证据状态。
// 调用方必须持有 Timeline 写锁，该方法只做判断，不写入 Timeline。
func (m *Timeline) isEvidenceOpNeededLocked(op EvidenceOperation) bool {
	if m == nil {
		return false
	}
	store, _ := m.evidenceStoreLocked()
	normalized, ok := normalizeEvidenceOp(op)
	return ok && opChangesStore(store, normalized)
}

// filterEvidenceOps 按顺序过滤无效或不会改变证据状态的操作。
// 返回新切片，保留仍有效操作的内容和顺序。
func (m *Timeline) filterEvidenceOps(ops []EvidenceOperation) []EvidenceOperation {
	filtered := make([]EvidenceOperation, 0, len(ops))
	if m == nil || len(ops) == 0 {
		return filtered
	}
	store, _ := m.evidenceStore()
	for _, op := range ops {
		normalized, ok := normalizeEvidenceOp(op)
		if !ok || !opChangesStore(store, normalized) {
			continue
		}
		filtered = append(filtered, op)
		store.ApplyOperations([]EvidenceOperation{normalized})
	}
	return filtered
}

// normalizeEvidenceOp 统一证据操作的 ID 和操作名；无效操作返回 false。
func normalizeEvidenceOp(op EvidenceOperation) (EvidenceOperation, bool) {
	id := strings.TrimSpace(op.ID)
	switch strings.ToLower(strings.TrimSpace(op.Op)) {
	case "add", "update", "upsert":
		normalized, err := BuildSessionEvidenceUpsert(id, op.Content)
		if err != nil {
			return EvidenceOperation{}, false
		}
		return normalized, true
	case "delete":
		if !sessionEvidenceIDPattern.MatchString(id) {
			return EvidenceOperation{}, false
		}
		return EvidenceOperation{ID: id, Op: "delete"}, true
	default:
		return EvidenceOperation{}, false
	}
}

// opChangesStore 判断规范化后的操作是否会改变给定证据集合。
func opChangesStore(store *EvidenceStore, op EvidenceOperation) bool {
	if store == nil {
		return false
	}
	for _, item := range store.Items {
		if item.ID != op.ID {
			continue
		}
		if op.Op == "delete" {
			return true
		}
		return item.Content != op.Content
	}
	return op.Op != "delete"
}

// Task branches project the same session journal, including tombstones, rather
// than rebuilding evidence from an inherited business snapshot. Only exact
// evidence entries are imported; ordinary task history stays in its own branch.
func (m *Timeline) importSessionEvidence(session *Timeline) {
	if m == nil || session == nil || m == session {
		return
	}
	type entry struct {
		item TimelineItem
		op   PromotableTimelineItem
		ts   int64
	}
	var entries []entry
	session.mu.RLock()
	initialized := session.evidenceInitialized
	for _, id := range session.idToTimelineItem.Keys() {
		item, ok := session.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		op, ok := item.value.(*PromotableTimelineItem)
		if !ok || op == nil || op.Kind != TimelinePromotedKindEvidence {
			continue
		}
		ts, _ := session.idToTs.Get(id)
		entries = append(entries, entry{item: *item, op: *op, ts: ts})
	}
	session.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range entries {
		if existing, ok := m.idToTimelineItem.Get(entry.op.ID); ok && existing != nil {
			if existing.deleted != entry.item.deleted {
				m.invalidateFreezeFromLocked(entry.op.ID)
				existing.deleted = entry.item.deleted
			}
			continue
		}
		m.invalidateFreezeFromLocked(entry.op.ID)
		op, item := entry.op, entry.item
		item.value = &op
		ts := entry.ts
		for m.tsToTimelineItem.Have(ts) {
			ts++
		}
		m.idToTs.Set(op.ID, ts)
		m.OrderInsertId(op.ID, &item)
		m.OrderInsertTs(ts, &item)
	}
	m.evidenceInitialized = m.evidenceInitialized || initialized
}

// Evidence mutations use the same journal and freeze transaction as cached
// tools. The runtime view includes pending operations; the semi-dynamic view
// includes only committed operations. Neither view advances a freeze boundary.
func (m *Timeline) evidenceStoreLocked(throughID ...int64) (*EvidenceStore, bool) {
	return m.evidenceStoreMatchingLocked(func(id int64) bool {
		return len(throughID) == 0 || id <= throughID[0]
	})
}

func (m *Timeline) evidenceStoreMatchingLocked(include func(int64) bool) (*EvidenceStore, bool) {
	store := NewEvidenceStore()
	found := m.evidenceInitialized
	for _, id := range m.idToTimelineItem.Keys() {
		if !include(id) {
			continue
		}
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
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)
	return m.replaceEvidenceLocked(store, acquireID, &notifications)
}

// Read/apply/journal under one Timeline lock: a concurrent fork merge or rollback
// must not be overwritten by a business snapshot read before that operation.
func (m *Timeline) applyEvidenceOperations(ops []EvidenceOperation, legacyJSON string, acquireID func() int64) (*EvidenceStore, error) {
	store, _, err := m.applyEvidenceOperationsWithChange(ops, legacyJSON, acquireID)
	return store, err
}

// Report content changes under the same journal lock as the mutation. A parent
// write racing a child retry must not turn that retry into a new discovery.
func (m *Timeline) applyEvidenceOperationsWithChange(ops []EvidenceOperation, legacyJSON string, acquireID func() int64) (*EvidenceStore, bool, error) {
	if m == nil || acquireID == nil {
		return nil, false, fmt.Errorf("evidence timeline and ID allocator are required")
	}
	m.mu.Lock()
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)
	store, found := m.evidenceStoreLocked()
	if !found {
		store = UnmarshalEvidenceStore(legacyJSON)
	}
	before := store.Render()
	store.ApplyOperations(ops)
	store.ShrinkToTokenBudget(sessionEvidenceTokenBudget)
	changed := store.Render() != before
	if err := m.replaceEvidenceLocked(store, acquireID, &notifications); err != nil {
		return nil, false, err
	}
	return store, changed, nil
}

func (m *Timeline) replaceEvidenceLocked(store *EvidenceStore, acquireID func() int64, notifications *[]func()) error {
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
		m.collectItemInputCallbackLocked(notifications, item)
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
