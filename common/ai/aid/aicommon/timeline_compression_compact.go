package aicommon

import (
	"context"
	"encoding/json"
)

// foldTimelineWithoutAI retires only mechanically redundant operations,
// then freezes the captured tail. It never summarizes, drops an effective
// value, or rewrites a payload. This transaction is independent of a later AI
// request: a failed AI request cannot undo a successful lossless compaction.
// The caller owns execution; observer delivery belongs to finishCompression.
func (m *Timeline) foldTimelineWithoutAI(ctx context.Context) (*timelineCompressionCommit, error) {
	m.mu.Lock()
	snapshot, err := m.captureCompressionSnapshotLocked()
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	retired := redundantPromotedOperationIDs(snapshot.PromotedItems)
	if len(retired) == 0 && !snapshot.hasNewPromotedEntries() {
		return nil, nil
	}
	snapshot.Context = ctx
	// Archive the live originals before retiring them. Persistence is outside
	// Timeline.mu; source validation below rejects an intervening edit.
	if err := m.sessionMemory.saveOwner(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	version, err := m.validateCompressionSourceLocked(snapshot)
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	result := &TimelineCompressionResult{ThroughID: snapshot.ThroughID,
		RetiredIDs: retired, RetiredPromotedEntryIDs: append([]int64(nil), retired...)}
	ordinaryIDs := make([]int64, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		ordinaryIDs = append(ordinaryIDs, item.ID)
	}
	state := m.compressionFreezeStateLocked(snapshot, ordinaryIDs, retired, version+1)
	notify := m.compressionCallbackLocked(snapshot, result)
	m.freezeState = state
	for _, id := range retired {
		m.retireTimelineItemLocked(id)
	}
	m.rebuildPromotedStateLocked(snapshot.ThroughID)
	if notify != nil {
		for _, pending := range m.freezeBudgetGroupsLocked(false) {
			snapshot.CommittedFreeze.PendingBytes += len(pending.Render())
		}
	}
	m.mu.Unlock()
	return &timelineCompressionCommit{Result: result, Notify: notify, FreezeVersion: version + 1},
		m.sessionMemory.saveOwner()
}

// Keep the latest replacement and the latest subsequent tool REUSE; a final
// DELETE makes the complete key redundant. Skill reads are additive and are
// deliberately not compacted. Malformed operations stay exact.
func redundantPromotedOperationIDs(items []timelineCompressionPromotedItem) []int64 {
	type identity struct{ kind, key string }
	type chain struct {
		upsert int64
		reuse  int64
		valid  bool
	}
	chains := make(map[identity]chain)
	for _, item := range items {
		if item.Kind != TimelinePromotedKindRecentTool && item.Kind != TimelinePromotedKindEvidence {
			continue
		}
		key := identity{item.Kind, item.Key}
		state, found := chains[key]
		if !found {
			state.valid = true
		}
		switch item.Operation {
		case TimelinePromotedOperationUpsert:
			state.upsert, state.reuse = item.ID, 0
		case TimelinePromotedOperationReuse:
			if item.Kind != TimelinePromotedKindRecentTool || state.upsert == 0 {
				state.valid = false
			}
			state.reuse = item.ID
		case TimelinePromotedOperationDelete:
			state.upsert, state.reuse = 0, 0
		default:
			state.valid = false
		}
		chains[key] = state
	}
	var retired []int64
	for _, item := range items {
		state, found := chains[identity{item.Kind, item.Key}]
		if found && state.valid && item.ID != state.upsert && item.ID != state.reuse {
			retired = append(retired, item.ID)
		}
	}
	return retired
}

// Validate evidence upserts before admitting their original items to mechanical
// compaction; the business replay ignores malformed evidence, unlike a normal
// replace operation. The snapshot renderer uses the same validation.
func validCompressionEvidence(op *PromotableTimelineItem) bool {
	var item EvidenceItem
	return json.Unmarshal([]byte(op.Payload), &item) == nil && item.ID == op.Key
}
