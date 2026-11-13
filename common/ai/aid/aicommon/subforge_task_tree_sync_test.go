package aicommon

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestSubForgeTaskTreeSyncBeyondHistoryPage(t *testing.T) {
	sessionID := uuid.NewString()
	var response *schema.AiOutputEvent
	cfg := NewConfig(context.Background(), WithWorkdir(t.TempDir()), WithPersistentSessionId(sessionID), WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.NodeId == "plan_exec_tasks" {
			response = e
		}
	}))
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AiOutputEvent{}, &schema.AISessionPlanAndExec{}).Error)
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("tree-sync", db)
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

func TestSubForgeTaskTreeSyncFailureKeepsParentResponse(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	// Model an older/partially initialized database: parent history is available.
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	require.NoError(t, db.Create(&schema.AISessionPlanAndExec{SessionID: "session", CoordinatorID: "parent", TaskTree: `{}`, TaskProgress: `{"phase":"Completed"}`}).Error)
	var response *schema.AiOutputEvent
	cfg := NewTestConfig(context.Background(), WithPersistentSessionId("session"), WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.NodeId == "plan_exec_tasks" {
			response = e
		}
	}))
	cfg.BaseCheckpointableStorage = NewCheckpointableStorageWithDB("restore", db)
	require.NoError(t, cfg.HandleSyncPlanExecTasksEvent(&ypb.AIInputEvent{SyncID: "restore"}))
	require.NotNil(t, response)
	require.Equal(t, "restore", response.SyncID)
	require.Equal(t, "1", response.GetContentJSONPath("$.total"))
}

func TestSubForgeTaskTreeSyncDeliveryPrecedesCompletion(t *testing.T) {
	entered, release, completed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cfg := NewTestConfig(context.Background(), WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.Type == schema.EVENT_TYPE_SUB_FORGE_TASK_TREE {
			close(entered)
			<-release
		}
		if e.NodeId == "recovery_history" {
			close(completed)
		}
	}))
	go func() {
		cfg.EmitSyncJSON(schema.EVENT_TYPE_SUB_FORGE_TASK_TREE, "sub-forge-task-tree", map[string]any{"status": "cancelled"}, "restore")
		cfg.EmitSyncJSON(schema.EVENT_TYPE_STRUCTURED, "recovery_history", map[string]any{}, "restore")
	}()
	<-entered
	select {
	case <-completed:
		t.Error("completion overtook terminal replay")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("replay did not complete")
	}
}
