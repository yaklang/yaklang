package aicommon

// Timeline compression: size checks, AI reduction, compressed history, and
// the storage emergency path live together here. Freeze membership is in
// timeline_freeze.go; persistence is in timeline_marshal.go.

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
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

// timelineCompressionSnapshot is a detached plan, not a running compression.
// It contains no references to mutable TimelineItem values and never freezes,
// deletes, promotes or calls AI. The production trigger still uses the old path.
type timelineCompressionSnapshot struct {
	Head            *TimelineCompressedHead
	ThroughID       int64 // captured watermark, including exact state and tombstones
	FrozenThroughID int64
	Items           []timelineCompressionSnapshotItem // all live ordinary items, in ID order
	ExactItemIDs    []int64                           // never candidates for summary/deletion
	InputText       string                            // old head + ordinary Frozen/Open content
	InputTokens     int
	RetainedContext map[string]string // caller-owned prompt parts, copied before the AI request
	SourceState     string            // ordinary + exact journal entries through ThroughID, including tombstones
	FreezeVersion   int64
}

type timelineCompressionSnapshotItem struct {
	ID         int64
	Timestamp  int64
	Frozen     bool
	SourceJSON string // original content, PromptText and shrink fields for later conflict checks
	PromptText string // detached, rendered prompt view; empty for omitted bookkeeping
}

// buildCompressionSnapshot includes the old summary and all ordinary Frozen/Open
// history. There is no retained suffix or fixed compression ratio.
func (m *Timeline) buildCompressionSnapshot() (*timelineCompressionSnapshot, error) {
	if m == nil {
		return nil, fmt.Errorf("compression snapshot requires a timeline")
	}
	snapshot, err := m.captureCompressionSnapshot()
	if err != nil {
		return nil, err
	}
	if err := prepareCompressionSnapshot(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func prepareCompressionSnapshot(snapshot *timelineCompressionSnapshot) error {
	previous := ""
	if snapshot.Head != nil {
		previous = snapshot.Head.Text
	}
	ordinary := renderCompressionSnapshotItems(snapshot.Items)
	if strings.TrimSpace(previous) == "" && strings.TrimSpace(ordinary) == "" {
		return fmt.Errorf("compression snapshot has no visible ordinary history")
	}
	snapshot.InputText = strings.TrimSpace(previous + "\n" + ordinary)
	snapshot.InputTokens = MeasureTokens(snapshot.InputText)
	return nil
}

// Copy under the read lock; tokenization happens after releasing it so writers
// can append the next open segment while this detached snapshot is budgeted.
func (m *Timeline) captureCompressionSnapshot() (*timelineCompressionSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.captureCompressionSnapshotLocked()
}

func (m *Timeline) captureCompressionSnapshotLocked() (*timelineCompressionSnapshot, error) {
	snapshot := &timelineCompressionSnapshot{
		Head:      cloneTimelineCompressedHead(m.compressedHead),
		ThroughID: m.getMaxIDLocked(), FrozenThroughID: m.frozenThroughLocked(),
	}
	if m.freezeState != nil {
		snapshot.FreezeVersion = m.freezeState.Version
	}
	var err error
	snapshot.SourceState, err = m.compressionSourceStateLocked(snapshot.ThroughID)
	if err != nil {
		return nil, err
	}
	ids := append([]int64(nil), m.idToTimelineItem.Keys()...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		if item.value == nil || item.GetID() != id {
			return nil, fmt.Errorf("invalid compression source item %d", id)
		}
		if isPromotableTimelineItem(item) {
			snapshot.ExactItemIDs = append(snapshot.ExactItemIDs, id)
			continue
		}
		ts, ok := m.idToTs.Get(id)
		if !ok {
			return nil, fmt.Errorf("compression source item %d has no timestamp", id)
		}
		// Native action replay is written as one validated assistant + N tools
		// envelope in PromptText. Reject a broken envelope instead of choosing
		// a cut through an incomplete protocol group. Literal tags in ordinary
		// tool/user data are not interpreted here.
		if text, ok := timelineTextItem(item); ok &&
			normalizeTimelinePromptCategory(extractTextEntryType(text.Text)) == "FUNCTION_CALL_ACTION_RESPONSE" &&
			strings.TrimSpace(text.PromptText) != "" {
			body := text.PromptText
			if normalizeTimelinePromptCategory(extractTextEntryType(body)) == "FUNCTION_CALL_ACTION_RESPONSE" {
				location := withTaskRegex.FindStringIndex(body)
				if location == nil {
					location = withoutTaskRegex.FindStringIndex(body)
				}
				if location != nil {
					body = body[location[1]:]
				}
			}
			if _, err := aiprojection.RebindReplayNonce(body, aiprojection.Nonce()); err != nil {
				return nil, fmt.Errorf("incomplete action replay at timeline item %d: %w", id, err)
			}
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, fmt.Errorf("snapshot timeline item %d: %w", id, err)
		}
		entry := timelineCompressionSnapshotItem{
			ID: id, Timestamp: ts, Frozen: id <= snapshot.FrozenThroughID, SourceJSON: string(raw),
		}
		if projected := projectTimelineItemForPromptWithModelReplay(item, true); projected != nil {
			// Keep the entire prompt body, including long single-line replay JSON.
			// The presentation renderer uses a bounded line scanner and may omit
			// oversized lines; it must not determine compression cuts or budgets.
			// Do not substitute previous per-item shrink results for full originals.
			entry.PromptText = fmt.Sprintf("# item=%d timestamp_ms=%d [%s]\n%s",
				id, ts, renderItemTypeVerbose(projected), projected.value.String())
		}
		snapshot.Items = append(snapshot.Items, entry)
	}
	return snapshot, nil
}

func renderCompressionSnapshotItems(items []timelineCompressionSnapshotItem) string {
	var texts []string
	for _, item := range items {
		if item.PromptText != "" {
			texts = append(texts, item.PromptText)
		}
	}
	return strings.Join(texts, "\n")
}

//go:embed prompts/timeline/compression.txt
var timelineCompressionTemplate string

//go:embed prompts/timeline/compression.json
var timelineCompressionSchema string

// TimelineCompressionOptions describes one explicit compression, not automatic
// scheduling policy. Safety limits reject oversized input/output without loss;
// they are never requested target lengths or compression ratios in the prompt.
type TimelineCompressionOptions struct {
	MaxInputTokens   int
	MaxSummaryTokens int
	RetainedContext  map[string]string // actual independent prompt fields, not inferred from their names
}

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
		ts, ok := m.idToTs.Get(id)
		sources = append(sources, source{id, ts, ok, item})
	}
	raw, err := json.Marshal(sources)
	if err != nil {
		return "", fmt.Errorf("snapshot compression source: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

// CompressOnce is the explicit step-2/3 entry. No production write/threshold
// path calls it yet. It reserves a snapshot, invokes the configured auxiliary AI
// outside Timeline.mu, then publishes all state changes under one write lock.
func (m *Timeline) CompressOnce(options TimelineCompressionOptions) (result *TimelineCompressionResult, err error) {
	if m == nil || options.MaxInputTokens <= 0 || options.MaxSummaryTokens <= 0 {
		return nil, fmt.Errorf("compression requires a timeline and positive input/summary safety limits")
	}
	m.mu.Lock()
	if m.compressing {
		m.mu.Unlock()
		return nil, fmt.Errorf("timeline compression is already running")
	}
	snapshot, err := m.captureCompressionSnapshotLocked()
	if err != nil {
		m.mu.Unlock()
		return nil, err
	}
	m.compressing, m.compressionSnapshot = true, snapshot
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.compressing, m.compressionSnapshot = false, nil
		m.mu.Unlock()
		if recovered := recover(); recovered != nil {
			result, err = nil, fmt.Errorf("timeline compression panicked: %v", recovered)
		}
	}()
	snapshot.RetainedContext = make(map[string]string, len(options.RetainedContext))
	for key, value := range options.RetainedContext {
		snapshot.RetainedContext[key] = value
	}
	if err := prepareCompressionSnapshot(snapshot); err != nil {
		return nil, err
	}
	summary, err := m.summarizeCompressionSnapshot(snapshot, options)
	if err != nil {
		return nil, err
	}
	if err := m.config.GetContext().Err(); err != nil {
		return nil, err
	}
	return m.commitCompressionSnapshot(snapshot, summary)
}

func (m *Timeline) commitCompressionSnapshot(snapshot *timelineCompressionSnapshot, summary string) (*TimelineCompressionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if snapshot == nil || m.compressionSnapshot != snapshot || strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("invalid or inactive compression transaction")
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
	retired := make(map[int64]struct{}, len(result.RetiredIDs))
	for _, id := range result.RetiredIDs {
		item, _ := m.idToTimelineItem.Get(id)
		item.deleted = true
		retired[id] = struct{}{}
	}
	// Restoring JSON creates separate values in the ID and timestamp indexes.
	// Retire both views so save/dump cannot resurrect summarized originals.
	m.tsToTimelineItem.ForEach(func(_ int64, item *TimelineItem) bool {
		if item != nil && item.value != nil {
			if _, ok := retired[item.GetID()]; ok {
				item.deleted = true
			}
		}
		return true
	})
	return result, nil
}

// renderCompressionSummaryPrompt renders one complete reduction request. Native
// replay is historical data here, not messages to execute/project in this helper.
// Redact the process nonce on this request-only copy, then JSON-encode the source
// so embedded tags cannot become projection controls or source delimiters.
// The output schema is supplied once by the auxiliary scheduler.
func renderCompressionSummaryPrompt(snapshot *timelineCompressionSnapshot) (string, error) {
	if snapshot == nil {
		return "", fmt.Errorf("invalid timeline compression snapshot")
	}
	previous := ""
	if snapshot.Head != nil {
		previous = snapshot.Head.Text
	}
	history := renderCompressionSnapshotItems(snapshot.Items)
	if strings.TrimSpace(previous) == "" && strings.TrimSpace(history) == "" {
		return "", fmt.Errorf("timeline compression has no history to summarize")
	}
	source, err := json.MarshalIndent(struct {
		RetainedContext map[string]string `json:"retained_context,omitempty"`
		PreviousSummary string            `json:"previous_summary"`
		OlderHistory    string            `json:"history_to_summarize"`
	}{snapshot.RetainedContext, aiprojection.RedactNonce(previous), aiprojection.RedactNonce(history)}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode compression source: %w", err)
	}
	tmpl, err := template.New("timeline-compression").Parse(timelineCompressionTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Source string
	}{aiprojection.RedactNonce(string(source))})
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// summarizeCompressionSnapshot is intentionally not wired to the production
// trigger. It schedules one complete reduction, without chunks, refinement or
// truncation. Transport/parser retries still follow the existing Config policy.
// It never commits, freezes, promotes or retires any Timeline content.
func (m *Timeline) summarizeCompressionSnapshot(snapshot *timelineCompressionSnapshot, limits ...TimelineCompressionOptions) (string, error) {
	if m == nil || m.config == nil {
		return "", fmt.Errorf("timeline compression requires an auxiliary scheduler")
	}
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	if err != nil {
		return "", err
	}
	// Capture the safety limits before invoking callbacks; don't reread mutable state
	// or the caller's snapshot after the request has started.
	var limit TimelineCompressionOptions
	if len(limits) > 0 {
		limit = limits[0]
	}
	if limit.MaxInputTokens > 0 && TokenCountExceeds(prompt+"\n"+timelineCompressionSchema, limit.MaxInputTokens) {
		return "", fmt.Errorf("timeline compression input exceeds safety limit %d; source preserved", limit.MaxInputTokens)
	}
	var summary string
	resultErr := fmt.Errorf("timeline compression skipped or returned no result")
	m.config.ScheduleAuxiliaryTask(m.config.GetContext(), CallerLabelTimelineCompress,
		func() string { return prompt },
		func(action *Action) {
			if action == nil {
				resultErr = fmt.Errorf("timeline compression returned no action")
				return
			}
			if err := action.WaitParseResult(m.config.GetContext()); err != nil {
				resultErr = fmt.Errorf("parse timeline compression: %w", err)
				return
			}
			if !action.ValidCheck("timeline-summary") {
				resultErr = fmt.Errorf("timeline compression returned an unexpected action")
				return
			}
			raw, exists := action.LookupCanonicalParam("summary")
			text, isString := raw.(string)
			if !exists || !isString {
				resultErr = fmt.Errorf("timeline compression summary must be a root string field")
				return
			}
			text = strings.TrimSpace(text)
			if text == "" {
				resultErr = fmt.Errorf("timeline compression returned an empty summary")
				return
			}
			if strings.Contains(text, aiprojection.Nonce()) {
				resultErr = fmt.Errorf("timeline compression returned a projection control token")
				return
			}
			if limit.MaxSummaryTokens > 0 && TokenCountExceeds(text, limit.MaxSummaryTokens) {
				resultErr = fmt.Errorf("timeline compression summary exceeds safety limit %d", limit.MaxSummaryTokens)
				return
			}
			summary, resultErr = text, nil
		},
		WithAuxiliaryOutputSchema("timeline-summary", timelineCompressionSchema),
		WithAuxiliaryOnError(func(err error) { resultErr = fmt.Errorf("timeline compression request failed: %w", err) }),
		WithAuxiliaryOpts(WithLiteForgeDisableTimeline(),
			WithLiteForgeMaxPromptTokens(limit.MaxInputTokens),
			WithGeneralConfigExtraRequestOpts(WithAIRequest_CallerLabel(CallerLabelTimelineCompress))),
	)
	return summary, resultErr
}

// MaxTimelineSaveSize is the maximum size (1.5MB storage limit) for timeline data when saving to database
const MaxTimelineSaveSize = 1536 * 1024

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

func (m *Timeline) calculateActualContentSize() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calculateActualContentSizeLocked()
}

func (m *Timeline) calculateActualContentSizeLocked() int64 {
	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	count := 0

	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		if isPromotableTimelineItem(item) {
			return true
		}
		initOnce.Do(func() {
			buf.WriteString("timeline:\n")
		})

		ts, ok := m.idToTs.Get(item.GetID())
		if !ok {
			log.Warnf("BUG: timeline id %v not found", item.GetID())
		}
		t := time.Unix(0, ts*int64(time.Millisecond))
		timeStr := t.Format(utils.DefaultTimeFormat3)

		if item.deleted {
			return true
		}

		buf.WriteString(fmt.Sprintf("--[%s]\n", timeStr))
		raw := selectShrunkContent(item)
		for _, line := range utils.ParseStringToRawLines(raw) {
			buf.WriteString(fmt.Sprintf("     %s\n", line))
		}
		count++
		return true
	})
	if count > 0 {
		return int64(ytoken.CalcTokenCount(buf.String()))
	}
	return 0
}

func (m *Timeline) dumpSizeCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dumpSizeCheckLocked()
}

func (m *Timeline) dumpSizeCheckLocked() {
	// 在 push 时检查内容大小，如果超过限制就压缩
	if m.totalDumpContentLimit <= 0 || m.autoCompressDisabled || m.compressing {
		return
	}

	// 获取当前内容大小（不包括reducer）
	contentSize := m.calculateActualContentSizeLocked()
	if contentSize <= m.totalDumpContentLimit {
		return // 内容大小正常
	}

	log.Infof("timeline content too large (%d > %d), triggering batch compression", contentSize, m.totalDumpContentLimit)

	// 压缩到合适的大小
	m.compressForSizeLimitLocked()
}

// emergencyCompress performs non-AI compression by removing oldest items
// This is used when timeline is too large and needs to be compressed without AI assistance
func (m *Timeline) emergencyCompress(targetSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emergencyCompressLocked(targetSize)
}

func (m *Timeline) emergencyCompressLocked(targetSize int) {
	if m == nil {
		return
	}

	// Calculate current size
	tlstr, err := marshalTimelineUnlocked(m)
	if err != nil {
		log.Errorf("emergency compress: failed to marshal timeline: %v", err)
		return
	}
	currentSize := len(tlstr)
	if currentSize <= targetSize {
		return // Already small enough
	}

	log.Warnf("emergency compress: current size %d, target size %d", currentSize, targetSize)

	// Get all item IDs ordered by timestamp (oldest first)
	var itemIDs []int64
	m.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		if item == nil || item.deleted || isPromotableTimelineItem(item) {
			return true
		}
		itemIDs = append(itemIDs, id)
		return true
	})

	if len(itemIDs) <= 1 {
		log.Warnf("emergency compress: only %d items left, cannot compress further", len(itemIDs))
		return
	}
	m.freezeLocked(true)

	// Keep at least one original item. Recheck after every removal: checking only
	// every ten items could discard nine more items than the storage bound needs.
	removedCount := 0
	var emergencySummaries []string
	var lastRemovedID int64
	for len(itemIDs) > 1 && currentSize > targetSize {
		// Remove the oldest item (first in the list)
		oldestID := itemIDs[0]
		itemIDs = itemIDs[1:]

		// Get the item for summary before removing
		item, ok := m.idToTimelineItem.Get(oldestID)
		if !ok || item == nil || item.deleted {
			continue
		}

		// Create a brief summary of what was removed (without AI)
		briefSummary := m.createEmergencySummary(item, oldestID)
		if briefSummary != "" {
			emergencySummaries = append(emergencySummaries, briefSummary)
		}
		lastRemovedID = oldestID
		item.deleted = true

		removedCount++

		tlstr, err = marshalTimelineUnlocked(m)
		if err != nil {
			log.Warnf("emergency compress: failed to measure after removal: %v", err)
			break
		}
		currentSize = len(tlstr)
	}
	if removedCount > 0 {
		var coveredEndAtMs int64
		if ts, ok := m.idToTs.Get(lastRemovedID); ok {
			coveredEndAtMs = ts
		}
		headText := strings.TrimSpace(strings.Join(emergencySummaries, "\n"))
		if m.compressedHead != nil && strings.TrimSpace(m.compressedHead.Text) != "" {
			if headText == "" {
				headText = m.compressedHead.Text
			} else {
				headText = m.compressedHead.Text + "\n" + headText
			}
		}
		if headText != "" {
			m.updateCompressedHead(&TimelineCompressedHead{
				Text:             headText,
				CoveredEndItemID: lastRemovedID,
				CoveredEndAtMs:   coveredEndAtMs,
			})
		}
	}

	// Final size check
	tlstr, _ = marshalTimelineUnlocked(m)
	log.Infof("emergency compress completed: removed %d items, final size: %d (target: %d)", removedCount, len(tlstr), targetSize)
}

// createEmergencySummary creates a brief summary of an item without AI assistance
func (m *Timeline) createEmergencySummary(item *TimelineItem, id int64) string {
	if item == nil {
		return ""
	}

	// Get timestamp
	ts, ok := m.idToTs.Get(id)
	if !ok {
		return ""
	}
	t := time.Unix(0, ts*int64(time.Millisecond))
	timeStr := t.Format(utils.DefaultTimeFormat3)

	// Create a very brief summary based on item type
	var summary string
	switch v := item.value.(type) {
	case *aitool.ToolResult:
		executionStatus, detail := v.GetExecutionStatus()
		switch {
		case !v.Success:
			summary = fmt.Sprintf("[%s] tool:%s protocol-error", timeStr, v.Name)
		case executionStatus == aitool.ToolExecutionStatusFailed:
			summary = fmt.Sprintf("[%s] tool:%s execution-failed", timeStr, v.Name)
		case executionStatus == aitool.ToolExecutionStatusSucceeded:
			summary = fmt.Sprintf("[%s] tool:%s execution-succeeded", timeStr, v.Name)
		default:
			summary = fmt.Sprintf("[%s] tool:%s protocol-completed; execution-outcome-unknown", timeStr, v.Name)
		}
		if detail != "" {
			// A tool can put arbitrarily large error text in status details.
			// Emergency summaries must remain smaller than the removed item.
			detail = ShrinkByTokens(detail, 64)
			summary += " (" + detail + ")"
		}
	case *UserInteraction:
		summary = fmt.Sprintf("[%s] user-interaction stage:%v", timeStr, v.Stage)
	case *TextTimelineItem:
		// Preserve UTF-8 when shortening user-visible text.
		runes := []rune(v.Text)
		text := v.Text
		if len(runes) > 50 {
			text = string(runes[:47]) + "..."
		}
		summary = fmt.Sprintf("[%s] text:%s", timeStr, text)
	default:
		summary = fmt.Sprintf("[%s] item removed (emergency compress)", timeStr)
	}

	return summary
}

// estimateItemContentTokens 按 calculateActualContentSize 一致的 wrap 格式估算单个 item 的 token 数
// 用于 batchCompress 切点：从最新端反向累加 token 找到保留区起点
// 注意：BPE token 化在多 item 拼接时不严格线性可加，本函数为近似估算（误差可接受）
// 关键词: estimateItemContentTokens, batchCompress 切点 token 估算
func (m *Timeline) estimateItemContentTokens(id int64, item *TimelineItem) int64 {
	if item == nil || item.deleted {
		return 0
	}
	var buf bytes.Buffer
	ts, _ := m.idToTs.Get(id)
	t := time.Unix(0, ts*int64(time.Millisecond))
	timeStr := t.Format(utils.DefaultTimeFormat3)

	buf.WriteString(fmt.Sprintf("--[%s]\n", timeStr))
	raw := selectShrunkContent(item)
	for _, line := range utils.ParseStringToRawLines(raw) {
		buf.WriteString(fmt.Sprintf("     %s\n", line))
	}
	return int64(ytoken.CalcTokenCount(buf.String()))
}

// findCompressSplitByRecentKeepTokens 找到 active 区按 token 大小划分的切点：
//
//	从最新端向旧端反向累加 token，累加首次 >= keepTokens 时停下，
//	返回 keepStartIdx：[0, keepStartIdx) 是 toCompress（最旧的，需要压缩），
//	[keepStartIdx, len) 是 recentKeep（最新的，保留不动）。
//
// 边界:
//   - 0 或 1 个活跃 item: 返回 0（不压缩）
//   - keepTokens <= 0:   至少保留最新 1 个 item
//   - 全部 item 都被纳入"最新保留区"才达到 keepTokens: 返回 0（不压缩，等价于全部都是最近）
//
// 关键词: findCompressSplitByRecentKeepTokens, batchCompress 切点, recent keep, token 维度
func (m *Timeline) findCompressSplitByRecentKeepTokens(keepTokens int64) int {
	if m == nil || m.idToTimelineItem == nil {
		return 0
	}
	activeIDs := m.getActiveTimelineItemIDs()
	total := len(activeIDs)
	if total <= 1 {
		return 0
	}

	if keepTokens <= 0 {
		// 至少保留最新 1 个 item
		return total - 1
	}

	var acc int64
	// 从最新端（数组尾部）向前累加 token
	for i := len(activeIDs) - 1; i >= 0; i-- {
		id := activeIDs[i]
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		acc += m.estimateItemContentTokens(id, item)
		if acc >= keepTokens {
			// 当前 i 即保留区起点；[0, i) 进入待压缩，[i, end] 留作最近保留
			return i
		}
	}
	// 全部 item 累加仍未达到 keepTokens => 全部都是"最近"，无需压缩
	return 0
}

// compressForSizeLimit 当活跃区 token 超过 totalDumpContentLimit 时，触发 batch compress：
//
//	keepTokens = currentSize / 6，按 token 反向累加从最新端向旧端切分，
//	[0, splitIdx) 进入 toCompress 一并压成 1 条 reducer，
//	[splitIdx, end] 进入 recentKeep 不动，并作为"现在 agent 在做什么"的 prompt 上下文一并喂给 AI。
//
// 关键词: compressForSizeLimit, recent keep token 切分, batch compress 触发
func (m *Timeline) compressForSizeLimit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.compressForSizeLimitLocked()
}

func (m *Timeline) compressForSizeLimitLocked() {
	if m.config == nil || m.totalDumpContentLimit <= 0 || m.compressing {
		return
	}

	activeIDs := m.getActiveTimelineItemIDs()
	total := len(activeIDs)
	if total <= 1 {
		return // 不能压缩到少于1个项目
	}

	// Caller already holds Timeline.mu. Do not call calculateActualContentSize(),
	// because it would try to RLock the same RWMutex and deadlock.
	currentSize := m.calculateActualContentSizeLocked()
	if currentSize <= m.totalDumpContentLimit {
		return
	}

	// 关键词: compressForSizeLimit, keepTokens, currentSize/6
	// 目标：保留最新约 1/6 token 的 item 不动，其余压缩
	keepTokens := currentSize / 6
	if keepTokens < 1 {
		keepTokens = 1
	}

	splitIdx := m.findCompressSplitByRecentKeepTokens(keepTokens)
	if splitIdx <= 0 {
		// 全部 item 累加 token 仍未达到 keepTokens，或活跃 item 太少；不压缩
		log.Infof("compress skipped: %d active items, keep all as recent (currentSize=%d, keepTokens=%d)",
			total, currentSize, keepTokens)
		return
	}

	// 防御性：splitIdx < 2 意味着只压缩了 1 条，价值很低，跳过本次（避免空炮）
	// 关键词: compressForSizeLimit, splitIdx 阈值, 避免无效压缩
	if splitIdx < 2 {
		log.Infof("compress skipped: only %d oldest item to compress (split=%d/%d), wait for more growth",
			splitIdx, splitIdx, total)
		return
	}

	// 按 id 升序收集 toCompress / recentKeep 切片
	var toCompress []*TimelineItem
	var recentKeep []*TimelineItem
	for i, id := range activeIDs {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil {
			continue
		}
		if i < splitIdx {
			toCompress = append(toCompress, item)
		} else {
			recentKeep = append(recentKeep, item)
		}
	}

	if len(toCompress) == 0 {
		return
	}
	// Reserve while holding Timeline.mu. Reduction itself must not advance
	// freeze/promotions until a complete summary is ready to commit.
	m.compressing = true

	log.Infof("content size %d > limit %d, compress oldest %d items, keep recent %d items (~%d tokens)",
		currentSize, m.totalDumpContentLimit, len(toCompress), len(recentKeep), keepTokens)

	go func() {
		defer func() {
			m.mu.Lock()
			m.compressing = false
			m.mu.Unlock()
			if err := recover(); err != nil {
				log.Errorf("batch compress panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		m.batchCompressOldestWithRecent(toCompress, recentKeep)
	}()
}

// batchCompressOldestWithRecent 把活跃区按 splitIdx 切出的"最旧 toCompress"压缩成 1 条 reducer，
// 同时把"最新 recentKeep"作为 prompt 中的 RECENT_KEEP 参考段一并喂给 AI（不修改、不删除），
// 让 AI 基于"现在 agent 在做什么"判断 toCompress 中哪些细节有价值需保留。
//
// 关键词: batchCompressOldestWithRecent, RECENT_KEEP context, batch compress 双段
func (m *Timeline) batchCompressOldestWithRecent(toCompress []*TimelineItem, recentKeep []*TimelineItem) {
	if len(toCompress) == 0 {
		return
	}

	// A missing scheduler cannot produce a faithful summary. The storage
	// emergency path is reserved for Save, not for a failed AI reduction.
	if m.config == nil {
		log.Warn("batch compress: Config is nil; preserving original timeline")
		return
	}

	m.mu.RLock()
	total := int64(len(m.getActiveTimelineItemIDs()))
	m.mu.RUnlock()
	if total <= 1 {
		return
	}

	// Capture coverage and the existing head before invoking callbacks. All
	// chunks must succeed before any item is retired or the head is replaced.
	m.mu.RLock()
	oldHead := cloneTimelineCompressedHead(m.compressedHead)
	var sources []timelineCompressionSource
	for _, item := range toCompress {
		if item != nil {
			sources = append(sources, timelineCompressionSource{id: item.GetID(), item: item, text: item.String()})
		}
	}
	limit := m.totalDumpContentLimit
	keepTokens := int64(0)
	for _, item := range recentKeep {
		if item != nil {
			keepTokens += m.estimateItemContentTokens(item.GetID(), item)
		}
	}
	promptItems := timelineCompressionPromptSnapshot(toCompress)
	promptRecent := timelineCompressionPromptSnapshot(recentKeep)
	m.mu.RUnlock()
	chunks, err := splitTimelineCompressionInput(promptItems, promptRecent)
	if err != nil {
		log.Warnf("batch compress: preserving timeline: %v", err)
		return
	}
	outputTokenBudget := limit - keepTokens - 50
	if outputTokenBudget < 200 {
		outputTokenBudget = 200
	}
	finalText := ""
	if oldHead != nil {
		finalText = strings.TrimSpace(oldHead.Text)
	}
	nonceStr := utils.RandStringBytes(4)
	for _, chunk := range chunks {
		memory := m.summarizeTimelineChunk(chunk, promptRecent, nonceStr, outputTokenBudget)
		if memory == "" {
			return // Failed/skipped/empty result: keep the entire original range.
		}
		if finalText != "" {
			finalText += "\n\n"
		}
		finalText += memory
	}
	if strings.TrimSpace(finalText) == "" {
		return
	}
	if TokenCountExceeds64(finalText, outputTokenBudget) {
		finalText = m.refineCompressedHeadLocked(finalText, outputTokenBudget, nonceStr)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Rollback, emergency reduction or a merge may have changed the head or
	// candidate range while the AI was running. Never commit a stale snapshot.
	if !sameTimelineCompressedHead(m.compressedHead, oldHead) {
		log.Warn("batch compress: head changed, discarding stale result")
		return
	}
	var idsToRemove []int64
	for _, source := range sources {
		current, ok := m.idToTimelineItem.Get(source.id)
		if !ok || current != source.item || current.deleted || current.GetID() != source.id || current.String() != source.text {
			log.Warn("batch compress: candidate range changed, discarding stale result")
			return
		}
		idsToRemove = append(idsToRemove, source.id)
	}
	if len(idsToRemove) == 0 {
		return
	}
	lastID := idsToRemove[len(idsToRemove)-1]
	lastTs, _ := m.idToTs.Get(lastID)
	// Publish exact promotions, frozen membership and the summary in one
	// transaction. Readers must not see an intermediate forced-freeze layout
	// immediately followed by another prefix rewrite when reduction finishes.
	m.freezeLocked(true, lastID)
	m.updateCompressedHead(&TimelineCompressedHead{
		Text: strings.TrimSpace(finalText), CoveredEndItemID: lastID, CoveredEndAtMs: lastTs,
	})
	for _, id := range idsToRemove {
		item, _ := m.idToTimelineItem.Get(id)
		item.deleted = true
	}
	log.Infof("batch compressed %d items into reducer at id: %v (%d complete input chunks)", len(idsToRemove), lastID, len(chunks))
}

// Opaque, detached reducer input: projection has already filtered bookkeeping.
// A concurrent history edit must not change chunk sizes/content mid-request.
type timelineCompressionPromptItem struct{ TextTimelineItem }

type timelineCompressionSource struct {
	id   int64
	item *TimelineItem
	text string
}

func timelineCompressionPromptSnapshot(items []*TimelineItem) []*TimelineItem {
	projected := projectTimelineItemsForPrompt(items)
	for i, item := range projected {
		projected[i] = &TimelineItem{createdAt: item.createdAt, value: &timelineCompressionPromptItem{
			TextTimelineItem{ID: item.GetID(), Text: item.String()},
		}}
	}
	return projected
}

func sameTimelineCompressedHead(a, b *TimelineCompressedHead) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// Split at complete item boundaries. A truncated prefix must never authorize
// deletion of unseen history. Overlarge single items remain active for now.
func splitTimelineCompressionInput(items, recent []*TimelineItem) ([][]*TimelineItem, error) {
	recentText, _, _ := buildRecentKeptString(recent, MaxBatchCompressRecentSize)
	budget := MaxBatchCompressPromptSize - len(recentText) - 1024
	var chunks [][]*TimelineItem
	for len(items) > 0 {
		_, count, _ := buildItemsToCompressString(items, budget)
		if count == 0 {
			return nil, utils.Errorf("one timeline item exceeds the reducer input budget (%d bytes)", budget)
		}
		chunks = append(chunks, items[:count])
		items = items[count:]
	}
	return chunks, nil
}

func (m *Timeline) summarizeTimelineChunk(items, recent []*TimelineItem, nonce string, outputBudget int64) string {
	var memory string
	var inputTokens int64
	for _, item := range items {
		inputTokens += m.estimateItemContentTokens(item.GetID(), item)
	}
	m.config.ScheduleAuxiliaryTask(m.config.GetContext(), CallerLabelTimelineBatchCompress,
		func() string {
			return m.renderBatchCompressPromptWithSchema(items, recent, nonce, inputTokens, outputBudget, "")
		},
		func(action *Action) {
			if action == nil {
				log.Warn("batch compress: nil AI result; preserving original timeline")
				return
			}
			memory = buildStructuredCompressedMemory(action)
			if memory == "" {
				memory = strings.TrimSpace(action.GetString("reducer_memory"))
			}
			if memory == "" {
				log.Warn("batch compress: empty summary, keeping original timeline")
				return
			}
			memory = enforceOutputTokenBudget(memory, outputBudget)
		},
		WithAuxiliaryOutputSchema("timeline-reducer", timelineReducerSchema),
		WithAuxiliaryOnError(func(err error) {
			log.Warnf("batch compress call ai failed: %v", err)
		}),
		WithAuxiliaryOpts(
			WithLiteForgeDisableTimeline(),
			WithGeneralConfigExtraRequestOpts(WithAIRequest_CallerLabel(CallerLabelTimelineBatchCompress)),
			WithGeneralConfigStreamableFieldResponseCallback(timelineReducerFields,
				func(key string, reader io.Reader, response *AIResponse, emitter *Emitter) {
					if emitter == nil {
						io.Copy(io.Discard, reader)
						return
					}
					emitter.EmitDefaultSystemStreamEvent("memory-timeline",
						utils.JSONStringReader(reader), response.GetTaskIndex(), func() {
							log.Infof("memory-timeline field [%s] streamed", key)
						})
				}),
		),
	)
	return memory
}

// MaxBatchCompressPromptSize is the maximum size (in bytes) for batch compress prompt
// This leaves room for the template overhead while keeping under the total token budget
const MaxBatchCompressPromptSize = 80 * 1024

// MaxBatchCompressRecentSize 是 batch compress prompt 中 RECENT_KEEP 段的字节预算上限
// 占总预算约 1/5，保证 ITEMS_TO_COMPRESS 仍是主体，且 RECENT_KEEP 提供足够的"现在"上下文
// 关键词: MaxBatchCompressRecentSize, recent keep prompt budget
const MaxBatchCompressRecentSize = 16 * 1024

var timelineBatchCompress = promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/batch_compress.txt")

var timelineReducerSchema = promptloader.MustLoad("ai/aid/aicommon/prompts/timeline/reducer.json")

var timelineReducerFields = []string{
	"key_findings", "active_config", "completed_work", "open_failures",
	"failed_and_resolved", "discarded", "user_directives",
}

// renderBatchCompressPrompt 渲染双段 batch compress prompt:
//
//	RECENT_KEEP   - 最新保留段，作为压缩参考"现在 agent 在做什么"，AI 不修改它
//	ITEMS_TO_COMPRESS - 待压缩的最旧段，AI 将其浓缩成 1 条 reducer
//
// 预算分配:
//
//	RECENT_KEEP   <= MaxBatchCompressRecentSize（先填，从最新向旧）
//	ITEMS_TO_COMPRESS <= MaxBatchCompressPromptSize - actualRecentSize（再填，按时间顺序最旧到次新）
//
// 关键词: renderBatchCompressPrompt, RECENT_KEEP, ITEMS_TO_COMPRESS, prompt 预算分配
func (m *Timeline) renderBatchCompressPrompt(toCompress []*TimelineItem, recentKeep []*TimelineItem, nonceStr string, inputTokenEstimate int64, outputTokenBudget int64) string {
	return m.renderBatchCompressPromptWithSchema(toCompress, recentKeep, nonceStr, inputTokenEstimate, outputTokenBudget, timelineReducerSchema)
}

// The LiteForge path supplies the schema separately; legacy prompt consumers
// can still render a self-contained prompt through renderBatchCompressPrompt.
func (m *Timeline) renderBatchCompressPromptWithSchema(toCompress []*TimelineItem, recentKeep []*TimelineItem, nonceStr string, inputTokenEstimate int64, outputTokenBudget int64, outputSchema string) string {
	if len(toCompress) == 0 {
		return ""
	}

	ins, err := template.New("timeline-batch-compress").Parse(timelineBatchCompress)
	if err != nil {
		log.Errorf("BUG: batch compress prompt template failed: %v", err)
		return ""
	}

	var buf bytes.Buffer
	var nonce = nonceStr
	if nonce == "" {
		nonce = utils.RandStringBytes(6)
	}

	// 1) 先构造 RECENT_KEEP 段（从最新向旧填，超限就在前面加 truncate notice）
	// 关键词: renderBatchCompressPrompt, RECENT_KEEP 截断, 从新向旧填充
	promptRecentKeep := projectTimelineItemsForPrompt(recentKeep)
	promptToCompress := projectTimelineItemsForPrompt(toCompress)
	recentStr, recentCount, recentTruncated := buildRecentKeptString(promptRecentKeep, MaxBatchCompressRecentSize)

	// 2) 剩余预算给 ITEMS_TO_COMPRESS（保留 1KB 给模板/指引/JSON schema）
	const templateOverheadReserve = 1024
	remainingBudget := MaxBatchCompressPromptSize - len(recentStr) - templateOverheadReserve
	if remainingBudget < 1024 {
		// 极端情况：recent 占满了，强行至少给 toCompress 留 1KB
		remainingBudget = 1024
	}

	itemsStr, actualItemCount, itemsTruncated := buildItemsToCompressString(promptToCompress, remainingBudget)

	if actualItemCount == 0 {
		if len(promptToCompress) == 0 {
			itemsStr = "[system bookkeeping omitted from prompt projection]"
			itemsTruncated = false
		} else {
			log.Warn("batch compress: no complete item fits in prompt budget; preserving original timeline")
			return ""
		}
	}

	if recentTruncated {
		log.Warnf("batch compress: RECENT_KEEP truncated to %d bytes (kept %d items)", len(recentStr), recentCount)
	}
	if itemsTruncated {
		log.Warnf("batch compress: ITEMS_TO_COMPRESS truncated (budget=%d)", remainingBudget)
	}

	err = ins.Execute(&buf, map[string]any{
		"ExtraMetaInfo":      m.ExtraMetaInfo(),
		"RecentKept":         recentStr,
		"RecentKeptCount":    recentCount,
		"HasRecentKept":      recentCount > 0,
		"ItemsToCompress":    itemsStr,
		"ItemCount":          actualItemCount,
		"InputTokenEstimate": inputTokenEstimate,
		"OutputTokenBudget":  outputTokenBudget,
		"OutputSchema":       outputSchema,
		"NONCE":              nonce,
	})
	if err != nil {
		log.Errorf("BUG: batch compress prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
}

func (m *Timeline) getActiveTimelineItemIDs() []int64 {
	if m == nil || m.idToTimelineItem == nil {
		return nil
	}
	ids := m.idToTimelineItem.Keys()
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted || isPromotableTimelineItem(item) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// buildRecentKeptString 从最新向旧填充 recentKeep 段，受 budget 字节上限约束
// 输出按时间顺序（最旧 → 最新）排版，前缀若有截断则加 truncate notice
// 关键词: buildRecentKeptString, recent keep 截断
func buildRecentKeptString(recentKeep []*TimelineItem, budget int) (string, int, bool) {
	if len(recentKeep) == 0 || budget <= 0 {
		return "", 0, false
	}

	// 从最新（末尾）向旧（开头）反向选取，保持总字节 <= budget
	type framed struct {
		idx  int
		text string
	}
	picked := make([]framed, 0, len(recentKeep))
	used := 0
	truncated := false
	for i := len(recentKeep) - 1; i >= 0; i-- {
		item := recentKeep[i]
		if item == nil {
			continue
		}
		// 与 ITEMS_TO_COMPRESS 同样格式: "[seq] <item.String()>"
		text := fmt.Sprintf("[%d] %s", i+1, item.String())
		// 含换行符
		need := used + len(text)
		if len(picked) > 0 {
			need++
		}
		if need > budget {
			truncated = true
			break
		}
		picked = append(picked, framed{idx: i, text: text})
		used = need
	}

	if len(picked) == 0 {
		return "", 0, len(recentKeep) > 0
	}

	// picked 当前是"最新→旧"，输出时反转为"旧→最新"
	for l, r := 0, len(picked)-1; l < r; l, r = l+1, r-1 {
		picked[l], picked[r] = picked[r], picked[l]
	}

	var buf strings.Builder
	if truncated {
		buf.WriteString(fmt.Sprintf("... [%d earlier recent items truncated due to size budget] ...\n", len(recentKeep)-len(picked)))
	}
	for i, f := range picked {
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(f.text)
	}
	return buf.String(), len(picked), truncated
}

// buildItemsToCompressString 按时间顺序（最旧 → 次新）填充 toCompress 段，受 budget 字节上限约束
// 关键词: buildItemsToCompressString, items to compress 截断
func buildItemsToCompressString(items []*TimelineItem, budget int) (string, int, bool) {
	if len(items) == 0 || budget <= 0 {
		return "", 0, false
	}
	var buf strings.Builder
	totalSize := 0
	actualItemCount := 0
	truncated := false
	for i, item := range items {
		if item == nil {
			continue
		}
		itemContent := fmt.Sprintf("[%d] %s", i+1, item.String())
		need := totalSize + len(itemContent)
		if i > 0 {
			need++
		}
		if need > budget {
			truncated = true
			truncateNotice := fmt.Sprintf("\n... [%d more items truncated due to size limit] ...", len(items)-i)
			if totalSize+len(truncateNotice) <= budget {
				buf.WriteString(truncateNotice)
			}
			break
		}
		if i > 0 {
			buf.WriteString("\n")
			totalSize++
		}
		buf.WriteString(itemContent)
		totalSize += len(itemContent)
		actualItemCount++
	}
	return buf.String(), actualItemCount, truncated
}

// buildStructuredCompressedMemory 从 AI action 中提取结构化字段，
// 拼成有固定段落标记的分段文本，存入 compressedHead.Text。
//
// 字段优先级: key_findings/open_failures > active_config > user_directives > completed_work > failed_and_resolved > discarded
// 关键词: buildStructuredCompressedMemory, 结构化压缩输出, 分段文本
func buildStructuredCompressedMemory(action *Action) string {
	if action == nil {
		return ""
	}

	var sections []struct {
		title string
		body  string
	}

	// key_findings (string array)
	findings := action.GetStringSlice("key_findings")
	if len(findings) > 0 {
		var buf strings.Builder
		for _, f := range findings {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			buf.WriteString("- ")
			buf.WriteString(f)
			buf.WriteString("\n")
		}
		if buf.Len() > 0 {
			sections = append(sections, struct {
				title string
				body  string
			}{"Key Findings", strings.TrimRight(buf.String(), "\n")})
		}
	}

	// active_config
	if s := strings.TrimSpace(action.GetString("active_config")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Active Config", s})
	}

	// user_directives
	if s := strings.TrimSpace(action.GetString("user_directives")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"User Directives", s})
	}

	// open_failures: unresolved execution failures, blockers and pending work are
	// control-critical state. They must never be folded into completed work.
	if s := strings.TrimSpace(action.GetString("open_failures")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Open Failures", s})
	}

	// completed_work
	if s := strings.TrimSpace(action.GetString("completed_work")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Completed Work", s})
	}

	// failed_and_resolved
	if s := strings.TrimSpace(action.GetString("failed_and_resolved")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Failed & Resolved", s})
	}

	// discarded
	if s := strings.TrimSpace(action.GetString("discarded")); s != "" {
		sections = append(sections, struct {
			title string
			body  string
		}{"Discarded", s})
	}

	if len(sections) == 0 {
		return ""
	}

	var buf strings.Builder
	for i, sec := range sections {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString("## ")
		buf.WriteString(sec.title)
		buf.WriteString("\n")
		buf.WriteString(sec.body)
	}
	return buf.String()
}

// enforceOutputTokenBudget 当 AI 输出超过 token 预算时，按优先级截断低价值字段。
// 截断顺序: discarded -> failed_and_resolved -> completed_work -> user_directives -> active_config.
// key_findings and open_failures are control-critical and are never selected
// for section deletion.
// 关键词: enforceOutputTokenBudget, post-check, 规则截断
func enforceOutputTokenBudget(text string, budget int64) string {
	if budget <= 0 {
		return text
	}
	current := int64(MeasureTokens(text))
	if current <= budget {
		return text
	}

	// 按 ## 标题分节
	sections := splitCompressedHeadSections(text)
	if len(sections) == 0 {
		// 无法分节，整体按 token 截断
		return ShrinkByTokens(text, int(budget))
	}

	// 低优先级字段按顺序截断/删除
	dropOrder := []string{"Discarded", "Failed & Resolved", "Completed Work", "User Directives", "Active Config"}
	for _, name := range dropOrder {
		if current <= budget {
			break
		}
		for i, sec := range sections {
			if sec.title == name {
				sectionTokens := int64(MeasureTokens(sec.body + "## " + sec.title + "\n"))
				// 先尝试截断到一半，如果还是太大就整个删除
				if sectionTokens > 50 {
					half := sectionTokens / 2
					sections[i].body = ShrinkByTokens(sec.body, int(half))
					current = int64(MeasureTokens(joinCompressedHeadSections(sections)))
				}
				if current > budget {
					sections[i].body = ""
					sections[i].title = "" // mark for removal
					current = int64(MeasureTokens(joinCompressedHeadSections(sections)))
				}
				break
			}
		}
	}

	return joinCompressedHeadSections(sections)
}

type compressedHeadSection struct {
	title string
	body  string
}

func splitCompressedHeadSections(text string) []compressedHeadSection {
	lines := strings.Split(text, "\n")
	var sections []compressedHeadSection
	var current *compressedHeadSection
	var bodyLines []string

	flush := func() {
		if current != nil {
			current.body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
			sections = append(sections, *current)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			flush()
			current = &compressedHeadSection{title: strings.TrimPrefix(trimmed, "## ")}
			bodyLines = nil
		} else if current != nil {
			bodyLines = append(bodyLines, line)
		}
	}
	flush()
	return sections
}

func joinCompressedHeadSections(sections []compressedHeadSection) string {
	var buf strings.Builder
	first := true
	for _, sec := range sections {
		if sec.title == "" && sec.body == "" {
			continue
		}
		if !first {
			buf.WriteString("\n\n")
		}
		first = false
		buf.WriteString("## ")
		buf.WriteString(sec.title)
		buf.WriteString("\n")
		buf.WriteString(sec.body)
	}
	return buf.String()
}

// refineCompressedHeadLocked 当 head 累积超过预算时，调用 AI 对 head 自身做精简。
// 只精简 completed_work/discarded/failed_and_resolved，保留
// key_findings/open_failures/active_config/user_directives。
// 关键词: refineCompressedHead, head-only 精简, head 累积控制
func (m *Timeline) refineCompressedHeadLocked(headText string, budget int64, nonceStr string) string {
	if headText == "" || budget <= 0 {
		return headText
	}

	// 如果 AI 不可用或精简 prompt 构建失败，用规则截断兜底
	if !TokenCountExceeds64(headText, budget) {
		return headText
	}
	if m.config == nil {
		return enforceOutputTokenBudget(headText, budget)
	}

	refined := enforceOutputTokenBudget(headText, budget)
	m.config.ScheduleAuxiliaryTask(m.config.GetContext(),
		CallerLabelTimelineHeadRefine,
		func() string { return renderRefineHeadPrompt(headText, budget, nonceStr, "") },
		func(action *Action) {
			if text := buildStructuredCompressedMemory(action); text != "" {
				refined = enforceOutputTokenBudget(text, budget)
			}
		},
		WithAuxiliaryOutputSchema("timeline-reducer", timelineReducerSchema),
		WithAuxiliaryOnError(func(err error) {
			log.Warnf("head refine AI call failed: %v, falling back to rule-based truncation", err)
		}),
		WithAuxiliaryOpts(
			WithLiteForgeDisableTimeline(),
			WithGeneralConfigExtraRequestOpts(WithAIRequest_CallerLabel(CallerLabelTimelineHeadRefine)),
			WithGeneralConfigStreamableFieldCallback(timelineReducerFields,
				func(_ string, reader io.Reader) { io.Copy(io.Discard, reader) }),
		),
	)
	return refined
}

// buildRefineHeadPrompt 构建 head-only 精简 prompt
func buildRefineHeadPrompt(headText string, budget int64, nonceStr string) string {
	return renderRefineHeadPrompt(headText, budget, nonceStr, timelineReducerSchema)
}

func renderRefineHeadPrompt(headText string, budget int64, nonceStr, outputSchema string) string {
	const refineTemplate = `# 角色与核心目标

你是一个 **AI 记忆精简模块**。当前的任务是对一段已压缩的历史摘要进行**精简**，使其 token 数不超过 {{ .OutputTokenBudget }}。

# 需要精简的压缩段
<|HEAD_TO_REFINE_{{ .NONCE }}|>
{{ .HeadText }}
<|HEAD_TO_REFINE_END_{{ .NONCE }}|>

## 精简规则

1. **key_findings、open_failures 和 active_config 不得丢失或删减**：这些是最高价值和控制关键状态，必须原样保留。
2. **user_directives 不得丢失**：用户指令必须原样保留。
3. **可以精简 completed_work**：合并相似条目，去除冗余描述，但保留工作阶段和关键结论。
4. **可以精简或删除 failed_and_resolved**：如果内容过长，保留最重要的转折性结论，删减细节。
5. **可以删除 discarded**：如果需要进一步缩减，优先删除此字段。
6. **不得将未解决失败移入 completed_work 或 failed_and_resolved**；只有后续独立成功证据才能改变其状态。
7. 保持原有的 7 字段结构化输出格式。

## 输出 token 预算
总输出 ≤ {{ .OutputTokenBudget }} tokens

# 输出格式：JSON

输出与原始压缩段相同的结构化 JSON 格式：

{{ .OutputSchema }}
`

	ins, err := template.New("timeline-head-refine").Parse(refineTemplate)
	if err != nil {
		log.Errorf("BUG: head refine prompt template failed: %v", err)
		return ""
	}

	nonce := nonceStr
	if nonce == "" {
		nonce = utils.RandStringBytes(6)
	}

	var buf bytes.Buffer
	err = ins.Execute(&buf, map[string]any{
		"HeadText":          headText,
		"OutputTokenBudget": budget,
		"OutputSchema":      outputSchema,
		"NONCE":             nonce,
	})
	if err != nil {
		log.Errorf("BUG: head refine prompt execution failed: %v", err)
		return ""
	}
	return buf.String()
}
