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

func isTimelineUserInput(item *TimelineItem) bool {
	if item == nil {
		return false
	}
	switch value := item.value.(type) {
	case *UserInteraction:
		return true
	case *TextTimelineItem:
		switch extractTextEntryType(value.Text) {
		case TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT, "user-clarification":
			return true // Include clarification records saved by older sessions.
		}
	case *PromotableTimelineItem:
		return value.Kind == TimelinePromotedKindUserInput
	}
	return false
}

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
		// Preserve task ingress and legacy clarification records verbatim.
		if !isTimelineUserInput(item) {
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
	body.WriteString("# Session User Input History\n\n")
	for _, entry := range ordered {
		body.WriteString(wrapUserInputWithKey(m.userInputBoundaryKey, entry.Payload))
		body.WriteString("\n\n")
	}
	return body.String()
}

// EnsureTaskUserInput records task ingress once, reusing a queued free-input
// record. Genuine repeated submissions are appended by AppendUserInputHistory.
func (m *Timeline) EnsureTaskUserInput(taskID, input string, acquireID func() int64) {
	if m == nil || strings.TrimSpace(input) == "" {
		return
	}
	m.mu.Lock()
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)
	ids := m.idToTimelineItem.Keys()
	for i := len(ids) - 1; i >= 0; i-- {
		item, ok := m.idToTimelineItem.Get(ids[i])
		if !ok || item == nil || item.deleted {
			continue
		}
		switch value := item.value.(type) {
		case *UserInteraction:
			if value.SystemPrompt == "" &&
				(value.Stage == "" || value.Stage == UserInteractionStage_FreeInput) && value.UserExtraPrompt == input {
				return
			}
		case *TextTimelineItem:
			if extractTextEntryType(value.Text) != TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT {
				continue
			}
			storedTaskID, _ := timelineItemTaskContext(item)
			if taskID != "" && storedTaskID != taskID {
				continue
			}
			// Read the exact body, without the legacy indentation cleanup.
			_, body, hasBody := strings.Cut(value.Text, ":\n")
			if hasBody && body == input {
				return
			}
		}
	}
	if acquireID != nil {
		id := acquireID()
		now := time.Now()
		ts := now.UnixMilli()
		for m.tsToTimelineItem.Have(ts) {
			ts++
		}
		header := fmt.Sprintf("[%s]", TIMELINE_ITEM_TYPE_CURRENT_TASK_USER_INPUT)
		if taskID != "" {
			header += fmt.Sprintf(" [task:%s]", taskID)
		}
		item := &TimelineItem{createdAt: now, value: &TextTimelineItem{ID: id, Text: header + ":\n" + input}}
		m.idToTs.Set(id, ts)
		m.pushTimelineItem(ts, id, item, &notifications)
	}
}

func (m *Timeline) pushUserInputRecord(record schema.AIAgentUserInputRecord, id int64) {
	m.mu.Lock()
	var notifications []func()
	defer m.unlockAndNotifyTimeline(&notifications)
	m.pushUserInputRecordLocked(record, id, &notifications)
}

func (m *Timeline) pushUserInputRecordLocked(record schema.AIAgentUserInputRecord, id int64, notifications *[]func()) {
	now := time.Now()
	ts := now.UnixMilli()
	for m.tsToTimelineItem.Have(ts) {
		ts++
	}
	m.idToTs.Set(id, ts)
	m.pushTimelineItem(ts, id, &TimelineItem{createdAt: now, value: &UserInteraction{
		ID: id, Stage: UserInteractionStage_FreeInput, UserExtraPrompt: record.UserInput,
		Round: record.Round, InputTimestamp: record.Timestamp,
	}}, notifications)
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
		m.pushUserInputRecordLocked(record, acquireID(), nil)
	}
	// Old frozen interactions now also have an exact Semi1 projection.
	m.rebuildPromotedStateLocked(m.frozenThroughLocked())
}
