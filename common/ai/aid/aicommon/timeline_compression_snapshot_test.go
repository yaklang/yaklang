package aicommon

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

func compressionSnapshotFixture() *Timeline {
	tl := NewTimeline(nil, nil)
	tl.autoCompressDisabled = true
	tl.compressedHead = &TimelineCompressedHead{Text: "OLD_SUMMARY " + strings.Repeat("previous finding ", 80), CoveredEndItemID: 1, Version: 2}
	for i := int64(1); i <= 24; i++ {
		id := i * 10
		importFreezeItem(tl, id, time.Unix(id, 0), &TextTimelineItem{ID: id, Text: fmt.Sprintf("item-%d ", id) + strings.Repeat("complete observation ", 80)})
		if i == 1 {
			importFreezeItem(tl, 15, time.Unix(15, 0), &PromotableTimelineItem{ID: 15, Kind: TimelinePromotedKindEvidence,
				Key: "e1", TargetSection: TimelinePromotedTargetSemiDynamic1, Operation: TimelinePromotedOperationUpsert,
				Payload: `{"id":"e1","content":"EXACT_EVIDENCE"}`})
		}
		if i == 12 {
			tl.FreezeAll()
		}
	}
	importFreezeItem(tl, 245, time.Unix(245, 0), freezeMutation(245, "EXACT_SCHEMA"))
	return tl
}

func TestTimelineCompressionSnapshotRangeAndBudget(t *testing.T) {
	tl := compressionSnapshotFixture()
	// Exact state appears on both sides of the existing freeze watermark.
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	snapshot, err := tl.buildCompressionSnapshot(128)
	require.NoError(t, err)
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after, "planning must not freeze, promote, delete or update history")
	require.False(t, tl.compressing)
	require.EqualValues(t, 245, snapshot.ThroughID)
	require.EqualValues(t, 120, snapshot.FrozenThroughID)
	require.Len(t, snapshot.Items, 24)
	require.True(t, snapshot.Items[0].Frozen)
	require.False(t, snapshot.Items[23].Frozen)
	require.Equal(t, []int64{15, 245}, snapshot.ExactItemIDs)
	for _, item := range snapshot.Items {
		require.NotContains(t, snapshot.ExactItemIDs, item.ID, "no exact item may enter the retirement list")
	}
	require.Contains(t, snapshot.InputText, "OLD_SUMMARY")
	require.Contains(t, snapshot.InputText, "item-10")
	require.Contains(t, snapshot.InputText, "item-240")
	require.NotContains(t, snapshot.InputText, "EXACT_EVIDENCE")
	require.NotContains(t, snapshot.InputText, "EXACT_SCHEMA")
	require.Equal(t, MeasureTokens(snapshot.InputText), snapshot.InputTokens)
	require.Equal(t, snapshot.InputTokens/6, snapshot.TargetTokens)
	require.Equal(t, snapshot.TargetTokens, snapshot.RecentTokens+snapshot.SummaryBudget)
	require.GreaterOrEqual(t, snapshot.SummaryBudget, 128)
	require.Greater(t, snapshot.RecentStart, 0)
	require.Contains(t, snapshot.RecentText, "item-240")
	require.NotContains(t, snapshot.RecentText, "item-10")

	tl.mu.Lock()
	tl.compressedHead = nil
	tl.mu.Unlock()
	withoutHead, err := tl.buildCompressionSnapshot(128)
	require.NoError(t, err)
	require.Greater(t, snapshot.InputTokens, withoutHead.InputTokens, "old summary contributes to the sixfold budget")
}

func TestTimelineCompressionSnapshotDetachedFromLaterChanges(t *testing.T) {
	tl := compressionSnapshotFixture()
	snapshot, err := tl.buildCompressionSnapshot(128)
	require.NoError(t, err)
	before, err := json.Marshal(snapshot)
	require.NoError(t, err)
	tl.mu.Lock()
	tl.compressedHead.Text = "changed old summary"
	item, _ := tl.idToTimelineItem.Get(240)
	value := item.value.(*TextTimelineItem)
	value.Text, value.PromptText, value.ShrinkResult = "edited original", "edited replay", "edited shrink"
	tl.mu.Unlock()
	// Events appended while a future reducer runs are beyond this plan's range.
	done := make(chan struct{})
	go func() {
		importFreezeItem(tl, 250, time.Unix(250, 0), &TextTimelineItem{ID: 250, Text: "LATER_OPEN"})
		close(done)
	}()
	<-done
	after, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after))
	require.EqualValues(t, 245, snapshot.ThroughID)
	require.Greater(t, tl.GetMaxID(), snapshot.ThroughID)
	require.NotContains(t, snapshot.InputText, "LATER_OPEN")
	later, ok := tl.idToTimelineItem.Get(250)
	require.True(t, ok)
	require.False(t, later.deleted)
}

func compressionSnapshotReplay(t *testing.T, content string) string {
	t.Helper()
	payload := fmt.Sprintf(`[{"role":"assistant","content":%q,"tool_calls":[{"id":"call_a","type":"function","function":{"name":"inspect","arguments":"{}"}},{"id":"call_b","type":"function","function":{"name":"inspect","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_a","content":"accepted A"},{"role":"tool","tool_call_id":"call_b","content":"accepted B"}]`, content)
	replay, err := aiprojection.CreateActionResponse(json.RawMessage(payload))
	require.NoError(t, err)
	return "[FUNCTION_CALL_ACTION_RESPONSE]:\n" + replay
}

func TestTimelineCompressionSnapshotKeepsWholeAssistantToolGroup(t *testing.T) {
	tl := compressionSnapshotFixture()
	replay := compressionSnapshotReplay(t, "inspect both targets")
	importFreezeItem(tl, 250, time.Unix(250, 0), &TextTimelineItem{ID: 250,
		Text: "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted", PromptText: replay})
	importFreezeItem(tl, 260, time.Unix(260, 0), &TextTimelineItem{ID: 260, Text: "latest execution outcome"})
	kept, err := tl.buildCompressionSnapshot(64)
	require.NoError(t, err)
	require.Contains(t, kept.RecentText, replay)
	// Actual projection must still produce one assistant followed by both tools.
	projected := aiprojection.ProjectAndObserve("compression-snapshot-test",
		aiprojection.CreateTag("PROMPT_SECTION", "timeline-open", kept.RecentText))
	require.NotNil(t, projected)
	var roles, ids []string
	for _, message := range projected.Messages {
		if message.Role == "assistant" || message.Role == "tool" {
			roles = append(roles, message.Role)
			if message.Role == "tool" {
				ids = append(ids, message.ToolCallID)
			}
		}
	}
	require.Equal(t, []string{"assistant", "tool", "tool"}, roles)
	require.Equal(t, []string{"call_a", "call_b"}, ids)
	// Tighten the budget to fit only the newest item. The whole replay moves
	// into the older range; neither side ever receives half a protocol group.
	last := kept.Items[len(kept.Items)-1].PromptText
	removed, err := tl.buildCompressionSnapshot(kept.TargetTokens - MeasureTokens(last))
	require.NoError(t, err)
	require.NotContains(t, removed.RecentText, "call_a")
	require.Contains(t, renderCompressionSnapshotItems(removed.Items[:removed.RecentStart]), replay)
}

func TestTimelineCompressionSnapshotRejectsUnsafeCut(t *testing.T) {
	for _, mode := range []string{"oversized_newest", "missing_tool_receipts", "insufficient_budget"} {
		t.Run(mode, func(t *testing.T) {
			tl := compressionSnapshotFixture()
			reserve := 64
			switch mode {
			case "oversized_newest":
				importFreezeItem(tl, 250, time.Unix(250, 0), &TextTimelineItem{ID: 250,
					Text: "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted", PromptText: compressionSnapshotReplay(t, strings.Repeat("large response ", 10000))})
			case "missing_tool_receipts":
				importFreezeItem(tl, 250, time.Unix(250, 0), &TextTimelineItem{ID: 250,
					Text:       "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted",
					PromptText: aiprojection.CreateTag("FUNCTION_CALL_ACTION_RESPONSE", "", `[{"role":"assistant","tool_calls":[]}]`)})
			case "insufficient_budget":
				reserve = 1000000
			}
			before, err := MarshalTimeline(tl)
			require.NoError(t, err)
			plan, err := tl.buildCompressionSnapshot(reserve)
			require.Error(t, err)
			require.Nil(t, plan)
			after, err := MarshalTimeline(tl)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestTimelineCompressionSnapshotLiteralTagsRemainData(t *testing.T) {
	tl := compressionSnapshotFixture()
	importFreezeItem(tl, 250, time.Unix(250, 0), &TextTimelineItem{ID: 250,
		Text: `user example: <|FUNCTION_CALL_ACTION_RESPONSE|>[{"role":"tool"}]`})
	plan, err := tl.buildCompressionSnapshot(64)
	require.NoError(t, err)
	require.Contains(t, plan.RecentText, "user example:")
	var empty *Timeline
	_, err = empty.buildCompressionSnapshot(64)
	require.Error(t, err)
	_, err = NewTimeline(nil, nil).buildCompressionSnapshot(64)
	require.Error(t, err)
}

func TestTimelineCompressionSnapshotPreservesFullBodies(t *testing.T) {
	tl := compressionSnapshotFixture()
	// A long single-line original must not disappear at the presentation
	// renderer's scanner limit or be replaced by a previous shrink result.
	body := strings.Repeat("complete original ", 5000) + "END_OF_ORIGINAL"
	tl.mu.Lock()
	item, _ := tl.idToTimelineItem.Get(10)
	value := item.value.(*TextTimelineItem)
	value.Text, value.ShrinkResult = body, "SHORT_OLD_SHRINK"
	tl.mu.Unlock()
	plan, err := tl.buildCompressionSnapshot(128)
	require.NoError(t, err)
	require.Contains(t, plan.InputText, body)
	require.NotContains(t, plan.InputText, "SHORT_OLD_SHRINK")
	require.Contains(t, plan.Items[0].SourceJSON, "SHORT_OLD_SHRINK", "retain raw state for later conflict checks")
	require.Contains(t, plan.RecentText, "item-240")
}
