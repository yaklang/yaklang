package yakgrpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestServer_DeleteAIEvent_BySession(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	c, ok := client.(*Client)
	require.True(t, ok)
	db := c.GetProjectDatabase()

	sessionA := "sess-" + uuid.NewString()
	sessionB := "sess-" + uuid.NewString()

	processA := uuid.NewString()
	processB := uuid.NewString()
	eventA := uuid.NewString()
	eventB := uuid.NewString()

	require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		EventUUID:   eventA,
		SessionId:   sessionA,
		IsStream:    false,
		Type:        schema.EVENT_TYPE_THOUGHT,
		ProcessesId: []string{processA},
	}))
	require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		EventUUID:   eventB,
		SessionId:   sessionB,
		IsStream:    false,
		Type:        schema.EVENT_TYPE_THOUGHT,
		ProcessesId: []string{processB},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rsp, err := client.DeleteAIEvent(ctx, &ypb.AIEventDeleteRequest{
		Filter: &ypb.AIEventFilter{SessionID: sessionA},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), rsp.GetEffectRows())

	var cnt int64
	require.NoError(t, db.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionA).Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
	require.NoError(t, db.Model(&schema.AiOutputEvent{}).Where("session_id = ?", sessionB).Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)

	require.NoError(t, db.Model(&schema.AiProcessAndAiEvent{}).Where("event_id = ?", eventA).Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)
	require.NoError(t, db.Model(&schema.AiProcessAndAiEvent{}).Where("event_id = ?", eventB).Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)
}

func TestServer_DeleteAIEvent_ClearAll(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	c, ok := client.(*Client)
	require.True(t, ok)
	db := c.GetProjectDatabase()

	require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		EventUUID:   uuid.NewString(),
		SessionId:   "sess-" + uuid.NewString(),
		IsStream:    false,
		Type:        schema.EVENT_TYPE_THOUGHT,
		ProcessesId: []string{uuid.NewString()},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.DeleteAIEvent(ctx, &ypb.AIEventDeleteRequest{ClearAll: true})
	require.NoError(t, err)

	var cnt int64
	require.NoError(t, db.Model(&schema.AiOutputEvent{}).Count(&cnt).Error)
	require.Equal(t, int64(0), cnt)

	// tables should be re-created and still usable
	require.NoError(t, yakit.CreateOrUpdateAIOutputEvent(db, &schema.AiOutputEvent{
		EventUUID:   uuid.NewString(),
		SessionId:   "sess-" + uuid.NewString(),
		IsStream:    false,
		Type:        schema.EVENT_TYPE_THOUGHT,
		ProcessesId: []string{uuid.NewString()},
	}))
	require.NoError(t, db.Model(&schema.AiOutputEvent{}).Count(&cnt).Error)
	require.Equal(t, int64(1), cnt)
}
