package yakit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestYieldAIEvent(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)

	err = db.AutoMigrate(&schema.AiOutputEvent{}).Error
	require.NoError(t, err)

	// Prepare data
	totalEvents := 100
	eventUUIDs := make([]string, totalEvents)
	types := []schema.EventType{"type_a", "type_b", "type_c"}

	tx := db.Begin()
	for i := 0; i < totalEvents; i++ {
		uuidStr := uuid.NewString()
		eventUUIDs[i] = uuidStr
		event := &schema.AiOutputEvent{
			EventUUID:     uuidStr,
			CoordinatorId: fmt.Sprintf("coord-%d", i%5), // 5 coordinators
			Type:          types[i%3],                   // 3 types
			TaskIndex:     fmt.Sprintf("task-%d", i%10), // 10 tasks
			Content:       []byte(fmt.Sprintf("content-%d", i)),
			Timestamp:     time.Now().Unix(),
		}
		err := tx.Create(event).Error
		require.NoError(t, err)
	}
	tx.Commit()

	t.Run("Basic_Yield_All", func(t *testing.T) {
		ctx := context.Background()
		filter := &ypb.AIEventFilter{} // Empty filter
		ch := YieldAIEvent(ctx, db, filter)

		count := 0
		for range ch {
			count++
		}
		assert.Equal(t, totalEvents, count)
	})

	t.Run("Filter_Single_Large_Array", func(t *testing.T) {
		// Test chunking logic with > 10 items (batch size is 10)
		targetUUIDs := eventUUIDs[:25] // 25 items -> 3 chunks
		ctx := context.Background()
		filter := &ypb.AIEventFilter{
			EventUUIDS: targetUUIDs,
		}

		ch := YieldAIEvent(ctx, db, filter)
		var results []*schema.AiOutputEvent
		for item := range ch {
			results = append(results, item)
		}

		assert.Equal(t, 25, len(results))
		// Verify IDs
		foundIDs := make(map[string]bool)
		for _, item := range results {
			foundIDs[item.EventUUID] = true
		}
		for _, id := range targetUUIDs {
			assert.True(t, foundIDs[id], "UUID %s should be found", id)
		}
	})

	t.Run("Filter_Multiple_Arrays_Cartesian", func(t *testing.T) {
		targetTypes := []string{"type_a", "type_b"}
		targetCoords := []string{"coord-0", "coord-1"}

		filter := &ypb.AIEventFilter{
			EventType:     targetTypes,
			CoordinatorId: targetCoords,
		}

		ch := YieldAIEvent(context.Background(), db, filter)
		var results []*schema.AiOutputEvent
		for item := range ch {
			results = append(results, item)
		}

		expectedCount := 0
		for i := 0; i < totalEvents; i++ {
			tStr := string(types[i%3])
			cStr := fmt.Sprintf("coord-%d", i%5)
			matchType := false
			for _, t := range targetTypes {
				if t == tStr {
					matchType = true
					break
				}
			}
			matchCoord := false
			for _, c := range targetCoords {
				if c == cStr {
					matchCoord = true
					break
				}
			}

			if matchType && matchCoord {
				expectedCount++
			}
		}

		assert.Equal(t, expectedCount, len(results))
	})

	t.Run("Filter_Huge_Multiple_Arrays", func(t *testing.T) {

		hugeList1 := make([]string, 25) // 3 chunks
		for i := 0; i < 25; i++ {
			hugeList1[i] = fmt.Sprintf("coord-%d", i)
		}

		hugeList2 := make([]string, 25) // 3 chunks
		for i := 0; i < 25; i++ {
			hugeList2[i] = fmt.Sprintf("task-%d", i)
		}

		// Total combinations: 3 * 3 = 9 chunks queries
		filter := &ypb.AIEventFilter{
			CoordinatorId: hugeList1,
			TaskIndex:     hugeList2,
		}

		ch := YieldAIEvent(context.Background(), db, filter)
		count := 0
		for range ch {
			count++
		}
		t.Logf("Huge filter query returned %d items", count)
	})

	t.Run("Context_Cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		filter := &ypb.AIEventFilter{} // All items
		ch := YieldAIEvent(ctx, db, filter)

		<-ch
		cancel()

		count := 0
		for range ch {
			count++
		}
		assert.True(t, count < totalEvents-1)
	})
}

func TestAiOutputEvent_ShouldSave(t *testing.T) {
	cases := []struct {
		name  string
		event *schema.AiOutputEvent
		want  bool
	}{
		{
			name:  "system event should not save",
			event: &schema.AiOutputEvent{IsSystem: true},
			want:  false,
		},
		{
			name:  "sync event should not save",
			event: &schema.AiOutputEvent{IsSync: true},
			want:  false,
		},
		{
			name:  "transient event type should not save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_CONSUMPTION},
			want:  false,
		},
		{
			name:  "structured status should not save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "status"},
			want:  false,
		},
		{
			name:  "structured system should not save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "system"},
			want:  false,
		},
		{
			name:  "structured status detail should save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "status_detail"},
			want:  true,
		},
		{
			name:  "structured user should save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "user"},
			want:  true,
		},
		{
			name:  "non structured status should save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STREAM, NodeId: "status"},
			want:  true,
		},
		{
			name:  "regular structured event should save",
			event: &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "artifact"},
			want:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.event.ShouldSave(); got != tc.want {
				t.Fatalf("ShouldSave() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAiOutputEvent_NormalizeRecoveryBlock(t *testing.T) {
	cases := []struct {
		name              string
		event             *schema.AiOutputEvent
		wantRecoveryBlock bool
		wantRecoveryID    string
	}{
		{
			name:              "plain structured event is its own recovery block",
			event:             &schema.AiOutputEvent{Type: schema.EVENT_TYPE_STRUCTURED, NodeId: "user"},
			wantRecoveryBlock: true,
			wantRecoveryID:    "",
		},
		{
			name: "tool call start anchors the tool block",
			event: &schema.AiOutputEvent{
				Type:       schema.EVENT_TOOL_CALL_START,
				CallToolID: "call-tool-1",
			},
			wantRecoveryBlock: true,
			wantRecoveryID:    "call-tool-1",
		},
		{
			name: "tool call update belongs to tool block but is not anchor",
			event: &schema.AiOutputEvent{
				Type:       schema.EVENT_TOOL_CALL_RESULT,
				CallToolID: "call-tool-1",
			},
			wantRecoveryBlock: false,
			wantRecoveryID:    "call-tool-1",
		},
		{
			name: "stream start anchors stream block by writer id",
			event: &schema.AiOutputEvent{
				Type:    schema.EVENT_TYPE_STREAM_START,
				IsJson:  true,
				Content: utils.Jsonify(map[string]any{"event_writer_id": "writer-1"}),
			},
			wantRecoveryBlock: true,
			wantRecoveryID:    "writer-1",
		},
		{
			name: "stream delta without explicit recovery id falls back to standalone block",
			event: &schema.AiOutputEvent{
				Type:      schema.EVENT_TYPE_STREAM,
				IsStream:  true,
				EventUUID: "writer-1",
			},
			wantRecoveryBlock: true,
			wantRecoveryID:    "",
		},
		{
			name: "tool call owned stream prefers tool block over stream writer",
			event: &schema.AiOutputEvent{
				Type:       schema.EVENT_TYPE_STREAM,
				IsStream:   true,
				EventUUID:  "writer-1",
				CallToolID: "call-tool-1",
			},
			wantRecoveryBlock: false,
			wantRecoveryID:    "call-tool-1",
		},
		{
			name: "stream finished stays inside stream block",
			event: &schema.AiOutputEvent{
				Type:    schema.EVENT_TYPE_STRUCTURED,
				NodeId:  "stream-finished",
				IsJson:  true,
				Content: utils.Jsonify(map[string]any{"event_writer_id": "writer-1"}),
			},
			wantRecoveryBlock: false,
			wantRecoveryID:    "writer-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.event.NormalizeRecoveryBlock()
			require.Equal(t, tc.wantRecoveryBlock, tc.event.IsRecoveryBlock)
			require.Equal(t, tc.wantRecoveryID, tc.event.RecoveryIndexID)
		})
	}
}

func TestYieldAIEventRecoveryHistory(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)

	create := func(event *schema.AiOutputEvent) uint {
		t.Helper()
		require.NoError(t, db.Create(event).Error)
		return event.ID
	}

	blockAID := create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-1",
		SessionId:       "session-1",
		Type:            schema.EVENT_TYPE_STRUCTURED,
		NodeId:          "artifact",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-a"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: true,
	})
	blockBStartID := create(&schema.AiOutputEvent{
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
	blockBResultID := create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-2",
		SessionId:       "session-1",
		Type:            schema.EVENT_TOOL_CALL_RESULT,
		NodeId:          "tool",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-b-result"}),
		Timestamp:       time.Now().Unix(),
		RecoveryIndexID: "tool-1",
	})
	blockCID := create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-3",
		SessionId:       "session-1",
		Type:            schema.EVENT_TYPE_STRUCTURED,
		NodeId:          "artifact",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "block-c"}),
		Timestamp:       time.Now().Unix(),
		IsRecoveryBlock: true,
	})

	require.NoError(t, db.Create(&schema.AiOutputEvent{
		CoordinatorId:   "coord-9",
		SessionId:       "session-2",
		Type:            schema.EVENT_TOOL_CALL_RESULT,
		NodeId:          "tool",
		IsJson:          true,
		Content:         utils.Jsonify(map[string]any{"label": "other-session-result"}),
		Timestamp:       time.Now().Unix(),
		RecoveryIndexID: "tool-1",
	}).Error)

	stream, result, err := YieldAIEventRecoveryHistory(context.Background(), db, "session-1", 0, 2)
	require.NoError(t, err)
	var events []*schema.AiOutputEvent
	for event := range stream {
		events = append(events, event)
	}
	require.Equal(t, 2, result.BlockCount)
	require.Equal(t, 3, result.EventCount)
	require.Equal(t, int64(blockBStartID), result.NextStartID)
	require.True(t, result.HasMore)
	require.Len(t, events, 3)
	require.Equal(t, blockCID, events[0].ID)
	require.Equal(t, blockBStartID, events[1].ID)
	require.Equal(t, blockBResultID, events[2].ID)

	stream, result, err = YieldAIEventRecoveryHistory(context.Background(), db, "session-1", int64(blockBStartID), 2)
	require.NoError(t, err)
	events = events[:0]
	for event := range stream {
		events = append(events, event)
	}
	require.Equal(t, 1, result.BlockCount)
	require.Equal(t, 1, result.EventCount)
	require.Equal(t, int64(blockAID), result.NextStartID)
	require.False(t, result.HasMore)
	require.Len(t, events, 1)
	require.Equal(t, blockAID, events[0].ID)
}
