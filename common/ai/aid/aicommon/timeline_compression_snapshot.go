package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strings"
)

// timelineCompressionSnapshot is a detached plan, not a running compression.
// It contains no references to mutable TimelineItem values and never freezes,
// deletes, promotes or calls AI. Compression before prompt assembly reserves
// and commits this snapshot atomically.
type timelineCompressionSnapshot struct {
	Context                 context.Context `json:"-"`
	BeforePromptFingerprint string          `json:"-"`
	BeforePromptLimit       int64           `json:"-"`
	Head                    *TimelineCompressedHead
	ThroughID               int64 // captured watermark, including exact state and tombstones
	FrozenThroughID         int64
	Items                   []timelineCompressionSnapshotItem // all live ordinary items, in ID order
	ExactItemIDs            []int64                           // never candidates for summary/deletion
	InputText               string                            // old head + ordinary Frozen/Open content
	InputTokens             int
	RetainedContext         map[string]string // caller-owned prompt parts, copied before the AI request
	SourceState             string            // live ordinary + exact journal entries through ThroughID
	FreezeVersion           int64
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

func (m *Timeline) captureCompressionSnapshotLocked() (snapshot *timelineCompressionSnapshot, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			snapshot, err = nil, fmt.Errorf("capture timeline compression source: %v", recovered)
		}
	}()
	snapshot = &timelineCompressionSnapshot{
		Head:      cloneTimelineCompressedHead(m.compressedHead),
		ThroughID: m.getMaxIDLocked(), FrozenThroughID: m.frozenThroughLocked(),
	}
	if m.freezeState != nil {
		snapshot.FreezeVersion = m.freezeState.Version
	}
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
		if utils.IsNil(item.value) || item.GetID() != id {
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
