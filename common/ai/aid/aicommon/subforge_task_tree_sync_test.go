package aicommon

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestSubForgeTaskTreeSyncBeyondHistoryPage(t *testing.T) {
	sessionID := uuid.NewString()
	var response *schema.AiOutputEvent
	cfg := NewConfig(context.Background(), WithWorkdir(t.TempDir()), WithPersistentSessionId(sessionID), WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.NodeId == "plan_exec_tasks" {
			response = e
		}
	}))
	db := cfg.GetDB()
	t.Cleanup(func() { db.Unscoped().Where("session_id = ?", sessionID).Delete(&schema.AiOutputEvent{}) })
	// The highest revision can have an older row ID than a delayed projection.
	for _, revision := range []int{3, 1, 2} {
		content, err := json.Marshal(map[string]any{"child_coordinator_id": "child", "revision": revision, "status": "succeeded"})
		require.NoError(t, err)
		require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: sessionID, Type: schema.EVENT_TYPE_SUB_FORGE_TASK_TREE, Content: content, IsJson: true}).Error)
	}
	for i := 0; i < 70; i++ {
		require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: sessionID, Type: schema.EVENT_TYPE_STRUCTURED, Content: []byte(`{}`), IsJson: true, IsRecoveryBlock: true}).Error)
	}
	require.NoError(t, db.Create(&schema.AiOutputEvent{SessionId: uuid.NewString(), Type: schema.EVENT_TYPE_SUB_FORGE_TASK_TREE, Content: []byte(`{"child_coordinator_id":"foreign","revision":9}`), IsJson: true}).Error)
	require.NoError(t, cfg.HandleSyncPlanExecTasksEvent(&ypb.AIInputEvent{SyncID: "restore"}))
	require.NotNil(t, response)
	var payload struct {
		Trees []struct {
			Child    string `json:"child_coordinator_id"`
			Revision int    `json:"revision"`
		} `json:"sub_forge_task_trees"`
	}
	require.NoError(t, json.Unmarshal(response.Content, &payload))
	require.Len(t, payload.Trees, 1)
	require.Equal(t, "child", payload.Trees[0].Child)
	require.Equal(t, 3, payload.Trees[0].Revision)
}
