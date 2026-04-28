package aicommon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type recoveryHistorySeed struct {
	blockAID       uint
	blockBStartID  uint
	blockBResultID uint
	blockCID       uint
}

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

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func decodeEventPayload(t *testing.T, event *schema.AiOutputEvent) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(event.Content, &payload))
	return payload
}

func collectUntilSyncResponse(t *testing.T, ch <-chan *schema.AiOutputEvent, syncID string) []*schema.AiOutputEvent {
	t.Helper()

	var events []*schema.AiOutputEvent
	timeout := time.After(3 * time.Second)
	for {
		select {
		case evt := <-ch:
			if evt == nil {
				continue
			}
			events = append(events, evt)
			if evt.IsSync && evt.SyncID == syncID && evt.NodeId == "recovery_history" {
				return events
			}
		case <-timeout:
			t.Fatal("timed out waiting for recovery history sync response")
		}
	}
}

func newRecoveryHistoryTestConfig(t *testing.T, ctx context.Context, handler func(*schema.AiOutputEvent)) (*Config, *recoveryHistorySeed) {
	t.Helper()

	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)

	cfg := NewTestConfig(ctx,
		WithID("coord-1"),
		WithPersistentSessionId("session-1"),
		WithEventHandler(handler),
	)
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("coord-1", db)

	create := func(event *schema.AiOutputEvent) uint {
		t.Helper()
		require.NoError(t, db.Create(event).Error)
		return event.ID
	}

	seed := &recoveryHistorySeed{}
	seed.blockAID = create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-1",
		SessionId:       "session-1",
		Type:            schema.EVENT_TYPE_STRUCTURED,
		NodeId:          "artifact",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-a"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: true,
		RecoveryIndexID: "",
	})
	seed.blockBStartID = create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-1",
		SessionId:       "session-1",
		Type:            schema.EVENT_TOOL_CALL_START,
		NodeId:          "tool",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-b-start"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: true,
		RecoveryIndexID: "tool-1",
	})
	seed.blockBResultID = create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-2",
		SessionId:       "session-1",
		Type:            schema.EVENT_TOOL_CALL_RESULT,
		NodeId:          "tool",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-b-result"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: false,
		RecoveryIndexID: "tool-1",
	})
	seed.blockCID = create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-1",
		SessionId:       "session-1",
		Type:            schema.EVENT_TYPE_STRUCTURED,
		NodeId:          "artifact",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-c"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: true,
		RecoveryIndexID: "",
	})

	require.NoError(t, db.Create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-2",
		SessionId:       "session-2",
		Type:            schema.EVENT_TOOL_CALL_RESULT,
		NodeId:          "tool",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "other-session-result"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: false,
		RecoveryIndexID: "tool-1",
	}).Error)

	return cfg, seed
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
		history := c.GetUserInputHistory()
		require.Len(t, history, 1)
		require.Equal(t, content, history[0].UserInput)
		require.Equal(t, content, c.GetPrevSessionUserInput())
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

	history := c.GetUserInputHistory()
	require.Len(t, history, 1)
	require.Equal(t, content, history[0].UserInput)
}

func TestHandleSyncRecoveryHistory(t *testing.T) {
	var emitted []*schema.AiOutputEvent
	cfg, seed := newRecoveryHistoryTestConfig(t, context.Background(), func(e *schema.AiOutputEvent) {
		emitted = append(emitted, e)
	})

	syncID := uuid.NewString()
	err := cfg.HandleSyncRecoveryHistoryEvent(&ypb.AIInputEvent{
		SyncID:        syncID,
		SyncJsonInput: mustMarshalJSON(t, map[string]any{"limit": 2}),
	})
	require.NoError(t, err)
	require.Len(t, emitted, 4)

	require.True(t, emitted[0].IsSync)
	require.Equal(t, seed.blockCID, emitted[0].ID)

	require.True(t, emitted[1].IsSync)
	require.Equal(t, seed.blockBStartID, emitted[1].ID)
	require.Equal(t, schema.EventType(schema.EVENT_TOOL_CALL_START), emitted[1].Type)

	require.True(t, emitted[2].IsSync)
	require.Equal(t, seed.blockBResultID, emitted[2].ID)
	require.Equal(t, schema.EventType(schema.EVENT_TOOL_CALL_RESULT), emitted[2].Type)

	response := emitted[3]
	require.True(t, response.IsSync)
	require.Equal(t, syncID, response.SyncID)
	require.Equal(t, schema.EVENT_TYPE_STRUCTURED, response.Type)
	require.Equal(t, "recovery_history", response.NodeId)

	payload := decodeEventPayload(t, response)
	require.Equal(t, "session-1", payload["session_id"])
	require.Equal(t, float64(0), payload["requested_start_id"])
	require.Equal(t, float64(2), payload["block_count"])
	require.Equal(t, float64(3), payload["event_count"])
	require.Equal(t, float64(seed.blockBStartID), payload["next_start_id"])
	require.Equal(t, true, payload["has_more"])
}

func TestProcessInputEvent_SyncRecoveryHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events := make(chan *schema.AiOutputEvent, 16)
	cfg, seed := newRecoveryHistoryTestConfig(t, ctx, func(e *schema.AiOutputEvent) {
		events <- e
	})
	cfg.StartEventLoop(ctx)

	syncID := uuid.NewString()
	cfg.EventInputChan.SafeFeed(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      SYNC_TYPE_RECOVERY_HISTORY,
		SyncID:        syncID,
		SyncJsonInput: mustMarshalJSON(t, map[string]any{
			"start_id": seed.blockBStartID,
			"limit":    2,
		}),
	})

	emitted := collectUntilSyncResponse(t, events, syncID)
	require.Len(t, emitted, 2)

	require.False(t, emitted[0].IsSync)
	require.Equal(t, seed.blockAID, emitted[0].ID)

	response := emitted[1]
	require.True(t, response.IsSync)
	payload := decodeEventPayload(t, response)
	require.Equal(t, float64(seed.blockBStartID), payload["requested_start_id"])
	require.Equal(t, float64(1), payload["block_count"])
	require.Equal(t, float64(1), payload["event_count"])
	require.Equal(t, float64(seed.blockAID), payload["next_start_id"])
	require.Equal(t, false, payload["has_more"])
}
