package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func compressionMemoryFixture() map[string]any {
	return map[string]any{
		"content":             "用户明确要求工程报告使用中文，附实际验证结果。",
		"potential_questions": []any{"工程报告应该如何撰写？"},
		"tags":                []any{"工程", "偏好"},
		"scores": map[string]any{"temporality": 0.9, "actionability": 0.8, "perference": 1.0,
			"origin": 1.0, "emotion": 0.1, "relevance": 0.8, "connectivity": 0.5},
	}
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
	completion := &timelineCompressionCompletion{done: make(chan struct{}), output: &TimelineCompressionSelection{MemoryEntities: []any{correction}}}
	fork.Branch.sessionMemory.record(&timelineCompressionSnapshot{MemoryCompletion: completion})
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
	h.record(&timelineCompressionSnapshot{MemoryCompletion: completion})
	require.Empty(t, h.snapshot(), "prompt assembly must not wait for a pending tail")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	waits := 0
	require.ErrorIs(t, h.waitWithProgress(ctx, func() { waits++ }), context.DeadlineExceeded)
	require.Equal(t, 1, waits)
	completion.output = &TimelineCompressionSelection{MemoryEntities: []any{compressionMemoryFixture()}}
	close(completion.done)
	require.NoError(t, h.waitWithProgress(context.Background(), func() { waits++ }))
	require.Equal(t, 1, waits, "completed memory must not flash a waiting status")
	require.Len(t, h.snapshot(), 1)
	failed := &timelineCompressionCompletion{done: make(chan struct{}), output: completion.output, err: errors.New("invalid memory tail")}
	h.record(&timelineCompressionSnapshot{MemoryCompletion: failed})
	close(failed.done)
	require.Len(t, h.snapshot(), 1)
}
