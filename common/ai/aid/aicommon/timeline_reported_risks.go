package aicommon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

const TimelinePromotedKindReportedRisk = "reported-risk"

const reportedRiskPromptTitle = "## 已上报风险（Reported Risks）"

func newReportedRiskTimelineItem(risk ReportedRiskItem) (*TimelineItem, error) {
	payload, err := json.Marshal(risk)
	if err != nil {
		return nil, err
	}
	op := &PromotableTimelineItem{
		Kind: TimelinePromotedKindReportedRisk, TargetSection: TimelinePromotedTargetSemiDynamic1,
		Operation: TimelinePromotedOperationUpsert,
		Payload:   string(payload), PayloadHash: promotedPayloadHash(string(payload)),
	}
	item := NewTimelineItem(op)
	// Business IDs deduplicate only this running session. Each actual report
	// has an immutable identity, including after restore without ledger replay.
	op.Key = item.historyID
	return item, nil
}

func (c *Config) appendReportedRiskToTimeline(risk *schema.Risk) bool {
	if c == nil || risk == nil {
		return false
	}
	summary := reportedRiskItemFromRisk(risk)
	state := c.GetSessionPromptState()
	state.m.Lock()
	store := state.getOrCreateReportedRiskStore()
	state.m.Unlock()

	// Reserve the deduplication key before broadcasting so callbacks may
	// re-enter AppendReportedRisk without deadlocking. Roll the reservation
	// back if item construction or delivery fails.
	store.mu.Lock()
	if store.hasRiskLocked(summary.ID) {
		store.mu.Unlock()
		return false
	}
	store.Items = append(store.Items, summary)
	store.mu.Unlock()

	item, err := newReportedRiskTimelineItem(summary)
	if err == nil {
		err = c.BroadcastTimelineItem(item)
	}
	if err != nil {
		store.mu.Lock()
		for i := range store.Items {
			if store.Items[i].ID == summary.ID {
				copy(store.Items[i:], store.Items[i+1:])
				store.Items = store.Items[:len(store.Items)-1]
				break
			}
		}
		store.mu.Unlock()
		log.Warnf("broadcast reported risk: %v", err)
		return false
	}
	return true
}

func timelineReportedRiskPrompt(op *PromotableTimelineItem) string {
	if op == nil || op.Kind != TimelinePromotedKindReportedRisk || op.Operation != TimelinePromotedOperationUpsert {
		return ""
	}
	var risk ReportedRiskItem
	if json.Unmarshal([]byte(op.Payload), &risk) != nil || risk.ID == "" {
		return ""
	}
	return fmt.Sprintf("%s\n\n%s\n\n此风险已上报到风险库，不要重复调用 `cybersecurity-risk`；补充证据请写入 evidence。",
		reportedRiskPromptTitle, formatReportedRiskLine(&risk))
}

// Frozen reports retain their original order and complete individual prompt
// bodies. New reports append; they never rewrite or evict earlier reports.
func renderPromotedReportedRisks(state *TimelinePromotedState) string {
	if state == nil {
		return ""
	}
	var entries []*PromotedTimelineEntry
	for _, entry := range state.Entries[TimelinePromotedTargetSemiDynamic1][TimelinePromotedKindReportedRisk] {
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].SourceItemID < entries[j].SourceItemID })
	var parts []string
	for _, entry := range entries {
		var risk ReportedRiskItem
		if json.Unmarshal([]byte(entry.Payload), &risk) != nil || risk.ID == "" {
			continue
		}
		parts = append(parts, "- "+formatReportedRiskLine(&risk))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("%s\n\n以下风险已上报到风险库，不要重复调用 `cybersecurity-risk`；补充证据请写入 evidence。\n\n%s",
		reportedRiskPromptTitle, strings.Join(parts, "\n\n"))
}
