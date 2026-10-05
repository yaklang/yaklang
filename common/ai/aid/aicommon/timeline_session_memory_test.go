package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimelineSessionMemoryAcrossCompressionsWithoutObservers(t *testing.T) {
	tl := NewTimeline(nil, nil)
	candidate := compressionMemoryFixture()
	var prompts []string
	bindCompressionMock(t, tl, func(request *AIRequest) (string, error) {
		prompts = append(prompts, request.GetPrompt())
		raw, err := json.Marshal(compressionOutputFixture("", []any{candidate}))
		return string(raw), err
	})
	for id := int64(1); id <= 2; id++ {
		tl.PushText(id, "verified historical observation")
		_, err := tl.CompressOnce(compressionTestOptions())
		require.NoError(t, err)
	}
	require.Len(t, prompts, 2)
	require.Contains(t, prompts[1], "<|SESSION_MEMORY_CANDIDATES_")
	require.Contains(t, prompts[1], candidate["content"])
	require.Contains(t, prompts[1], candidate["title"])
	for _, field := range []string{"tags：", "potential_questions：", "scores：", "# candidate="} {
		require.NotContains(t, prompts[1], field, "session memory metadata must stay out of compression materials")
	}
	require.NotContains(t, prompts[1], `\"content\"`)
	require.NoError(t, tl.sessionMemory.wait(context.Background()))
	require.Len(t, tl.GetSessionMemoryCandidates(), 1, "exact duplicates must not accumulate")
	require.Equal(t, candidate, tl.GetSessionMemoryCandidates()[0], "prompt projection must preserve stored metadata")
	require.NotContains(t, RenderTimelineFrozenOpen(tl).Frozen, "scores：", "candidate history is compression input only")
}

func TestTimelineSessionMemoryRestoreForkAndIsolation(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.sessionMemory = newTimelineSessionMemory(tl, []any{compressionMemoryFixture()})
	raw, err := MarshalTimeline(tl)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(raw)
	require.NoError(t, err)
	require.Equal(t, tl.GetSessionMemoryCandidates(), restored.GetSessionMemoryCandidates())
	owned := restored.GetSessionMemoryCandidates()
	owned[0].(map[string]any)["content"] = "observer mutated copy"
	require.NotEqual(t, owned, restored.GetSessionMemoryCandidates())
	fork, err := tl.ForkForTask("child", "private task", nil, nil)
	require.NoError(t, err)
	correction := compressionMemoryFixture()
	correction["content"] = "用户更正先前要求；仅在当前项目使用英文报告。"
	completion := &timelineCompressionCompletion{done: make(chan struct{}), output: &timelineCompressionOutput{MemoryEntities: []any{correction}}}
	fork.Branch.sessionMemory.record(completion)
	close(completion.done)
	require.Len(t, tl.GetSessionMemoryCandidates(), 2)
	require.Len(t, tl.CopyReducibleTimelineWithMemory().GetSessionMemoryCandidates(), 2)
	require.Empty(t, NewTimeline(nil, nil).GetSessionMemoryCandidates())
	var legacy map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &legacy))
	delete(legacy, "session_memory_candidates")
	old, err := json.Marshal(legacy)
	require.NoError(t, err)
	restored, err = UnmarshalTimeline(string(old))
	require.NoError(t, err)
	require.Empty(t, restored.GetSessionMemoryCandidates(), "old dumps remain readable")
}

func TestTimelineSessionMemoryPendingTailAndFailure(t *testing.T) {
	h := newTimelineSessionMemory(nil, nil)
	completion := &timelineCompressionCompletion{done: make(chan struct{})}
	h.record(completion)
	require.Empty(t, h.snapshot(), "prompt assembly must not wait for a pending tail")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, h.wait(ctx), context.DeadlineExceeded)
	completion.output = &timelineCompressionOutput{MemoryEntities: []any{compressionMemoryFixture()}}
	close(completion.done)
	require.NoError(t, h.wait(context.Background()))
	require.Len(t, h.snapshot(), 1)
	failed := &timelineCompressionCompletion{done: make(chan struct{}), output: completion.output, err: errors.New("invalid memory tail")}
	h.record(failed)
	close(failed.done)
	require.Len(t, h.snapshot(), 1)
}

func TestTimelineSessionMemoryRejectsDiscardedCompression(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "original source")
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		tl.PushText(1, "changed source")
		raw, err := json.Marshal(compressionOutputFixture("", []any{compressionMemoryFixture()}))
		return string(raw), err
	})
	_, err := tl.CompressOnce(compressionTestOptions())
	require.Error(t, err)
	require.Empty(t, tl.GetSessionMemoryCandidates())
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "changed source")
}

func TestTimelineSessionMemoryNextCompressionWaitsWithinContext(t *testing.T) {
	tl := NewTimeline(nil, nil)
	tl.PushText(1, "new observation after the published summary")
	completion := &timelineCompressionCompletion{done: make(chan struct{})}
	tl.sessionMemory.record(completion)
	var calls int
	bindCompressionMock(t, tl, func(request *AIRequest) (string, error) {
		calls++
		require.Contains(t, request.GetPrompt(), compressionMemoryFixture()["content"])
		return compressionMockSummary("next history"), nil
	})
	options := compressionTestOptions()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	options.Context = ctx
	_, err := tl.CompressOnce(options)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Zero(t, calls, "do not issue another extraction with missing prior candidates")
	require.Contains(t, RenderTimelineFrozenOpen(tl).Open, "new observation")
	completion.output = &timelineCompressionOutput{MemoryEntities: []any{compressionMemoryFixture()}}
	close(completion.done)
	_, err = tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}
