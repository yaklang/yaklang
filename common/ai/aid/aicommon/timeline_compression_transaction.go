package aicommon

// One atomic transaction publishes summary, freeze membership and promotions.
// Automatic scheduling lives in reactloops/prompt.go.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type TimelineCompressedHead struct {
	Text             string `json:"text"`
	CoveredEndItemID int64  `json:"covered_end_item_id"`
	CoveredEndAtMs   int64  `json:"covered_end_at_ms"`
	Version          int64  `json:"version"`
}

type TimelineCompressedHistoryNode struct {
	Version          int64  `json:"version"`
	PrevVersion      int64  `json:"prev_version"`
	Text             string `json:"text"`
	CoveredEndItemID int64  `json:"covered_end_item_id"`
	CoveredEndAtMs   int64  `json:"covered_end_at_ms"`
	CreatedAtMs      int64  `json:"created_at_ms"`
}

// TimelineCompressionStage describes actual compression work or blocking waits.
type TimelineCompressionStage string

const (
	TimelineCompressionOptimizing TimelineCompressionStage = "optimizing"
	TimelineCompressionWaiting    TimelineCompressionStage = "waiting"
	TimelineCompressionMemory     TimelineCompressionStage = "memory"
)

// ContextCompressionAction is the only action on a main-loop compression turn.
const ContextCompressionAction = "compress_context"

const (
	TimelineCompressionMaxInputTokens   = 512 * 1024
	TimelineCompressionMaxSummaryTokens = 16 * 1024
)

// TimelineCompressionOptions describes one explicit compression, not automatic
// scheduling policy. PromptTokens and TargetTokens refer to the entire
// assembled main-loop prompt, not a separately measured Timeline budget.
type TimelineCompressionOptions struct {
	Context          context.Context
	MaxSummaryTokens int
	PromptTokens     int64
	TargetTokens     int64
	// ThroughID limits the guide to history at or below this ID. Nil uses the current timeline.
	ThroughID *int64
	// OnProgress runs synchronously outside Timeline locks, only when work or
	// waiting actually occurs. It must not schedule another compression.
	OnProgress func(TimelineCompressionStage)
}

func (o TimelineCompressionOptions) reportProgress(stage TimelineCompressionStage) {
	if o.OnProgress != nil {
		o.OnProgress(stage)
	}
}

type TimelineCompressionResult struct {
	ThroughID     int64
	RetiredIDs    []int64
	RetainedIDs   []int64
	RetainedRange string
	Summary       string
	InputTokens   int
	SummaryTokens int
	// Original promoted item IDs actually retired, never aggregate handles.
	RetiredPromotedEntryIDs []int64
}

// compressionSourceStateLocked detects edits, rollback, late insertions and
// exact-state changes inside the captured range. Appends beyond it are allowed.
func (m *Timeline) compressionSourceStateLocked(through int64) (string, error) {
	type source struct {
		ID           int64
		Timestamp    int64
		HasTimestamp bool
		Item         *TimelineItem
	}
	var sources []source
	ids := append([]int64(nil), m.idToTimelineItem.Keys()...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if id > through {
			break
		}
		item, _ := m.idToTimelineItem.Get(id)
		if item == nil || item.deleted {
			continue
		}
		ts, ok := m.idToTs.Get(id)
		sources = append(sources, source{id, ts, ok, item})
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return "", fmt.Errorf("snapshot compression source: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

// applyCompressionLocked commits one parsed compress_context selection.
// The caller holds Timeline.mu. The snapshot taken here is only the pre-delete
// memory source; retain and retire decisions come from selection.
func (m *Timeline) applyCompressionLocked(selection *TimelineCompressionSelection, requestPrompt string, trajectoryConfig *Config, response TrajectoryResponseReceipt) (*TimelineCompressionResult, func(), error) {
	if selection == nil {
		return nil, nil, fmt.Errorf("compression selection is nil")
	}
	// The model can summarize only the history rendered in its request.
	// Appends after that boundary remain Open, including new promoted versions.
	throughID := m.getMaxIDLocked()
	if selection.ThroughID != nil {
		throughID = *selection.ThroughID
	}
	source, err := m.captureCompressionSnapshotThroughLocked(throughID)
	if err != nil {
		return nil, nil, err
	}
	observed := compressionObservedIDs(selection.ObservedIDs)
	for _, ids := range [][]int64{selection.RetainedIDs, selection.RetiredPromotedEntries} {
		for _, id := range ids {
			if id > source.ThroughID {
				return nil, nil, fmt.Errorf("compression item %d is outside the request history boundary %d", id, source.ThroughID)
			}
		}
	}
	for _, id := range selection.RetiredPromotedEntries {
		if observed != nil && !observed[id] {
			return nil, nil, fmt.Errorf("compression item %d was not present in the request", id)
		}
	}
	_ = prepareCompressionSnapshot(source)
	summary := strings.TrimSpace(selection.Summary)
	if summary == "" && source.hasSummarySource() {
		return nil, nil, fmt.Errorf("invalid compression selection")
	}

	// 选择直接来自 action：保留的普通历史、点名淘汰的技能和证据。
	result, head := compressionSelectionResult(source, selection, summary)
	memorySource := m.compressionObservedSourceLocked(source, selection.ObservedIDs)
	trajectoryNotify := trajectoryConfig.compressionTrajectoryNotification(requestPrompt, result, response)

	// 观察者要在删除前拿到冻结回执，回调本身等锁释放后再执行。
	completion := newTimelineCompressionCompletion(selection)
	notify, freeze := m.compressionCallbackLocked(requestPrompt, completion, result)
	version := int64(0)
	if m.freezeState != nil {
		version = m.freezeState.Version
	}
	m.publishCompressionLocked(source, result, head, version+1)
	m.noteCompressionPendingBytes(notify, freeze)

	// 记忆登记用的是删除前的来源，不是已经淘汰过的 timeline。
	memorySource.SummaryPrompt = requestPrompt
	memorySource.MemoryCompletion = completion
	if m.sessionMemory == nil {
		m.sessionMemory = newTimelineSessionMemory(m, nil)
	}
	m.sessionMemory.record(memorySource)
	m.markCompressionMemoryCoveredLocked(memorySource)
	return result, func() {
		if trajectoryNotify != nil {
			trajectoryNotify()
		}
		if notify != nil {
			notify()
		}
	}, nil
}

// compressionSelectionResult applies the whitelist to ordinary history.
// Promoted IDs in the selection are retired as given; ordinary IDs not listed are summarized away.
func compressionSelectionResult(source *timelineCompressionSnapshot, selection *TimelineCompressionSelection, summary string) (*TimelineCompressionResult, *TimelineCompressedHead) {
	result := &TimelineCompressionResult{
		ThroughID: source.ThroughID, Summary: summary,
		InputTokens: source.InputTokens, SummaryTokens: MeasureTokens(summary),
		RetainedIDs:             append([]int64(nil), selection.RetainedIDs...),
		RetainedRange:           selection.RetainedRange,
		RetiredPromotedEntryIDs: append([]int64(nil), selection.RetiredPromotedEntries...),
	}
	selected := make(map[int64]bool, len(result.RetainedIDs))
	for _, id := range result.RetainedIDs {
		selected[id] = true
	}
	observed := compressionObservedIDs(selection.ObservedIDs)
	head := &TimelineCompressedHead{Text: summary}
	if source.Head != nil {
		head.CoveredEndItemID, head.CoveredEndAtMs = source.Head.CoveredEndItemID, source.Head.CoveredEndAtMs
	}
	for _, item := range source.Items {
		if observed != nil && !observed[item.ID] && !selected[item.ID] {
			// Late inserts below the watermark must be included in freeze
			// membership, otherwise they would fall behind the new boundary.
			result.RetainedIDs = append(result.RetainedIDs, item.ID)
			selected[item.ID] = true
		}
		if !selected[item.ID] {
			result.RetiredIDs = append(result.RetiredIDs, item.ID)
		}
		if item.ID >= head.CoveredEndItemID {
			head.CoveredEndItemID, head.CoveredEndAtMs = item.ID, item.Timestamp
		}
	}
	result.RetiredIDs = append(result.RetiredIDs, result.RetiredPromotedEntryIDs...)
	sort.Slice(result.RetainedIDs, func(i, j int) bool { return result.RetainedIDs[i] < result.RetainedIDs[j] })
	sort.Slice(result.RetiredIDs, func(i, j int) bool { return result.RetiredIDs[i] < result.RetiredIDs[j] })
	return result, head
}

func compressionObservedIDs(ids []int64) map[int64]bool {
	if ids == nil {
		return nil
	}
	observed := make(map[int64]bool, len(ids))
	for _, id := range ids {
		observed[id] = true
	}
	return observed
}

// Memory receipts cover only model-visible source identities. Preserve the
// complete apply-time source separately for freeze membership and late results.
func (m *Timeline) compressionObservedSourceLocked(source *timelineCompressionSnapshot, ids []int64) *timelineCompressionSnapshot {
	observed := compressionObservedIDs(ids)
	if observed == nil {
		return source
	}
	copy := *source
	copy.ObservedIDs = append([]int64{}, ids...)
	copy.Items, copy.PromotedItems, copy.ExactItemIDs, copy.UserContexts = nil, nil, nil, nil
	copy.ExactText = ""
	for _, item := range source.Items {
		if observed[item.ID] {
			copy.Items = append(copy.Items, item)
		}
	}
	for _, item := range source.PromotedItems {
		if observed[item.ID] {
			copy.PromotedItems = append(copy.PromotedItems, item)
		}
	}
	for _, id := range source.ExactItemIDs {
		if !observed[id] {
			continue
		}
		copy.ExactItemIDs = append(copy.ExactItemIDs, id)
		if item, ok := m.idToTimelineItem.Get(id); ok {
			if op := timelinePromotionForItem(item); op != nil && op.Kind != TimelinePromotedKindUserInput {
				copy.ExactText += "\n" + op.OpenPromptText()
			}
		}
	}
	for _, entry := range source.UserContexts {
		if observed[entry.ID] {
			copy.UserContexts = append(copy.UserContexts, entry)
		}
	}
	evidence, _ := m.evidenceStoreMatchingLocked(func(id int64) bool { return observed[id] })
	copy.Evidence = renderEvidenceItems(evidence.Items)
	_ = prepareCompressionSnapshot(&copy)
	return &copy
}

// publishCompressionLocked installs the freeze set, retires the chosen items,
// rebuilds promoted state, and publishes the new summary head.
func (m *Timeline) publishCompressionLocked(source *timelineCompressionSnapshot, result *TimelineCompressionResult, head *TimelineCompressedHead, version int64) {
	promotedIDs := make([]int64, 0, len(source.PromotedItems))
	for _, item := range source.PromotedItems {
		promotedIDs = append(promotedIDs, item.ID)
	}
	m.freezeState = m.compressionFreezeStateLocked(source.ThroughID, source.ExactItemIDs, promotedIDs, result.RetainedIDs, result.RetiredPromotedEntryIDs, version)
	for _, id := range result.RetiredIDs {
		m.retireTimelineItemLocked(id)
	}
	m.rebuildPromotedStateLocked(source.ThroughID)
	m.updateCompressedHead(head)
}

func (m *Timeline) noteCompressionPendingBytes(notify func(), freeze *TimelineFreezeResult) {
	if notify == nil || freeze == nil {
		return
	}
	for _, pending := range m.freezeBudgetGroupsLocked(false) {
		freeze.PendingBytes += len(pending.Render())
	}
}

// Build all freeze metadata before publishing or retiring any original.
// Deterministic compaction keeps every ordinary item; AI uses its whitelist.
func (m *Timeline) compressionFreezeStateLocked(through int64, exactIDs, promotedIDs, retainedIDs, retiredPromotedIDs []int64, version int64) *TimelineFreezeState {
	// Exact and promoted journals retain freeze membership. The watermark
	// sentinel survives serialization even when every original was retired.
	ids := append([]int64(nil), exactIDs...)
	retiredSet := make(map[int64]bool, len(retiredPromotedIDs))
	for _, id := range retiredPromotedIDs {
		retiredSet[id] = true
	}
	for _, id := range promotedIDs {
		if !retiredSet[id] {
			ids = append(ids, id)
		}
	}
	ids = append(ids, retainedIDs...)
	ids = append(ids, through)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	unique := ids[:0]
	for _, id := range ids {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	var start, end time.Time
	for _, id := range unique {
		if ts, ok := m.idToTs.Get(id); ok {
			at := time.UnixMilli(ts)
			if start.IsZero() || at.Before(start) {
				start = at
			}
			if end.IsZero() || at.After(end) {
				end = at
			}
		}
	}
	batch := &TimelineFreezeBatch{IDs: unique, BucketStart: start, BucketEnd: end,
		Nonce: fmt.Sprintf("c%dv%d", through, version)}
	return &TimelineFreezeState{Version: version, Batches: []*TimelineFreezeBatch{batch}}
}

func cloneTimelineCompressedHead(head *TimelineCompressedHead) *TimelineCompressedHead {
	if head == nil {
		return nil
	}
	cp := *head
	return &cp
}

func cloneTimelineCompressedHistory(history []*TimelineCompressedHistoryNode) []*TimelineCompressedHistoryNode {
	if len(history) == 0 {
		return nil
	}
	out := make([]*TimelineCompressedHistoryNode, 0, len(history))
	for _, h := range history {
		if h == nil {
			continue
		}
		cp := *h
		out = append(out, &cp)
	}
	return out
}

func (m *Timeline) updateCompressedHead(newHead *TimelineCompressedHead) {
	if m == nil || newHead == nil {
		return
	}
	newHead.Text = strings.TrimSpace(newHead.Text)
	if newHead.Text == "" {
		return
	}
	if m.compressedHead != nil {
		prev := m.compressedHead
		prevVersion := prev.Version - 1
		if prevVersion < 0 {
			prevVersion = 0
		}
		m.compressedHistory = append(m.compressedHistory, &TimelineCompressedHistoryNode{
			Version:          prev.Version,
			PrevVersion:      prevVersion,
			Text:             prev.Text,
			CoveredEndItemID: prev.CoveredEndItemID,
			CoveredEndAtMs:   prev.CoveredEndAtMs,
			CreatedAtMs:      time.Now().UnixMilli(),
		})
	}
	if newHead.Version <= 0 {
		if m.compressedHead == nil {
			newHead.Version = 1
		} else {
			newHead.Version = m.compressedHead.Version + 1
		}
	}
	m.compressedHead = cloneTimelineCompressedHead(newHead)
}

func sameTimelineCompressedHead(a, b *TimelineCompressedHead) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (m *Timeline) validateCompressionSourceLocked(snapshot *timelineCompressionSnapshot) (int64, error) {
	if snapshot.Context != nil {
		if err := snapshot.Context.Err(); err != nil {
			return 0, err
		}
	}
	version := int64(0)
	if m.freezeState != nil {
		version = m.freezeState.Version
	}
	state, err := m.compressionSourceStateLocked(snapshot.ThroughID)
	if err != nil {
		return 0, err
	}
	if version != snapshot.FreezeVersion || !sameTimelineCompressedHead(m.compressedHead, snapshot.Head) || state != snapshot.SourceState {
		return 0, fmt.Errorf("timeline changed inside compression snapshot; discarding stale summary")
	}
	return version, nil
}
