package aicommon

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/schema"
)

func TestProjectionNonceUserVisibleLogs(t *testing.T) {
	var events []*schema.AiOutputEvent
	emitter := NewEmitter("nonce-log", func(event *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		events = append(events, event)
		return event, nil
	})
	prompt := aiprojection.CreateTag("PROMPT_SECTION", "dynamic", "ordinary payload")
	for _, level := range []string{"info", "warning", "error"} {
		_, err := emitter.EmitLogWithLevel(level, "", "transaction prompt: %s", prompt)
		require.NoError(t, err)
	}
	require.Len(t, events, 3)
	for _, event := range events {
		require.NotContains(t, string(event.Content), aiprojection.Nonce())
		require.Contains(t, string(event.Content), "REDACTED_PROJECTION_NONCE")
		require.Contains(t, string(event.Content), "ordinary payload")
	}
}

func TestTimelineProjectionNonceRestoreAndFork(t *testing.T) {
	oldNonce := strings.Repeat("a", 64)
	// The old value also occurs in genuine payload and display text. Restore
	// must change only the two envelope tokens, never these application values.
	payload := `[{"role":"assistant","content":"` + oldNonce + `","tool_calls":[{"id":"call_restore","type":"function","function":{"name":"inspect","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_restore","content":"` + oldNonce + `"}]`
	replay, err := aiprojection.CreateActionResponse(json.RawMessage(payload))
	require.NoError(t, err)
	replay = "[FUNCTION_CALL_ACTION_RESPONSE]:\n" + replay
	tl := NewTimeline(nil, nil)
	display := "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted " + oldNonce
	tl.PushTextWithPromptProjection(1, display, replay)
	tl.PushText(2, "[note]:\n<|FUNCTION_CALL_ACTION_RESPONSE_"+oldNonce+"|>ordinary example")
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	var saved timelineSerializable
	require.NoError(t, json.Unmarshal([]byte(raw), &saved))
	require.Equal(t, aiprojection.Nonce(), saved.ProjectionNonce)
	for _, index := range []map[string]*TimelineItem{saved.IdToTimelineItem, saved.TsToTimelineItem} {
		for _, item := range index {
			if text, ok := timelineTextItem(item); ok {
				text.PromptText = strings.ReplaceAll(text.PromptText, "_"+aiprojection.Nonce()+"|>", "_"+oldNonce+"|>")
			}
		}
	}
	saved.ProjectionNonce = oldNonce
	oldJSON, err := json.Marshal(saved)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(string(oldJSON))
	require.NoError(t, err)
	item, ok := restored.idToTimelineItem.Get(1)
	require.True(t, ok)
	text, ok := timelineTextItem(item)
	require.True(t, ok)
	require.Equal(t, display, text.Text)
	require.Equal(t, replay, text.PromptText)
	restored.tsToTimelineItem.ForEach(func(_ int64, item *TimelineItem) bool {
		if item.GetID() == 1 {
			require.Equal(t, replay, item.GetValue().(*TextTimelineItem).PromptText)
		}
		return true
	})
	require.Equal(t, tl.Dump(), restored.Dump())
	require.NotContains(t, restored.Dump(), aiprojection.Nonce())

	branch, err := restored.ForkForTask("nonce-test", "fork", nil, nil)
	require.NoError(t, err)
	branchItem, ok := branch.Branch.idToTimelineItem.Get(1)
	require.True(t, ok)
	require.Equal(t, replay, branchItem.GetValue().(*TextTimelineItem).PromptText)

	// Render the restored timeline and pass through the real send-time hook.
	blocks := projectTimelineRenderableBlocksForPromptWithLatestModelReplay(restored.GroupByMinutes(3).GetAllRenderable())
	prompt := BuildTaggedPromptSections("system", "frozen", "skills", "instructions", blocks.Render("TIMELINE"), "continue", "turn")
	projected := aiprojection.ProjectAndObserve("restore-test", prompt)
	require.True(t, projected.IsHijacked)
	var calls, results int
	for _, message := range projected.Messages {
		if message.Role == "assistant" {
			calls++
			require.Equal(t, oldNonce, message.Content)
		}
		if message.Role == "tool" {
			results++
			require.Equal(t, "call_restore", message.ToolCallID)
			require.Equal(t, oldNonce, message.Content)
		}
	}
	require.Equal(t, 1, calls)
	require.Equal(t, 1, results)
	encoded, err := json.Marshal(projected)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), aiprojection.Nonce())
}

func TestTimelineProjectionNonceLegacyAndInvalidSnapshots(t *testing.T) {
	const valid = `<|FUNCTION_CALL_ACTION_RESPONSE|>
[{"role":"assistant","content":"","tool_calls":[{"id":"call_legacy","type":"function","function":{"name":"inspect","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call_legacy","content":"accepted"}]
<|FUNCTION_CALL_ACTION_RESPONSE_END|>`
	for _, tc := range []struct {
		name, prompt string
		valid        bool
	}{
		{"legacy", valid, true},
		{"malformed", strings.Replace(valid, `"tool_call_id":"call_legacy"`, `"tool_call_id":"wrong"`, 1), false},
		{"arbitrary-data", "source example: " + valid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			tl.PushTextWithPromptProjection(1, "[FUNCTION_CALL_ACTION_RESPONSE]:\ndisplay", tc.prompt)
			raw, err := MarshalTimeline(tl)
			require.NoError(t, err)
			var saved map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &saved))
			delete(saved, "projection_nonce")
			encoded, err := json.Marshal(saved)
			require.NoError(t, err)
			restored, err := UnmarshalTimeline(string(encoded))
			require.NoError(t, err)
			item, ok := restored.idToTimelineItem.Get(1)
			require.True(t, ok)
			text := item.GetValue().(*TextTimelineItem)
			require.Equal(t, "[FUNCTION_CALL_ACTION_RESPONSE]:\ndisplay", text.Text)
			if tc.valid {
				require.Contains(t, text.PromptText, aiprojection.Nonce())
			} else {
				require.Empty(t, text.PromptText)
			}
		})
	}
}
