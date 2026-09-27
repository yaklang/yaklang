package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func compressionTestOptions() TimelineCompressionOptions {
	return TimelineCompressionOptions{MaxInputTokens: 100000, MaxSummaryTokens: 4096,
		RetainedContext: map[string]string{"user_query": "修复配置合并，保持公开接口兼容", "todo": "验证嵌套配置"}}
}

func bindCompressionMock(t *testing.T, tl *Timeline, callback func(*AIRequest) (string, error)) context.CancelFunc {
	t.Helper()
	registerTimelineTestLiteForge(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cfg := NewTestConfig(ctx, WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true),
		WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
		WithSpeedPriorityAICallback(func(_ AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
			if request.GetCallerLabel() != CallerLabelTimelineCompress {
				return nil, fmt.Errorf("unexpected auxiliary call: %s", request.GetCallerLabel())
			}
			payload, err := callback(request)
			if err != nil {
				return nil, err
			}
			response := NewUnboundAIResponse()
			response.EmitOutputStream(strings.NewReader(payload))
			response.Close()
			return response, nil
		}))
	tl.SoftBindConfig(cfg, nil)
	return cancel
}

func compressionMockSummary(summary string) string {
	payload, _ := json.Marshal(map[string]string{"@action": "timeline-summary", "summary": summary})
	return string(payload)
}

// Old head + Frozen ordinary + Open ordinary are replaced together. Exact
// journals are promoted at the same commit, while writes during AI stay Open.
func TestTimelineCompressionTransactionAtomicPublish(t *testing.T) {
	tl := compressionSnapshotFixture()
	importFreezeItem(tl, 247, time.Unix(247, 0), &PromotableTimelineItem{ID: 247,
		Kind: TimelinePromotedKindEvidence, TargetSection: TimelinePromotedTargetSemiDynamic1,
		Key: "e2", Operation: TimelinePromotedOperationUpsert, Payload: `{"id":"e2","content":"PENDING_EVIDENCE"}`})
	before := RenderTimelineFrozenOpen(tl)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	calls := 0
	bindCompressionMock(t, tl, func(request *AIRequest) (string, error) {
		calls++
		close(entered)
		<-release
		return compressionMockSummary("NEW_SUMMARY: previous work verified, nested validation pending."), nil
	})
	type outcome struct {
		result *TimelineCompressionResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() { result, err := tl.CompressOnce(compressionTestOptions()); finished <- outcome{result, err} }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("mock was not reached")
	}
	// Check outside the AI callback: no lock held across the network operation.
	require.Equal(t, before, RenderTimelineFrozenOpen(tl))
	_, err := tl.CompressOnce(compressionTestOptions())
	require.ErrorContains(t, err, "already running")
	fork, err := tl.ForkForTask("child", "isolated", nil, nil)
	require.NoError(t, err)
	require.False(t, fork.Branch.compressing)
	require.Nil(t, fork.Branch.compressionSnapshot)
	// Both byte and time freeze triggers must defer while the transaction runs.
	tl.SetTimelineBucketByteSize(1)
	tl.PushText(300, "LATER_OPEN")
	require.True(t, tl.PushPromotable(310, TimelinePromotedKindRecentTool, TimelinePromotedTargetSemiDynamic1,
		"beta", TimelinePromotedOperationUpsert, "LATER_SCHEMA"))
	require.True(t, tl.PushPromotable(320, TimelinePromotedKindEvidence, TimelinePromotedTargetSemiDynamic1,
		"e3", TimelinePromotedOperationUpsert, `{"id":"e3","content":"LATER_EVIDENCE"}`))
	require.Empty(t, tl.FreezeAll().NewlyFrozenIDs)
	during := RenderTimelineFrozenOpen(tl)
	require.Equal(t, before.Frozen, during.Frozen)
	require.Equal(t, before.PromotedSemiDynamic1, during.PromotedSemiDynamic1)
	require.Equal(t, before.EvidenceSemiDynamic, during.EvidenceSemiDynamic)
	require.Contains(t, during.Open, "LATER_OPEN")
	unblock()
	// A reader observes the complete old state or the complete new state.
	for i := 0; i < 30; i++ {
		view := RenderTimelineFrozenOpen(tl)
		if strings.Contains(view.Frozen, "NEW_SUMMARY") {
			require.Contains(t, view.PromotedSemiDynamic1, "EXACT_SCHEMA")
			require.Contains(t, view.EvidenceSemiDynamic, "PENDING_EVIDENCE")
			require.NotContains(t, view.Open, "item-240")
		} else {
			require.Equal(t, during, view)
		}
	}
	got := <-finished
	require.NoError(t, got.err)
	require.Equal(t, 1, calls)
	require.EqualValues(t, 247, got.result.ThroughID)
	require.Len(t, got.result.RetiredIDs, 24)
	require.Equal(t, []int64{300}, tl.GetTimelineItemIDs())
	after := RenderTimelineFrozenOpen(tl)
	require.Contains(t, after.Frozen, "NEW_SUMMARY")
	require.NotContains(t, after.Frozen, "OLD_SUMMARY")
	require.NotContains(t, after.Frozen+after.Open, "item-240")
	require.Contains(t, after.Open, "LATER_OPEN")
	require.Contains(t, after.Open, "LATER_SCHEMA")
	require.Contains(t, after.Open, "LATER_EVIDENCE")
	require.NotContains(t, after.PromotedSemiDynamic1, "LATER_SCHEMA")
	require.NotContains(t, after.EvidenceSemiDynamic, "LATER_EVIDENCE")
	require.Contains(t, after.EvidenceSemiDynamic, "EXACT_EVIDENCE")
	require.Equal(t, before, RenderTimelineFrozenOpen(fork.Branch), "fork must not share commit state")
	require.False(t, tl.compressing)
	require.Nil(t, tl.compressionSnapshot)
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, after, RenderTimelineFrozenOpen(restored))
	require.EqualValues(t, 247, restored.frozenThroughLocked())
}

func TestTimelineCompressionTransactionRejectsStaleSource(t *testing.T) {
	for _, mutation := range []string{"text", "prompt", "shrink", "timestamp", "exact", "head", "delete", "rollback", "insert"} {
		t.Run(mutation, func(t *testing.T) {
			tl := compressionSnapshotFixture()
			var afterMutation string
			bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
				switch mutation {
				case "delete":
					tl.SoftDelete(240)
				case "rollback":
					tl.TruncateAfter(100)
				case "insert":
					importFreezeItem(tl, 239, time.Unix(239, 0), &TextTimelineItem{ID: 239, Text: "late import"})
				default:
					tl.mu.Lock()
					item, _ := tl.idToTimelineItem.Get(240)
					text := item.value.(*TextTimelineItem)
					switch mutation {
					case "text":
						text.Text = "edited source"
					case "prompt":
						text.PromptText = "edited prompt"
					case "shrink":
						text.ShrinkResult = "edited shrink"
					case "timestamp":
						tl.idToTs.Set(240, 999)
					case "exact":
						item, _ = tl.idToTimelineItem.Get(245)
						item.value.(*PromotableTimelineItem).Payload = "new schema"
					case "head":
						tl.compressedHead.Text = "newer summary"
					}
					tl.mu.Unlock()
				}
				afterMutation, _ = MarshalTimeline(tl)
				return compressionMockSummary("STALE_SUMMARY"), nil
			})
			result, err := tl.CompressOnce(compressionTestOptions())
			require.ErrorContains(t, err, "stale summary")
			require.Nil(t, result)
			after, err := MarshalTimeline(tl)
			require.NoError(t, err)
			require.Equal(t, afterMutation, after, "no partial freeze, promotion, retirement or head update")
			require.False(t, tl.compressing)
			require.Nil(t, tl.compressionSnapshot)
		})
	}
}

func TestTimelineCompressionTransactionFailurePreservesHistory(t *testing.T) {
	for _, mode := range []string{"empty", "invalid_json", "provider", "oversized_output", "oversized_input", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			tl := compressionSnapshotFixture()
			calls := 0
			var cancel context.CancelFunc
			cancel = bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
				calls++
				switch mode {
				case "empty":
					return compressionMockSummary(" "), nil
				case "invalid_json":
					return "not JSON", nil
				case "provider":
					return "", errors.New("mock unavailable")
				case "oversized_output":
					return compressionMockSummary(strings.Repeat("large summary ", 10000)), nil
				case "cancel":
					cancel()
				}
				return compressionMockSummary("rejected"), nil
			})
			before, err := MarshalTimeline(tl)
			require.NoError(t, err)
			options := compressionTestOptions()
			if mode == "oversized_input" {
				options.MaxInputTokens = 10
			}
			result, err := tl.CompressOnce(options)
			require.Error(t, err)
			require.Nil(t, result)
			if mode == "oversized_input" {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			after, err := MarshalTimeline(tl)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.False(t, tl.compressing)
			require.Nil(t, tl.compressionSnapshot)
		})
	}
}

func TestTimelineCompressionTransactionRepeatAndRestore(t *testing.T) {
	tl := compressionSnapshotFixture()
	// Restored indexes may hold distinct item pointers. A commit must retire
	// every stored view, otherwise old history can reappear on subsequent saves.
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	tl, err = UnmarshalTimeline(raw)
	require.NoError(t, err)
	calls := 0
	bindCompressionMock(t, tl, func(request *AIRequest) (string, error) {
		calls++
		if calls == 2 {
			require.Contains(t, request.GetPrompt(), "SUMMARY_1")
			require.Contains(t, request.GetPrompt(), "SECOND_SEGMENT")
			require.NotContains(t, request.GetPrompt(), "OLD_SUMMARY")
			require.NotContains(t, request.GetPrompt(), "item-240")
		}
		return compressionMockSummary(fmt.Sprintf("SUMMARY_%d", calls)), nil
	})
	_, err = tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	raw, err = MarshalTimeline(tl)
	require.NoError(t, err)
	require.NotContains(t, raw, "item-240", "retired original must not survive in the timestamp index")
	tl.PushText(300, "SECOND_SEGMENT")
	_, err = tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Empty(t, tl.GetTimelineItemIDs())
	require.Empty(t, RenderTimelineFrozenOpen(tl).Open)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "SUMMARY_2")
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Frozen, "SUMMARY_1")
	require.Len(t, tl.compressedHistory, 2)
}

func TestTimelineCompressionTransactionReplacesWholeReplay(t *testing.T) {
	tl := NewTimeline(nil, nil)
	replay := compressionSnapshotReplay(t, "inspect both targets")
	importFreezeItem(tl, 1, time.Unix(1, 0), &TextTimelineItem{ID: 1,
		Text: "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted", PromptText: replay})
	importFreezeItem(tl, 2, time.Unix(2, 0), &TextTimelineItem{ID: 2, Text: "call_a completed, call_b still pending"})
	bindCompressionMock(t, tl, func(request *AIRequest) (string, error) {
		require.Contains(t, request.GetPrompt(), "call_a")
		require.Contains(t, request.GetPrompt(), "call_b")
		return compressionMockSummary("call_a 已完成，call_b 仍待结果。"), nil
	})
	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, result.RetiredIDs)
	view := RenderTimelineFrozenOpenWithLatestModelReplay(tl)
	require.Contains(t, view.Frozen, result.Summary)
	require.Empty(t, view.Open)
	require.NotContains(t, view.Frozen, "FUNCTION_CALL_ACTION_RESPONSE")
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, view, RenderTimelineFrozenOpenWithLatestModelReplay(restored))
	require.EqualValues(t, 2, restored.GetMaxID())
}

func TestTimelineCompressionTransactionInvalidInput(t *testing.T) {
	_, err := (*Timeline)(nil).CompressOnce(compressionTestOptions())
	require.Error(t, err)
	tl := NewTimeline(nil, nil)
	_, err = tl.CompressOnce(compressionTestOptions())
	require.ErrorContains(t, err, "no visible ordinary history")
	require.False(t, tl.compressing)
	require.Nil(t, tl.compressionSnapshot)
	tl.PushText(1, "ordinary history")
	before, err := MarshalTimeline(tl)
	require.NoError(t, err)
	_, err = tl.CompressOnce(TimelineCompressionOptions{})
	require.Error(t, err)
	_, err = tl.CompressOnce(compressionTestOptions())
	require.ErrorContains(t, err, "scheduler")
	after, err := MarshalTimeline(tl)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.False(t, tl.compressing)
	require.Nil(t, tl.compressionSnapshot)
}

func TestTimelineCompressionInvalidSourceDoesNotHoldLock(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.SetTimelineContentLimit(1)
	tl.idToTimelineItem.Set(1, &TimelineItem{value: (*TextTimelineItem)(nil)})
	_, err := tl.CompressBeforePrompt(compressionTestOptions())
	require.Error(t, err)
	// Replacing the invalid entry must still acquire the lock and allow retry.
	tl.PushText(1, "repaired history")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) { return compressionMockSummary("repaired summary"), nil })
	tl.SetTimelineContentLimit(1)
	_, err = tl.CompressBeforePrompt(compressionTestOptions())
	require.NoError(t, err)
	require.Contains(t, RenderTimelineFrozenOpen(tl).Frozen, "repaired summary")
}

// Export a small complete BEFORE -> request -> mock answer -> AFTER example.
// This demonstrates state transitions, not the semantic quality of a live AI.
func TestTimelineCompressionTransactionReviewExample(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.compressedHead = &TimelineCompressedHead{Text: "已定位配置合并入口，尚未确认根因。", CoveredEndItemID: 1, Version: 1}
	for i, text := range []string{
		"已确认 merge.go 把显式空值当作缺失值，修改了内部判断；公开接口未改。",
		"最初测试因工作目录错误没有执行；修正目录后，顶层三种输入测试通过，日志见 artifacts/config-tests.txt。",
		"嵌套测试 call_nested_7 已受理，结果未到，不能声称验证完成。",
	} {
		id := int64(i + 2)
		importFreezeItem(tl, id, time.Unix(id, 0), &TextTimelineItem{ID: id, Text: text})
		if i == 0 {
			tl.FreezeAll()
		}
	}
	importFreezeItem(tl, 5, time.Unix(5, 0), freezeMutation(5, `{"type":"object","properties":{"path":{"type":"string"}}}`))
	importFreezeItem(tl, 6, time.Unix(6, 0), &PromotableTimelineItem{ID: 6, Kind: TimelinePromotedKindEvidence,
		TargetSection: TimelinePromotedTargetSemiDynamic1, Key: "config-tests", Operation: TimelinePromotedOperationUpsert,
		Payload: `{"id":"config-tests","content":"顶层空值、缺失值、非空值测试通过；日志 artifacts/config-tests.txt。"}`})
	before := RenderTimelineFrozenOpen(tl)
	var prompt string
	summary := "已修正 merge.go 将显式空值当作缺失值的判断，顶层三类输入测试通过，证据 config-tests，日志 artifacts/config-tests.txt。最初一次测试因工作目录错误未执行，后续须从仓库目录运行。嵌套测试 call_nested_7 仅已受理，结果未到；接下来核对其结果，暂不能认定验证完成。"
	bindCompressionMock(t, tl, func(req *AIRequest) (string, error) {
		prompt = req.GetPrompt()
		tl.PushText(7, "压缩期间追加：call_nested_7 返回，嵌套测试通过。")
		return compressionMockSummary(summary), nil
	})
	result, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	after := RenderTimelineFrozenOpen(tl)
	require.Contains(t, after.Frozen, summary)
	require.NotContains(t, after.Frozen, "嵌套测试通过")
	require.Contains(t, after.Open, "嵌套测试通过")
	require.Contains(t, after.EvidenceSemiDynamic, "顶层空值")
	t.Logf("ordinary source=%d tokens; mock summary=%d tokens; retired=%d; AI calls=1", result.InputTokens, result.SummaryTokens, len(result.RetiredIDs))
	if dir := os.Getenv("YAK_TIMELINE_COMPRESSION_EXAMPLES_DIR"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0755))
		render := func(v TimelineFrozenOpenBlocks) string {
			return "Frozen Timeline:\n" + v.Frozen + "\nSemi evidence:\n" + v.EvidenceSemiDynamic + "\nSemi toolcache:\n" + v.PromotedSemiDynamic1 + "\nOpen Timeline:\n" + v.Open
		}
		for name, body := range map[string]string{"transaction-before.txt": render(before), "transaction-request.txt": prompt, "transaction-mock-response.json": compressionMockSummary(summary), "transaction-after.txt": render(after)} {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0644))
		}
	}
}
