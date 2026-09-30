package aicommon

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/schema"
)

const TimelinePromotedKindUserInput = "user-input"

// Prompt-only wrapper bypasses whitespace cleanup and shrink results.
type timelineUserInputPromptItem struct{ TextTimelineItem }

// timelinePromotionForItem gives user input the same exact journal semantics as
// Evidence without replacing the typed audit records consumed by the UI.
func timelinePromotionForItem(item *TimelineItem) *PromotableTimelineItem {
	if item == nil || item.value == nil {
		return nil
	}
	if op, ok := item.value.(*PromotableTimelineItem); ok {
		return op
	}
	var payload string
	switch value := item.value.(type) {
	case *UserInteraction:
		timestamp := item.createdAt
		if !value.InputTimestamp.IsZero() {
			timestamp = value.InputTimestamp
		}
		stage := value.Stage
		if stage == "" {
			stage = UserInteractionStage_FreeInput
		}
		payload = fmt.Sprintf("- Time: %s | Stage: %s", timestamp.Format(time.RFC3339Nano), stage)
		if value.Round > 0 {
			payload += fmt.Sprintf(" | Round %d", value.Round)
		}
		if value.SystemPrompt != "" {
			payload += "\nSystem Question: " + value.SystemPrompt
		}
		payload += "\nUser Input: " + value.UserExtraPrompt
	case *TextTimelineItem:
		// These records are written by the loop at task start, including nested
		// tasks. Preserve the complete original text, task identity and whitespace.
		if extractTextEntryType(value.Text) != TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT {
			return nil
		}
		payload = value.Text
	default:
		return nil
	}
	return &PromotableTimelineItem{ID: item.GetID(), Kind: TimelinePromotedKindUserInput,
		TargetSection: TimelinePromotedTargetSemiDynamic1, Key: fmt.Sprintf("input-%d", item.GetID()),
		Operation: TimelinePromotedOperationUpsert, Payload: payload, PayloadHash: promotedPayloadHash(payload)}
}

func (m *Timeline) projectUserInputHistoryLocked() string {
	if m.promotedState == nil {
		return ""
	}
	entries := m.promotedState.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindUserInput]
	ordered := make([]*PromotedTimelineEntry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil {
			ordered = append(ordered, entry)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].SourceItemID < ordered[j].SourceItemID })
	if len(ordered) == 0 {
		return ""
	}
	var body strings.Builder
	body.WriteString("# Session User Input History\n以下是已封存的用户输入原文，按 Timeline 顺序保留，包含任务输入、追加输入及问答/审阅回复。结合开放时间线中的后续输入理解用户意图；当前执行请求见 USER_QUERY。\n")
	for _, entry := range ordered {
		body.WriteString(wrapUserInputWithKey(m.userInputBoundaryKey, entry.Payload))
		body.WriteString("\n\n")
	}
	nonce := userInputBoundaryNonce(m.userInputBoundaryKey, "user-input-promoted-history\x00"+body.String())
	return fmt.Sprintf("<|PREV_USER_INPUT_%s|>\n%s<|PREV_USER_INPUT_END_%s|>", nonce, body.String(), nonce)
}

func (m *Timeline) pushUserInputRecord(record schema.AIAgentUserInputRecord, id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pushUserInputRecordLocked(record, id)
}

func (m *Timeline) pushUserInputRecordLocked(record schema.AIAgentUserInputRecord, id int64) {
	now := time.Now()
	ts := now.UnixMilli()
	for m.tsToTimelineItem.Have(ts) {
		ts++
	}
	m.idToTs.Set(id, ts)
	m.pushTimelineItem(ts, id, &TimelineItem{createdAt: now, value: &UserInteraction{
		ID: id, Stage: UserInteractionStage_FreeInput, UserExtraPrompt: record.UserInput,
		Round: record.Round, InputTimestamp: record.Timestamp,
	}})
}

// Import legacy DB history once. Matching occurrences (rather than unique text)
// preserve repeated identical inputs and avoid duplicating existing audit events.
func (m *Timeline) importUserInputHistory(history []schema.AIAgentUserInputRecord, acquireID func() int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := make(map[string]int)
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		if input, ok := item.value.(*UserInteraction); ok && (input.Stage == UserInteractionStage_FreeInput || input.Stage == "") && input.SystemPrompt == "" {
			existing[input.UserExtraPrompt]++
		}
	}
	for _, record := range history {
		if existing[record.UserInput] > 0 {
			existing[record.UserInput]--
			continue
		}
		m.pushUserInputRecordLocked(record, acquireID())
	}
	// Old frozen interactions now also have an exact Semi1 projection.
	m.rebuildPromotedStateLocked(m.frozenThroughLocked())
}
