package aicommon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func waitForOutputEvent(t *testing.T, ch <-chan *schema.AiOutputEvent, match func(*schema.AiOutputEvent) bool) *schema.AiOutputEvent {
	t.Helper()

	timeout := time.After(3 * time.Second)
	for {
		select {
		case evt := <-ch:
			if evt != nil && match(evt) {
				return evt
			}
		case <-timeout:
			t.Fatal("timed out waiting for matching output event")
		}
	}
}

func mustMarshalSyncInput(t *testing.T, content string) string {
	t.Helper()

	raw, err := json.Marshal(map[string]string{
		"content": content,
	})
	require.NoError(t, err)
	return string(raw)
}

func TestHandleSyncUserIntervention(t *testing.T) {
	t.Run("pushes timeline entry and emits sync response", func(t *testing.T) {
		events := make(chan *schema.AiOutputEvent, 8)
		c := NewTestConfig(context.Background(), WithEventHandler(func(e *schema.AiOutputEvent) {
			events <- e
		}))
		syncID := uuid.NewString()
		content := uuid.NewString()

		err := c.HandleSyncUserIntervention(&ypb.AIInputEvent{
			SyncID:        syncID,
			SyncJsonInput: mustMarshalSyncInput(t, content),
		})
		require.NoError(t, err)

		evt := waitForOutputEvent(t, events, func(e *schema.AiOutputEvent) bool {
			return e.IsSync && e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "user_intervention"
		})

		require.Equal(t, syncID, evt.SyncID)

		var payload map[string]string
		require.NoError(t, json.Unmarshal(evt.Content, &payload))
		require.Equal(t, content, payload["content"])

		entries := c.Timeline.ToTimelineItemOutputLastN(1)
		require.Len(t, entries, 1)
		require.Equal(t, "text", entries[0].Type)
		require.Equal(t, "[User Intervention] "+content, entries[0].Content)
	})

	t.Run("emits error when content is empty", func(t *testing.T) {
		events := make(chan *schema.AiOutputEvent, 8)
		c := NewTestConfig(context.Background(), WithEventHandler(func(e *schema.AiOutputEvent) {
			events <- e
		}))

		err := c.HandleSyncUserIntervention(&ypb.AIInputEvent{
			SyncJsonInput: `{"content":""}`,
		})
		require.NoError(t, err)

		evt := waitForOutputEvent(t, events, func(e *schema.AiOutputEvent) bool {
			return !e.IsSync && e.Type == schema.EVENT_TYPE_STRUCTURED && e.NodeId == "system"
		})

		var payload map[string]string
		require.NoError(t, json.Unmarshal(evt.Content, &payload))
		require.Equal(t, "error", payload["level"])
		require.Equal(t, "content is empty in sync json input", payload["message"])
		require.Nil(t, c.Timeline.ToTimelineItemOutputLastN(1))
	})
}

func TestProcessInputEvent_SyncUserIntervention(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events := make(chan *schema.AiOutputEvent, 16)
	c := NewTestConfig(ctx, WithEventHandler(func(e *schema.AiOutputEvent) {
		events <- e
	}))
	c.StartEventLoop(ctx)
	syncID := uuid.NewString()
	content := uuid.NewString()

	c.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_USER_INTERVENTION,
		SyncID:        syncID,
		SyncJsonInput: mustMarshalSyncInput(t, content),
	})

	evt := waitForOutputEvent(t, events, func(e *schema.AiOutputEvent) bool {
		return e.IsSync && e.SyncID == syncID && e.NodeId == "user_intervention"
	})

	var payload map[string]string
	require.NoError(t, json.Unmarshal(evt.Content, &payload))
	require.Equal(t, content, payload["content"])

	require.Eventually(t, func() bool {
		entries := c.Timeline.ToTimelineItemOutputLastN(1)
		return len(entries) == 1 && entries[0].Content == "[User Intervention] "+content
	}, time.Second, 20*time.Millisecond)
}
