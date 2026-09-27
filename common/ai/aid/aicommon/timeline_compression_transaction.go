package aicommon

// One atomic transaction publishes summary, freeze membership and promotions.
// Automatic scheduling lives in timeline_compression_before_prompt.go.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
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

// TimelineCompressionOptions describes one explicit compression, not automatic
// scheduling policy. Safety limits reject oversized input/output without loss;
// they are never requested target lengths or compression ratios in the prompt.
type TimelineCompressionOptions struct {
	Context          context.Context
	MaxInputTokens   int
	MaxSummaryTokens int
	RetainedContext  map[string]string // actual independent prompt fields, not inferred from their names
}

var errTimelineCompressionBusy = errors.New("timeline compression is already running")
var errTimelineCompressionBeforePromptChanged = errors.New("timeline changed after threshold check")

type TimelineCompressionResult struct {
	ThroughID     int64
	RetiredIDs    []int64
	Summary       string
	InputTokens   int
	SummaryTokens int
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

// CompressOnce is the explicit compression/retry entry used by before-prompt checks.
// It reserves a snapshot, invokes the configured auxiliary AI
// outside Timeline.mu, then publishes all state changes under one write lock.
func (m *Timeline) CompressOnce(options TimelineCompressionOptions) (result *TimelineCompressionResult, err error) {
	return m.compressOnce(options, nil)
}

func (m *Timeline) compressOnce(options TimelineCompressionOptions, checked *timelineCompressionSnapshot) (result *TimelineCompressionResult, err error) {
	if m == nil || options.MaxInputTokens <= 0 || options.MaxSummaryTokens <= 0 {
		return nil, fmt.Errorf("compression requires a timeline and positive input/summary safety limits")
	}
	m.mu.Lock()
	if m.compressing {
		m.mu.Unlock()
		return nil, errTimelineCompressionBusy
	}
	snapshot, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	// The threshold was measured outside the lock. A different before-prompt check may
	// have finished meanwhile; never summarize its newly committed head again.
	if checked != nil && (m.totalDumpContentLimit != checked.BeforePromptLimit || snapshot.SourceState != checked.SourceState || snapshot.FreezeVersion != checked.FreezeVersion || !sameTimelineCompressedHead(snapshot.Head, checked.Head)) {
		m.mu.Unlock()
		return nil, errTimelineCompressionBeforePromptChanged
	}
	m.compressing, m.compressionSnapshot = true, snapshot
	m.compressionDone = make(chan struct{})
	m.mu.Unlock()
	defer func() {
		if recovered := recover(); recovered != nil {
			result, err = nil, fmt.Errorf("timeline compression panicked: %v", recovered)
		}
		m.mu.Lock()
		// Publish backoff before waking waiting before-prompt callers, otherwise a failed
		// request could be immediately repeated by the next waiting caller.
		if err != nil && checked != nil {
			m.compressionLastFailure = checked.BeforePromptFingerprint
			m.compressionRetryAfter = time.Now().Add(timelineCompressionRetryDelay)
		} else if err == nil {
			m.compressionLastFailure = ""
			m.compressionRetryAfter = time.Time{}
		}
		m.compressing, m.compressionSnapshot = false, nil
		close(m.compressionDone)
		m.compressionDone = nil
		m.mu.Unlock()
	}()
	snapshot.RetainedContext = make(map[string]string, len(options.RetainedContext))
	for key, value := range options.RetainedContext {
		snapshot.RetainedContext[key] = value
	}
	if err := prepareCompressionSnapshot(snapshot); err != nil && len(snapshot.ExactItemIDs) == 0 {
		return nil, err
	}
	if options.Context == nil && m.config != nil {
		options.Context = m.config.GetContext()
	}
	if options.Context == nil {
		options.Context = context.Background()
	}
	snapshot.Context = options.Context
	if err := options.Context.Err(); err != nil {
		return nil, err
	}
	if snapshot.InputText == "" {
		return m.commitCompressionSnapshot(snapshot, "")
	}
	summary, err := m.summarizeCompressionSnapshot(snapshot, options)
	if err != nil {
		return nil, err
	}
	if err := options.Context.Err(); err != nil {
		return nil, err
	}
	return m.commitCompressionSnapshot(snapshot, summary)
}

func (m *Timeline) commitCompressionSnapshot(snapshot *timelineCompressionSnapshot, summary string) (*TimelineCompressionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if snapshot == nil || m.compressionSnapshot != snapshot || (strings.TrimSpace(summary) == "" && snapshot.InputText != "") {
		return nil, fmt.Errorf("invalid or inactive compression transaction")
	}
	if snapshot.Context != nil {
		if err := snapshot.Context.Err(); err != nil {
			return nil, err
		}
	}
	version := int64(0)
	if m.freezeState != nil {
		version = m.freezeState.Version
	}
	state, err := m.compressionSourceStateLocked(snapshot.ThroughID)
	if err != nil {
		return nil, err
	}
	if version != snapshot.FreezeVersion || !sameTimelineCompressedHead(m.compressedHead, snapshot.Head) || state != snapshot.SourceState {
		return nil, fmt.Errorf("timeline changed inside compression snapshot; discarding stale summary")
	}
	result := &TimelineCompressionResult{ThroughID: snapshot.ThroughID, Summary: summary,
		InputTokens: snapshot.InputTokens, SummaryTokens: MeasureTokens(summary)}
	head := &TimelineCompressedHead{Text: summary}
	if snapshot.Head != nil {
		head.CoveredEndItemID, head.CoveredEndAtMs = snapshot.Head.CoveredEndItemID, snapshot.Head.CoveredEndAtMs
	}
	for _, item := range snapshot.Items {
		result.RetiredIDs = append(result.RetiredIDs, item.ID)
		if item.ID >= head.CoveredEndItemID {
			head.CoveredEndItemID, head.CoveredEndAtMs = item.ID, item.Timestamp
		}
	}
	// Exact journals retain freeze membership. The watermark sentinel survives
	// serialization even when every ordinary original has been retired.
	ids := append([]int64(nil), snapshot.ExactItemIDs...)
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
		Nonce: fmt.Sprintf("c%dv%d", snapshot.ThroughID, version+1)}
	// Everything below is local, non-failing state publication; no callback or
	// AI call is allowed between freezing/promoting and publishing the new head.
	m.freezeState = &TimelineFreezeState{Version: version + 1, Batches: []*TimelineFreezeBatch{batch}}
	m.rebuildPromotedStateLocked(snapshot.ThroughID)
	m.updateCompressedHead(head)
	for _, id := range result.RetiredIDs {
		m.retireTimelineItemLocked(id)
	}
	return result, nil
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
