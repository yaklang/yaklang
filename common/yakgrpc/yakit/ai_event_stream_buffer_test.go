package yakit

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func TestSaveStreamAIEvent_CoalesceAndFlush(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)

	eventID := uuid.NewString()
	t.Cleanup(func() { FinishStreamAIEvent(db, eventID) })

	first := &schema.AiOutputEvent{
		EventUUID:   eventID,
		IsStream:    true,
		Type:        schema.EVENT_TYPE_STREAM,
		StreamDelta: []byte("a"),
	}
	require.NoError(t, CreateOrUpdateAIOutputEvent(db, first))

	second := &schema.AiOutputEvent{
		EventUUID:   eventID,
		IsStream:    true,
		Type:        schema.EVENT_TYPE_STREAM,
		StreamDelta: []byte("b"),
	}
	require.NoError(t, CreateOrUpdateAIOutputEvent(db, second))

	FinishStreamAIEvent(db, eventID)

	var out schema.AiOutputEvent
	require.NoError(t, db.Where("event_uuid = ?", eventID).First(&out).Error)
	require.Equal(t, "ab", string(out.StreamDelta))
	_, ok := globalStreamEventBuffer.entries.Get(eventID)
	require.False(t, ok)
}

func TestStreamFinished_ClosesStreamBuffer(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}).Error)

	eventID := uuid.NewString()
	t.Cleanup(func() { FinishStreamAIEvent(db, eventID) })

	require.NoError(t, CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		EventUUID:   eventID,
		IsStream:    true,
		Type:        schema.EVENT_TYPE_STREAM,
		StreamDelta: []byte("a"),
	}))
	require.NoError(t, CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		EventUUID:   eventID,
		IsStream:    true,
		Type:        schema.EVENT_TYPE_STREAM,
		StreamDelta: []byte("b"),
	}))

	// Emulate emitter's structured finish event:
	// NodeId == "stream-finished" and JSON contains event_writer_id.
	require.NoError(t, CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		NodeId:    "stream-finished",
		Type:      schema.EVENT_TYPE_STRUCTURED,
		IsJson:    true,
		Content:   utils.Jsonify(map[string]any{"event_writer_id": eventID}),
		Timestamp: 1,
	}))

	var out schema.AiOutputEvent
	require.NoError(t, db.Where("event_uuid = ?", eventID).First(&out).Error)
	require.Equal(t, "ab", string(out.StreamDelta))

	// Buffer entry is removed (no need to wait for idle TTL).
	_, ok := globalStreamEventBuffer.entries.Get(eventID)
	require.False(t, ok)
}
