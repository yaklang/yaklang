package aicommon

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimelineSessionMemoryCallbacksShareForksButNotRestore(t *testing.T) {
	parent := NewTimeline(nil, nil)
	parent.PushText(1, "original history")
	before, err := MarshalTimeline(parent)
	require.NoError(t, err)
	received := make(chan TimelineMemoryEvent, 8)
	parent.RegisterSessionMemoryCallback("storage", func(event TimelineMemoryEvent) { received <- event })
	after, err := MarshalTimeline(parent)
	require.NoError(t, err)
	require.Equal(t, before, after, "listeners must not enter persistence or prompts")
	fork, err := parent.ForkForTask("child", "worker", nil, nil)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(before)
	require.NoError(t, err)
	require.True(t, parent.IsSessionTimeline())
	require.True(t, restored.IsSessionTimeline())
	require.Empty(t, restored.sessionMemory.callbacks.snapshot(), "recovery must rebind its backend")
	for _, tl := range []*Timeline{parent, fork.Branch, parent.CopyReducibleTimelineWithMemory(), parent.CreateSubTimeline(1)} {
		if tl != parent {
			require.False(t, tl.IsSessionTimeline())
		}
		bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
			raw, err := json.Marshal(compressionOutputFixture("", []any{compressionMemoryFixture()}))
			return string(raw), err
		})
		tl.PushText(20, "new verified observation")
		_, err := tl.CompressOnce(compressionTestOptions())
		require.NoError(t, err)
		select {
		case event := <-received:
			require.NoError(t, event.Err)
			require.Len(t, event.MemoryEntities, 1)
		case <-time.After(3 * time.Second):
			t.Fatal("shared session callback was not delivered")
		}
	}
	parent.RegisterSessionMemoryCallback("storage", nil)
	require.Empty(t, fork.Branch.sessionMemory.callbacks.snapshot())
}

func TestTimelineSessionMemoryCallbacksDetachAndReplace(t *testing.T) {
	tl := NewTimeline(nil, nil)
	received := make(chan TimelineMemoryEvent, 1)
	tl.RegisterMemoryCallback("local", func(event TimelineMemoryEvent) {
		event.MemoryEntities[0].(map[string]any)["content"] = "local observer mutation"
	})
	tl.RegisterSessionMemoryCallback("storage", func(TimelineMemoryEvent) { t.Error("replaced listener invoked") })
	tl.RegisterSessionMemoryCallback("storage", func(event TimelineMemoryEvent) { received <- event })
	bindCompressionMock(t, tl, func(*AIRequest) (string, error) {
		raw, err := json.Marshal(compressionOutputFixture("", []any{compressionMemoryFixture()}))
		return string(raw), err
	})
	tl.PushText(1, "verified observation")
	_, err := tl.CompressOnce(compressionTestOptions())
	require.NoError(t, err)
	select {
	case event := <-received:
		require.Equal(t, compressionMemoryFixture(), event.MemoryEntities[0])
	case <-time.After(3 * time.Second):
		t.Fatal("session callback was not delivered")
	}
}
