package aireact

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func mustMarshalSyncInput(t *testing.T, content string) string {
	t.Helper()

	raw, err := json.Marshal(map[string]string{
		"content": content,
	})
	require.NoError(t, err)
	return string(raw)
}

func TestReAct_SyncUserIntervention(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 16)

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
	)
	require.NoError(t, err)

	syncID := uuid.NewString()
	content := uuid.NewString()
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_USER_INTERVENTION,
		SyncID:        syncID,
		SyncJsonInput: mustMarshalSyncInput(t, content),
	}

	var result *schema.AiOutputEvent
LOOP:
	for {
		select {
		case event := <-out:
			if event != nil && event.IsSync && event.SyncID == syncID && event.NodeId == "user_intervention" {
				result = event
				break LOOP
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for user_intervention sync event")
		}
	}

	require.NotNil(t, result)
	require.Equal(t, schema.EVENT_TYPE_STRUCTURED, result.Type)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(result.Content, &payload))
	require.Equal(t, content, payload["content"])

	require.Eventually(t, func() bool {
		return strings.Contains(ins.DumpTimeline(), "[User Intervention] "+content)
	}, time.Second, 20*time.Millisecond)
}

func TestReAct_SyncUserIntervention_EmptyContent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 16)

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
	)
	require.NoError(t, err)

	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_USER_INTERVENTION,
		SyncJsonInput: mustMarshalSyncInput(t, ""),
	}

	var result *schema.AiOutputEvent
LOOP:
	for {
		select {
		case event := <-out:
			if event != nil && !event.IsSync && event.NodeId == "system" {
				var payload map[string]string
				if json.Unmarshal(event.Content, &payload) == nil &&
					payload["level"] == "error" &&
					payload["message"] == "content is empty in sync json input" {
					result = event
					break LOOP
				}
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for user_intervention error event")
		}
	}

	require.NotNil(t, result)
	require.Empty(t, ins.DumpTimeline())
}
