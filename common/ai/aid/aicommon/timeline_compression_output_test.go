package aicommon

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func compressionMemoryFixture() map[string]any {
	return map[string]any{
		"content":             "用户明确要求工程报告使用中文，附实际验证结果。",
		"potential_questions": []any{"工程报告应该如何撰写？"}, "tags": []any{"工程", "偏好"},
		"scores": map[string]any{"temporality": 0.9, "actionability": 0.8, "perference": 1.0,
			"origin": 1.0, "emotion": 0.1, "relevance": 0.8, "connectivity": 0.5},
	}
}

func compressionOutputFixture(retained string, entities []any) map[string]any {
	return map[string]any{"@action": "timeline-summary", "summary": "历史验证通过，继续处理后续结果。",
		"ratain_timeline_item_range": retained, "memory_entities": entities}
}

func TestTimelineCompressionPlainSource(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "[MODEL_THINKING]:\nPRIVATE_THOUGHT")
	tl.PushText(2, "[ITERATION]:\nITERATION_NOISE")
	tl.PushText(3, "[review]:\n已验证空值处理；嵌套验证未完成。")
	tl.FreezeAll()
	tl.PushText(4, "待核对最新工具结果。")
	snapshot, err := tl.buildCompressionSnapshot()
	require.NoError(t, err)
	query := "检查 C:\\repo\\merge.go\n示例：{\"value\":\"\"}"
	snapshot.RetainedContext = map[string]string{"user_query": query, "todo": "核对嵌套路径", "z": "last", "a": "first"}
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	require.Contains(t, prompt, compressionSourceTag("ORIGINAL_USER_QUERY", query))
	require.NotContains(t, prompt, `C:\\repo`)
	require.NotContains(t, prompt, `\"value\"`)
	require.NotContains(t, prompt, "PRIVATE_THOUGHT")
	require.NotContains(t, prompt, "ITERATION_NOISE")
	require.Contains(t, prompt, compressionSourceTag("FROZEN_TIMELINE", snapshot.Items[2].PromptText))
	require.Contains(t, prompt, compressionSourceTag("OPEN_TIMELINE", snapshot.Items[3].PromptText))
	require.Less(t, strings.Index(prompt, "a:\nfirst"), strings.Index(prompt, "z:\nlast"))
	again, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	require.Equal(t, prompt, again)
}

func TestTimelineCompressionRetainedRange(t *testing.T) {
	items := []timelineCompressionSnapshotItem{{ID: 1, PromptText: "first"}, {ID: 3}, {ID: 5, PromptText: "last"}, {ID: 9, PromptText: "ninth"}}
	ids, err := parseCompressionRetainedRange(" 1-5,9,5 ", items)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 5, 9}, ids)
	for _, invalid := range []string{"0", "3", "2", "-1", "5-1", "1-9223372036854775807", "1-3", "1--5", "1,", "x", "1-5-9", "U:1", "1-", "+1"} {
		_, err := parseCompressionRetainedRange(invalid, items)
		require.Error(t, err, invalid)
	}
	_, err = parseCompressionRetainedRange("1-2000000000", []timelineCompressionSnapshotItem{
		{ID: 1, PromptText: "first"}, {ID: 2000000000, PromptText: "last"},
	})
	require.Error(t, err)

}

func TestTimelineCompressionRetainsOriginalsAcrossRounds(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "已完成顶层验证。")
	original := "关键原文：C:\\repo\\merge.go\n嵌套验证 call_nested 尚未返回。"
	tl.PushText(2, original)
	calls := 0
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		calls++
		retained := "2"
		if calls == 2 {
			require.Contains(t, req.GetPrompt(), original)
			require.Contains(t, req.GetPrompt(), "历史验证通过")
			require.Contains(t, req.GetPrompt(), "嵌套验证结果已通过")
			retained = ""
		}
		raw, err := json.Marshal(compressionOutputFixture(retained, []any{}))
		return string(raw), err
	})
	tl.SetTimelineContentLimit(1)
	first, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, []int64{2}, first.RetainedIDs)
	require.Equal(t, []int64{1}, first.RetiredIDs)
	require.EqualValues(t, 2, tl.compressedHead.CoveredEndItemID)
	view := RenderTimelineFrozenOpen(tl)
	require.Contains(t, view.Frozen, original)
	require.Contains(t, view.Frozen, first.Summary)
	require.Empty(t, view.Open)
	kept, _ := tl.idToTimelineItem.Get(2)
	require.Equal(t, original, kept.value.(*TextTimelineItem).Text)
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpen(restored))
	for i := 0; i < 3; i++ {
		next, err := tl.CompressBeforePrompt(compressionTestOptions())
		require.NoError(t, err)
		require.Nil(t, next, "retained originals alone must not repeat compression")
	}
	require.Equal(t, 1, calls)
	tl.PushText(3, "嵌套验证结果已通过。")
	second, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, second.RetiredIDs)
	require.Empty(t, second.RetainedIDs)
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Frozen, original)
	require.Equal(t, 2, calls)
}

func TestTimelineCompressionMemoryCommittedObservers(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "已确认用户报告偏好。")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		raw, err := json.Marshal(compressionOutputFixture("1", []any{compressionMemoryFixture()}))
		return string(raw), err
	})
	called := 0
	tl.RegisterSummaryCallback("consumer", func(event TimelineSummaryEvent) {
		called++
		require.EqualValues(t, 1, tl.FreezeSnapshot().Version)
		body, err := io.ReadAll(event.Summary)
		require.NoError(t, err)
		require.Contains(t, string(body), "历史验证通过")
		require.Equal(t, []int64{1}, event.RetainedIDs)
		event.RetainedIDs[0] = -1
	})
	tl.RegisterMemoryCallback("mutation", func(event TimelineMemoryEvent) {
		event.MemoryEntities[0].(map[string]any)["scores"].(map[string]any)["origin"] = 0.0
	})
	memory := make(chan TimelineMemoryEvent, 1)
	tl.RegisterMemoryCallback("consumer", func(event TimelineMemoryEvent) { memory <- event })
	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, called)
	require.Equal(t, []int64{1}, result.RetainedIDs)
	select {
	case event := <-memory:
		require.NoError(t, event.Err)
		require.Equal(t, compressionMemoryFixture(), event.MemoryEntities[0])
	case <-time.After(5 * time.Second):
		t.Fatal("memory notification not delivered")
	}
}

func TestTimelineCompressionOptionalOutputCompatibility(t *testing.T) {
	for _, mode := range []string{"missing_range", "wrong_range", "missing_memory", "null_memory", "invalid_id", "missing_score", "bad_score", "too_many_tags", "no_content", "no_questions", "retained_budget", "memory_budget"} {
		t.Run(mode, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			tl.PushText(1, strings.Repeat("complete original finding ", 100))
			entity := compressionMemoryFixture()
			output := compressionOutputFixture("", []any{entity})
			switch mode {
			case "missing_range":
				delete(output, "ratain_timeline_item_range")
			case "wrong_range":
				output["ratain_timeline_item_range"] = []any{1}
			case "missing_memory":
				delete(output, "memory_entities")
			case "null_memory":
				output["memory_entities"] = nil
			case "invalid_id":
				output["ratain_timeline_item_range"] = "999"
			case "missing_score":
				delete(entity["scores"].(map[string]any), "origin")
			case "bad_score":
				entity["scores"].(map[string]any)["origin"] = 1.1
			case "too_many_tags":
				entity["tags"] = []any{"a", "b", "c", "d", "e", "f"}
			case "no_content":
				entity["content"] = " "
			case "no_questions":
				delete(entity, "potential_questions")
			case "retained_budget":
				output["ratain_timeline_item_range"] = "1"
			case "memory_budget":
				entity["content"] = strings.Repeat("oversized durable memory ", 100)
			}
			bindCompressionMock(t, tl, func(*AIRequest) (string, error) { raw, err := json.Marshal(output); return string(raw), err })
			before, err := MarshalTimeline(tl)
			require.NoError(t, err)
			called := 0
			tl.RegisterSummaryCallback("consumer", func(TimelineSummaryEvent) { called++ })
			memory := make(chan TimelineMemoryEvent, 1)
			tl.RegisterMemoryCallback("consumer", func(event TimelineMemoryEvent) { memory <- event })
			options := compressionTestOptions()
			if strings.HasSuffix(mode, "budget") {
				options.MaxSummaryTokens = 150
			}
			result, err := tl.CompressOnce(options)
			if mode == "retained_budget" {
				require.Error(t, err)
				require.Zero(t, called)
				after, err := MarshalTimeline(tl)
				require.NoError(t, err)
				require.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, called)
			require.NotEmpty(t, result.Summary)
			require.Empty(t, result.RetainedIDs)
			select {
			case event := <-memory:
				if mode == "memory_budget" {
					require.Error(t, event.Err)
				} else {
					require.NoError(t, event.Err)
				}
				if mode == "missing_range" || mode == "wrong_range" || mode == "invalid_id" {
					require.Len(t, event.MemoryEntities, 1)
				} else {
					require.Empty(t, event.MemoryEntities)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("missing memory notification")
			}
		})
	}
}

func TestTimelineCompressionSummaryOnlyCompatibility(t *testing.T) {
	for _, raw := range []string{
		`{"summary":"已确认顶层验证通过，嵌套结果待核对。"}`,
		"结果如下：\n```json\n" + `{"summary":"已确认顶层验证通过，嵌套结果待核对。"}` + "\n```\n以上为历史摘要。",
		`{"summary":"已确认顶层验证通过，嵌套结果待核对。","ratain_timeline_item_range":null,"memory_entities":"unavailable","extension":true}`,
		`{"@action":"timeline-summary","summary":"已确认顶层验证通过，嵌套结果待核对。"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			tl.PushText(1, "old ordinary history")
			bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
				require.Contains(t, req.GetPrompt(), `"memory_entities"`)
				require.Contains(t, req.GetPrompt(), `"ratain_timeline_item_range"`)
				return raw, nil
			})
			called := 0
			tl.RegisterSummaryCallback("consumer", func(event TimelineSummaryEvent) { called++ })
			result, err := tl.CompressOnce(compressionTestOptions())
			require.NoError(t, err)
			require.Contains(t, result.Summary, "顶层验证通过")
			require.Empty(t, result.RetainedIDs)
			require.Equal(t, 1, called)
		})
	}
}

func TestTimelineCompressionMemoryCandidatesValidatedIndependently(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "history")
	invalid := compressionMemoryFixture()
	invalid["scores"].(map[string]any)["origin"] = 2
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		raw, err := json.Marshal(compressionOutputFixture("1", []any{invalid, "bad entry", compressionMemoryFixture()}))
		return string(raw), err
	})
	memory := make(chan TimelineMemoryEvent, 1)
	tl.RegisterMemoryCallback("consumer", func(event TimelineMemoryEvent) { memory <- event })
	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, []int64{1}, result.RetainedIDs)
	select {
	case event := <-memory:
		require.NoError(t, event.Err)
		require.Len(t, event.MemoryEntities, 1)
		require.Equal(t, compressionMemoryFixture(), event.MemoryEntities[0])
	case <-time.After(5 * time.Second):
		t.Fatal("missing memory notification")
	}
}

func TestTimelineCompressionRetainedNativeReplay(t *testing.T) {
	tl := NewTimeline(nil, nil)
	replay := compressionSnapshotReplay(t, "inspect targets")
	importFreezeItem(tl, 1, time.Now(), &TextTimelineItem{ID: 1, Text: "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted", PromptText: replay})
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		raw, err := json.Marshal(compressionOutputFixture("1", []any{}))
		return string(raw), err
	})
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	item, _ := tl.idToTimelineItem.Get(1)
	require.Equal(t, replay, item.value.(*TextTimelineItem).PromptText)
	snapshot, err := tl.buildCompressionSnapshot()
	require.NoError(t, err)
	require.Contains(t, snapshot.InputText, "call_a")
	require.Contains(t, snapshot.InputText, "call_b")
}

func TestTimelineCompressionRetainedRestoreRemapsCoverage(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(9000, "kept original")
	tl.PushText(9100, "observation summarized")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		raw, err := json.Marshal(compressionOutputFixture("9000", []any{}))
		return string(raw), err
	})
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	var id int64
	restored.ReassignIDs(func() int64 { id++; return id })
	require.EqualValues(t, 2, restored.compressedHead.CoveredEndItemID)
	require.EqualValues(t, 2, restored.GetMaxID())
	restored.PushText(3, "new open finding")
	view := RenderTimelineFrozenOpen(restored)
	require.Contains(t, view.Frozen, "kept original")
	require.Contains(t, view.Frozen, "历史验证通过")
	require.NotContains(t, view.Frozen, "new open finding")
	require.Contains(t, view.Open, "new open finding")
}
