package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHandleSyncTypeCloseBrowserEvent_EmitsSessionSnapshot(t *testing.T) {
	ctx := context.Background()
	var snapshotEvents []*schema.AiOutputEvent

	ins, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e != nil && e.NodeId == aicommon.SessionSnapshotNodeID {
				snapshotEvents = append(snapshotEvents, e)
			}
		}),
	)
	require.NoError(t, err)

	browserID := "test-browser-" + uuid.NewString()
	ins.TrackBrowserSession(browserID)
	require.Len(t, ins.config.BuildSessionSnapshotBackgroundProcesses(), 1)

	syncID := uuid.NewString()
	syncInput, err := json.Marshal(map[string]string{"browser_id": browserID})
	require.NoError(t, err)

	err = ins.HandleSyncTypeCloseBrowserEvent(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncID:        syncID,
		SyncJsonInput: string(syncInput),
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(snapshotEvents) > 0
	}, time.Second, 20*time.Millisecond)

	last := snapshotEvents[len(snapshotEvents)-1]
	require.Equal(t, schema.EVENT_TYPE_STRUCTURED, last.Type)
	require.Equal(t, aicommon.SessionSnapshotNodeID, last.NodeId)

	var snapshot aicommon.SessionSnapshot
	require.NoError(t, json.Unmarshal(last.Content, &snapshot))
	require.Empty(t, snapshot.BackgroundProcesses)
	require.Empty(t, ins.config.BuildSessionSnapshotBackgroundProcesses())
}
