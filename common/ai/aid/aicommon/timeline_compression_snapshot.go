package aicommon

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
	"strings"
)

// timelineCompressionSnapshot is a detached plan, not a running compression.
// It contains no references to mutable TimelineItem values and never freezes,
// deletes, promotes or calls AI. Compression before prompt assembly reserves
// and commits this snapshot atomically.
type timelineCompressionSnapshot struct {
	FinalizingMemory        bool
	MemoryInputLimit        int
	MemoryOutputLimit       int
	Context                 context.Context `json:"-"`
	TargetTokens            int64           // desired main-model Timeline size, not helper-output size
	Head                    *TimelineCompressedHead
	ThroughID               int64 // captured watermark, including exact state and tombstones
	FrozenThroughID         int64
	Items                   []timelineCompressionSnapshotItem // ordinary items, retained by whitelist
	PromotedItems           []timelineCompressionPromotedItem // original promoted items, retired by blacklist
	ExactItemIDs            []int64                           // protected items: user input and unknown kinds
	ExactText               string                            // other protected item text, for threshold accounting
	InputText               string                            // whole visible timeline text
	InputTokens             int
	RetainedContext         map[string]string // caller-owned prompt parts, copied before the AI request
	SourceState             string            // live ordinary + exact journal entries through ThroughID
	FreezeVersion           int64
	Evidence                string // detached effective session evidence, including pending mutations
	UserContexts            []timelineCompressionUserContext
	SessionMemoryCandidates []any                           // completed session extractions, detached from listeners and storage
	MemoryCompletion        *timelineCompressionCompletion  `json:"-"`
	Output                  *timelineCompressionOutput      `json:"-"`
	SummaryPrompt           string                          `json:"-"` // actual rendered request source, runtime-only
	CommittedFreeze         TimelineFreezeResult            `json:"-"` // detached receipt, populated only after validation
	NotifyCommitted         func()                          `json:"-"` // observers captured at commit, called after unlock
	PromptBudget            timelineCompressionPromptBudget `json:"-"` // main-model occupancy captured with this snapshot
}

type timelineCompressionUserContext struct {
	ID     int64
	Frozen bool
	Text   string
}

type timelineCompressionSnapshotItem struct {
	ID         int64
	Timestamp  int64
	Frozen     bool
	PromptText string // detached, rendered prompt view; empty for omitted bookkeeping
}

// timelineCompressionPromotedItem is one original promoted journal item. The
// snapshot never aggregates a key or replays promotedState: the item is the
// unit the model selects and the commit retires.
type timelineCompressionPromotedItem struct {
	timelineCompressionSnapshotItem
	Kind      string
	Key       string
	Operation string
}

// timelineCompressionPromptBudget is the main-model Timeline occupancy, using
// the same projection that decides whether compression runs. Section token
// counts are for the helper prompt; they are not a second budget enforcer.
type timelineCompressionPromptBudget struct {
	Current  int64
	Target   int64
	Limit    int64
	Sections []timelineCompressionSectionUsage
}

type timelineCompressionSectionUsage struct {
	Name   string
	Tokens int64
}

// buildCompressionSnapshot includes the old summary and the original Frozen/Open
// items. There is no retained suffix or fixed compression ratio.
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
	parts := []string{previous, renderCompressionSnapshotItems(snapshot.Items),
		renderCompressionPromotedItems(snapshot.PromotedItems), snapshot.ExactText}
	for _, entry := range snapshot.UserContexts {
		parts = append(parts, entry.Text)
	}
	// Evidence items are original promoted items. Legacy memory retries may only
	// carry the detached reference view; count it once in that case.
	if !snapshot.hasEvidenceItems() {
		parts = append(parts, snapshot.Evidence)
	}
	snapshot.InputText = strings.TrimSpace(strings.Join(parts, "\n"))
	snapshot.InputTokens = MeasureTokens(snapshot.InputText)
	if !snapshot.hasCompressionInput() {
		return fmt.Errorf("compression snapshot has no visible history")
	}
	return nil
}

// hasSummarySource distinguishes actual history from a promoted-only decision,
// which may return an empty summary. User input is reference material, not an
// ordinary compression candidate, except during explicit memory finalization.
func (s *timelineCompressionSnapshot) hasSummarySource() bool {
	if s == nil {
		return false
	}
	if s.Head != nil && strings.TrimSpace(s.Head.Text) != "" {
		return true
	}
	return strings.TrimSpace(renderCompressionSnapshotItems(s.Items)) != "" ||
		(s.FinalizingMemory && (len(s.UserContexts) > 0 || strings.TrimSpace(s.Evidence) != ""))
}

func (s *timelineCompressionSnapshot) hasCompressionInput() bool {
	return s != nil && (s.hasSummarySource() || len(s.PromotedItems) > 0)
}

func (s *timelineCompressionSnapshot) hasPromotedItems() bool {
	return s != nil && len(s.PromotedItems) > 0
}

func (s *timelineCompressionSnapshot) hasEvidenceItems() bool {
	if s == nil {
		return false
	}
	for _, item := range s.PromotedItems {
		if item.Kind == TimelinePromotedKindEvidence {
			return true
		}
	}
	return false
}

func (s *timelineCompressionSnapshot) hasNewPromotedEntries() bool {
	if s == nil {
		return false
	}
	for _, item := range s.PromotedItems {
		if item.ID > s.FrozenThroughID {
			return true
		}
	}
	for _, id := range s.ExactItemIDs {
		if id > s.FrozenThroughID {
			return true
		}
	}
	return false
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
		SessionMemoryCandidates: m.sessionMemory.snapshot(),
	}
	if m.freezeState != nil {
		snapshot.FreezeVersion = m.freezeState.Version
	}
	snapshot.SourceState, err = m.compressionSourceStateLocked(snapshot.ThroughID)
	if err != nil {
		return nil, err
	}
	if evidence, found := m.evidenceStoreLocked(); found {
		snapshot.Evidence = renderEvidenceItems(evidence.Items)
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
		if isPromotableTimelineItem(item) || isTimelineUserInput(item) {
			control := timelinePromotionForItem(item)
			if control == nil {
				return nil, fmt.Errorf("invalid promoted compression source item %d", id)
			}
			switch {
			case control.Kind == TimelinePromotedKindUserInput:
				// User input is reference material: counted, never deletable.
				snapshot.ExactItemIDs = append(snapshot.ExactItemIDs, id)
				snapshot.UserContexts = append(snapshot.UserContexts, timelineCompressionUserContext{id, id <= snapshot.FrozenThroughID, control.Payload})
			case isCompressionPromotedItem(control):
				// One snapshot entry per original item: upsert/reuse/delete are
				// all listed with their own IDs and complete text.
				ts, ok := m.idToTs.Get(id)
				if !ok {
					return nil, fmt.Errorf("compression source item %d has no timestamp", id)
				}
				// PromptText is the original body only. RETIRE_ID frames are added
				// when the helper prompt is rendered, after this lock is released.
				snapshot.PromotedItems = append(snapshot.PromotedItems, timelineCompressionPromotedItem{
					timelineCompressionSnapshotItem: timelineCompressionSnapshotItem{
						ID: id, Timestamp: ts, Frozen: id <= snapshot.FrozenThroughID,
						PromptText: compressionPromotedItemText(control),
					},
					Kind: control.Kind, Key: compressionPromotedItemKey(control), Operation: control.Operation,
				})
			default:
				// Unknown kinds stay exact and only contribute to the threshold.
				snapshot.ExactItemIDs = append(snapshot.ExactItemIDs, id)
				snapshot.ExactText += "\n" + control.OpenPromptText()
			}
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
		var replayText string
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
			// Decode the validated group to historical data. Keep business call IDs,
			// arguments and receipts together, without reasoning or replay controls.
			payload := body[strings.Index(body, "|>")+2 : strings.LastIndex(body, "<|")]
			var messages []aispec.ChatDetail
			if err := json.Unmarshal([]byte(payload), &messages); err != nil {
				return nil, fmt.Errorf("decode compression replay at item %d: %w", id, err)
			}
			var plain strings.Builder
			for _, message := range messages {
				fmt.Fprintf(&plain, "%s tool_call_id=%s\n%v\n", message.Role, message.ToolCallID, message.Content)
				for _, call := range message.ToolCalls {
					fmt.Fprintf(&plain, "function=%s tool_call_id=%s\narguments:\n%s\n", call.Function.Name, call.ID, call.Function.Arguments)
				}
			}
			replayText = plain.String()
		}
		// SourceState already fingerprints every original field, including
		// shrink state, for the commit conflict check. Do not serialize a
		// second unused copy of each item while holding the read lock.
		entry := timelineCompressionSnapshotItem{
			ID: id, Timestamp: ts, Frozen: id <= snapshot.FrozenThroughID,
		}
		if projected := projectTimelineItemForPrompt(item); projected != nil &&
			normalizeTimelinePromptCategory(extractTextEntryType(projected.String())) != "ITERATION" {
			// Keep the entire prompt body, including long single-line replay JSON.
			// The presentation renderer uses a bounded line scanner and may omit
			// oversized lines; it must not determine compression cuts or budgets.
			// Do not substitute previous per-item shrink results for full originals.
			body := projected.value.String()
			if replayText != "" {
				body = replayText
			}
			entry.PromptText = fmt.Sprintf("ITEM_ID=%d timestamp_ms=%d [%s]\n%s",
				id, ts, renderItemTypeVerbose(projected), body)
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
