package aicommon

// One atomic transaction publishes summary, freeze membership and promotions.
// Automatic scheduling lives in timeline_compression_before_prompt.go.

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

// TimelineCompressionAction is the only action on a main-loop compression turn.
const TimelineCompressionAction = "compress_timeline"

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

func (m *Timeline) commitCompressionRound(snapshot *timelineCompressionSnapshot, summary string) (*timelineCompressionCommit, error) {
	result, err := m.commitCompressionSnapshot(snapshot, summary)
	if err != nil {
		return nil, err
	}
	return &timelineCompressionCommit{Result: result, Notify: snapshot.NotifyCommitted}, nil
}

func (m *Timeline) commitCompressionSnapshot(snapshot *timelineCompressionSnapshot, summary string) (*TimelineCompressionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if snapshot == nil || m.compressionSnapshot != snapshot || (strings.TrimSpace(summary) == "" && snapshot.hasSummarySource()) {
		return nil, fmt.Errorf("invalid or inactive compression transaction")
	}
	version, err := m.validateCompressionSourceLocked(snapshot)
	if err != nil {
		return nil, err
	}
	result := &TimelineCompressionResult{ThroughID: snapshot.ThroughID, Summary: summary,
		InputTokens: snapshot.InputTokens, SummaryTokens: MeasureTokens(summary)}
	selected := make(map[int64]bool)
	if snapshot.Output != nil {
		result.RetainedIDs = append([]int64(nil), snapshot.Output.RetainedIDs...)
		result.RetainedRange = snapshot.Output.RetainedRange
		// Parsing already restricts this list to promoted IDs in the snapshot.
		// Apply exactly those IDs without expanding or cancelling the selection.
		result.RetiredPromotedEntryIDs = append([]int64(nil), snapshot.Output.RetiredPromotedEntries...)
		for _, id := range result.RetainedIDs {
			selected[id] = true
		}
	}
	head := &TimelineCompressedHead{Text: summary}
	if snapshot.Head != nil {
		head.CoveredEndItemID, head.CoveredEndAtMs = snapshot.Head.CoveredEndItemID, snapshot.Head.CoveredEndAtMs
	}
	for _, item := range snapshot.Items {
		if !selected[item.ID] {
			result.RetiredIDs = append(result.RetiredIDs, item.ID)
		}
		if item.ID >= head.CoveredEndItemID {
			head.CoveredEndItemID, head.CoveredEndAtMs = item.ID, item.Timestamp
		}
	}
	result.RetiredIDs = append(result.RetiredIDs, result.RetiredPromotedEntryIDs...)
	sort.Slice(result.RetiredIDs, func(i, j int) bool { return result.RetiredIDs[i] < result.RetiredIDs[j] })
	snapshot.NotifyCommitted = m.compressionCallbackLocked(snapshot, result)
	m.freezeState = m.compressionFreezeStateLocked(snapshot, result.RetainedIDs, result.RetiredPromotedEntryIDs, version+1)
	for _, id := range result.RetiredIDs {
		m.retireTimelineItemLocked(id)
	}
	m.rebuildPromotedStateLocked(snapshot.ThroughID)
	m.updateCompressedHead(head)
	if snapshot.MemoryCompletion != nil {
		if m.sessionMemory == nil {
			m.sessionMemory = newTimelineSessionMemory(m, nil)
		}
		m.sessionMemory.record(snapshot)
		m.markCompressionMemoryCoveredLocked(snapshot)
	}
	if snapshot.NotifyCommitted != nil {
		for _, pending := range m.freezeBudgetGroupsLocked(false) {
			snapshot.CommittedFreeze.PendingBytes += len(pending.Render())
		}
	}
	return result, nil
}

// Build all freeze metadata before publishing or retiring any original.
// Deterministic compaction keeps every ordinary item; AI uses its whitelist.
func (m *Timeline) compressionFreezeStateLocked(snapshot *timelineCompressionSnapshot, retainedIDs, retiredPromotedIDs []int64, version int64) *TimelineFreezeState {
	// Exact and promoted journals retain freeze membership. The watermark
	// sentinel survives serialization even when every original was retired.
	ids := append([]int64(nil), snapshot.ExactItemIDs...)
	retiredSet := make(map[int64]bool, len(retiredPromotedIDs))
	for _, id := range retiredPromotedIDs {
		retiredSet[id] = true
	}
	for _, item := range snapshot.PromotedItems {
		if !retiredSet[item.ID] {
			ids = append(ids, item.ID)
		}
	}
	ids = append(ids, retainedIDs...)
	ids = append(ids, snapshot.ThroughID)
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
		Nonce: fmt.Sprintf("c%dv%d", snapshot.ThroughID, version)}
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
