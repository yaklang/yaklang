package aicommon

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils/omap"
)

// Freeze lifecycle:
//
//	append ordinary/control items -> measure the shared open bucket
//	-> commit batch membership and exact promotions together -> read-only prompt.
//
// AI compression is a separate consumer of ordinary facts; it cannot summarize
// structured promotion payloads. Evidence and tool schemas share this journal.
//
// TimelineFreezeResult is a detached receipt for one freeze transaction. An
// unchanged Version means nothing was committed. Promotions contains exact
// journal operations, never AI summaries. No callback runs under Timeline.mu.
type TimelineFreezeResult struct {
	Version        int64
	ThroughID      int64
	NewlyFrozenIDs []int64
	Promotions     []PromotableTimelineItem
	PendingBytes   int
}

// TimelineFreezeState persists boundaries, not rendered text. Keeping the
// original membership prevents promotion from shrinking and reopening a bucket.
type TimelineFreezeState struct {
	Version int64                  `json:"version"`
	Batches []*TimelineFreezeBatch `json:"batches,omitempty"`
}

type TimelineFreezeBatch struct {
	IDs           []int64   `json:"ids"`
	BucketStart   time.Time `json:"bucket_start"`
	BucketEnd     time.Time `json:"bucket_end"`
	Nonce         string    `json:"nonce"`
	InitialTaskID string    `json:"initial_task_id,omitempty"`
}

func cloneTimelineFreezeState(state *TimelineFreezeState) *TimelineFreezeState {
	if state == nil {
		return nil
	}
	out := &TimelineFreezeState{Version: state.Version}
	for _, batch := range state.Batches {
		if batch == nil {
			continue
		}
		cp := *batch
		cp.IDs = append([]int64(nil), batch.IDs...)
		out.Batches = append(out.Batches, &cp)
	}
	return out
}

func (m *Timeline) frozenThroughLocked() int64 {
	var through int64
	if m.freezeState != nil {
		for _, batch := range m.freezeState.Batches {
			for _, id := range batch.IDs {
				if id > through {
					through = id
				}
			}
		}
	}
	return through
}

// Freeze seals complete time/byte buckets. Appending an item calls this same
// operation; callers importing a batch can call it explicitly after insertion.
// The last bucket stays open unless its own rendered size reaches the budget.
func (m *Timeline) Freeze() TimelineFreezeResult {
	if m == nil {
		return TimelineFreezeResult{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.freezeLocked(false)
}

// FreezeAll explicitly closes the current tail too, e.g. before emergency
// compression. It is the compatibility path for the former ForcePromoteAll.
func (m *Timeline) FreezeAll() TimelineFreezeResult {
	if m == nil {
		return TimelineFreezeResult{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.freezeLocked(true)
}

// freezeBudgetGroupsLocked uses the existing fixed/adaptive bucket algorithm
// on a private view. Promotion payloads count as non-reducible entries;
// the raw journal, ordinary dump, UI and reducer views stay unchanged.
func (m *Timeline) freezeBudgetGroupsLocked(useSizer bool, throughLimit ...int64) TimelineIntervalBlocks {
	view := &Timeline{idToTimelineItem: omap.NewOrderedMap(map[int64]*TimelineItem{}), idToTs: m.idToTs}
	through := m.frozenThroughLocked()
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted || id <= through || (len(throughLimit) > 0 && id > throughLimit[0]) {
			continue
		}
		if control, ok := item.value.(*PromotableTimelineItem); ok {
			item = &TimelineItem{createdAt: item.createdAt, value: &TextTimelineItem{
				ID: id, Text: control.OpenPromptText(),
			}}
			if control.Kind == TimelinePromotedKindRecentTool {
				item.value = &timelineToolCachePromptItem{TextTimelineItem{ID: id, Text: control.OpenPromptText()}}
			}
		}
		view.idToTimelineItem.OrderInsert(id, item, lessInt64)
	}
	seed := m.openTaskSeedLocked(view)
	if useSizer && m.bucketSizer != nil {
		return view.groupByMinutesWithSizer(TimelineDumpDefaultIntervalMinutes, m.bucketSizer, seed).blocks
	}
	budget := int64(-1)
	if useSizer {
		budget = m.getEffectiveBucketByteSize()
	}
	return view.groupByMinutesAndBytesLocked(TimelineDumpDefaultIntervalMinutes, budget, seed).blocks
}

func (m *Timeline) freezeLocked(all bool, throughLimit ...int64) TimelineFreezeResult {
	// A one-shot compression publishes its captured freeze and summary together.
	// Writes remain append-only during generation, including exact-state deltas.
	if m.compressionSnapshot != nil {
		version := int64(0)
		if m.freezeState != nil {
			version = m.freezeState.Version
		}
		return TimelineFreezeResult{Version: version, ThroughID: m.frozenThroughLocked()}
	}
	if m.freezeState == nil {
		m.freezeState = &TimelineFreezeState{}
	}
	result := TimelineFreezeResult{Version: m.freezeState.Version, ThroughID: m.frozenThroughLocked()}
	groups := m.freezeBudgetGroupsLocked(true, throughLimit...)
	for i, block := range groups {
		if len(block.Items) == 0 {
			continue
		}
		seal := all || i < len(groups)-1
		if !seal {
			budget := block.freezeBudget
			seal = budget > 0 && int64(len(block.Render())) >= budget
		}
		if seal && !all {
			lastID := block.Items[len(block.Items)-1].GetID()
			for _, later := range groups[i+1:] {
				if len(later.Items) > 0 && later.Items[0].GetID() < lastID {
					seal = false
					break
				}
			}
		}
		if !seal {
			// Keep the complete remaining ID range open; a watermark must not
			// skip earlier entries when imported timestamps are out of order.
			break
		}
		batch := &TimelineFreezeBatch{BucketStart: block.BucketStart, BucketEnd: block.BucketEnd,
			InitialTaskID: block.initialTaskID}
		for _, item := range block.Items {
			id := item.GetID()
			batch.IDs = append(batch.IDs, id)
			result.NewlyFrozenIDs = append(result.NewlyFrozenIDs, id)
			if id > result.ThroughID {
				result.ThroughID = id
			}
			if original, ok := m.idToTimelineItem.Get(id); ok {
				if control, ok := original.value.(*PromotableTimelineItem); ok {
					result.Promotions = append(result.Promotions, *control)
				}
			}
		}
		batch.Nonce = fmt.Sprintf("f%dt%d", batch.IDs[0], batch.BucketStart.Unix())
		m.freezeState.Batches = append(m.freezeState.Batches, batch)
	}
	sort.Slice(result.NewlyFrozenIDs, func(i, j int) bool { return result.NewlyFrozenIDs[i] < result.NewlyFrozenIDs[j] })
	sort.Slice(result.Promotions, func(i, j int) bool { return result.Promotions[i].ID < result.Promotions[j].ID })
	if len(result.NewlyFrozenIDs) > 0 {
		m.freezeState.Version++
		result.Version = m.freezeState.Version
		// Exact state and boundary are published together under the same lock.
		m.rebuildPromotedStateLocked(result.ThroughID)
	}
	for _, pending := range m.freezeBudgetGroupsLocked(false) {
		result.PendingBytes += len(pending.Render())
	}
	return result
}

// frozenPromptBlocksLocked is read-only. Pending evidence/tool-cache events are
// rendered in place; frozen controls appear only in semi-dynamic snapshots.
// Filtering occurs after bucket selection, so helper prompts share the same
// freeze boundaries without receiving unrelated tool-cache schemas.
func (m *Timeline) frozenPromptBlocksLocked(excludeToolCache bool) TimelineRenderableBlocks {
	var blocks TimelineRenderableBlocks
	if m.compressedHead != nil && strings.TrimSpace(m.compressedHead.Text) != "" {
		blocks = append(blocks, &TimelineCompressedHeadBlock{
			CoveredEndItemID: m.compressedHead.CoveredEndItemID, CoveredEndAtMs: m.compressedHead.CoveredEndAtMs,
			Version: m.compressedHead.Version, Text: m.compressedHead.Text,
		})
	}
	appendBlock := func(ids []int64, start, end time.Time, nonce, initialTaskID string, open bool) {
		block := &TimelineIntervalBlock{BucketStart: start, BucketEnd: end,
			IntervalMinutes: TimelineDumpDefaultIntervalMinutes, Open: open,
			frozenNonce: nonce, initialTaskID: initialTaskID}
		filteredToolCache := false
		for _, id := range ids {
			item, ok := m.idToTimelineItem.Get(id)
			if !ok || item == nil || item.deleted {
				continue
			}
			if isPromotableTimelineItem(item) {
				op := item.value.(*PromotableTimelineItem)
				if !open || op == nil {
					continue
				}
				if op.Kind == TimelinePromotedKindRecentTool && excludeToolCache {
					filteredToolCache = true
					continue
				}
				if op.Kind != TimelinePromotedKindEvidence && op.Kind != TimelinePromotedKindRecentTool {
					continue
				}
				// A detached prompt view preserves the journal, reducer exclusion and UI audit.
				item = &TimelineItem{createdAt: item.createdAt, value: &TextTimelineItem{ID: id, Text: op.OpenPromptText()}}
				if op.Kind == TimelinePromotedKindRecentTool {
					item.value = &timelineToolCachePromptItem{TextTimelineItem{ID: id, Text: op.OpenPromptText()}}
				}
			}
			block.Items = append(block.Items, item)
		}
		if len(block.Items) > 0 || filteredToolCache {
			blocks = append(blocks, block)
		}
	}
	if m.freezeState != nil {
		for _, batch := range m.freezeState.Batches {
			appendBlock(batch.IDs, batch.BucketStart, batch.BucketEnd, batch.Nonce, batch.InitialTaskID, false)
		}
	}
	for _, block := range m.freezeBudgetGroupsLocked(false) {
		var ids []int64
		for _, item := range block.Items {
			ids = append(ids, item.GetID())
		}
		appendBlock(ids, block.BucketStart, block.BucketEnd,
			fmt.Sprintf("f%dt%d", ids[0], block.BucketStart.Unix()), block.initialTaskID, true)
	}
	return blocks
}

// Reopening is reserved for explicit history edits (rollback/late insertion),
// never caused by rendering, promotion or a change of bucket budget.
func (m *Timeline) invalidateFreezeFromLocked(firstID int64) {
	if m.freezeState == nil || firstID > m.frozenThroughLocked() {
		return
	}
	var kept []*TimelineFreezeBatch
	for _, batch := range m.freezeState.Batches {
		if len(batch.IDs) == 0 {
			continue
		}
		if batch.IDs[len(batch.IDs)-1] >= firstID {
			break
		}
		kept = append(kept, batch)
	}
	m.freezeState.Batches = kept
	m.freezeState.Version++
	m.rebuildPromotedStateLocked(m.frozenThroughLocked())
}

// FreezeSnapshot returns detached boundary metadata for inspection/consumers.
func (m *Timeline) FreezeSnapshot() *TimelineFreezeState {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneTimelineFreezeState(m.freezeState)
}

func (m *Timeline) pruneEmptyFreezeBatchesLocked() {
	if m.freezeState == nil {
		return
	}
	var kept []*TimelineFreezeBatch
	for _, batch := range m.freezeState.Batches {
		if batch != nil && len(batch.IDs) > 0 {
			kept = append(kept, batch)
		}
	}
	m.freezeState.Batches = kept
}

// Legacy dumps had only a promotion watermark. Bootstrap once during restore,
// preserving that committed prefix; prompt rendering itself remains read-only.
func (m *Timeline) restoreFreezeStateLocked() {
	if m.freezeState != nil {
		m.pruneEmptyFreezeBatchesLocked()
		return
	}
	var watermark int64
	if m.promotedState != nil {
		watermark = m.promotedState.Watermark
	}
	m.freezeState = &TimelineFreezeState{}
	if watermark > 0 {
		for _, block := range m.freezeBudgetGroupsLocked(false) {
			batch := &TimelineFreezeBatch{BucketStart: block.BucketStart, BucketEnd: block.BucketEnd, InitialTaskID: block.initialTaskID}
			for _, item := range block.Items {
				if item.GetID() <= watermark {
					batch.IDs = append(batch.IDs, item.GetID())
				}
			}
			if len(batch.IDs) > 0 {
				batch.Nonce = fmt.Sprintf("f%dt%d", batch.IDs[0], batch.BucketStart.Unix())
				m.freezeState.Batches = append(m.freezeState.Batches, batch)
			}
		}
		if len(m.freezeState.Batches) > 0 {
			m.freezeState.Version++
		}
	}
	m.freezeLocked(false)
}

// A byte split can start with tool output lacking an explicit task label.
// Carry the preceding task context within the same calendar bucket.
func (m *Timeline) openTaskSeedLocked(view *Timeline) string {
	if m.freezeState == nil || len(m.freezeState.Batches) == 0 || view.idToTimelineItem.Len() == 0 {
		return ""
	}
	batch := m.freezeState.Batches[len(m.freezeState.Batches)-1]
	first, ok := view.idToTimelineItem.GetByIndex(0)
	if !ok || !alignToBucket(first.createdAt, TimelineDumpDefaultIntervalMinutes).Equal(batch.BucketStart) {
		return ""
	}
	taskID := batch.InitialTaskID
	for _, id := range batch.IDs {
		if item, ok := m.idToTimelineItem.Get(id); ok && item != nil && !item.deleted {
			advanceTimelineTaskContext(&taskID, item)
		}
	}
	return taskID
}
