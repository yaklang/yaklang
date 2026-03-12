package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_SyncPlanExecTasks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *schema.AiOutputEvent, 10)

	sessionID := uuid.NewString()
	db := consts.GetGormProjectDatabase()
	require.NoError(t, db.AutoMigrate(&schema.AISessionPlanAndExec{}).Error)
	t.Cleanup(func() {
		_ = yakit.DeleteAISessionPlanAndExecBySessionID(db, sessionID)
	})

	record1 := &schema.AISessionPlanAndExec{
		SessionID:     sessionID,
		CoordinatorID: uuid.NewString(),
		TaskTree:      `{"root":"t1"}`,
		TaskProgress:  `{"phase":"executing"}`,
	}
	record2 := &schema.AISessionPlanAndExec{
		SessionID:     sessionID,
		CoordinatorID: uuid.NewString(),
		TaskTree:      `{"root":"t2"}`,
		TaskProgress:  `{"phase":"paused"}`,
	}
	require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(db, record1))
	require.NoError(t, yakit.CreateOrUpdateAISessionPlanAndExec(db, record2))

	_, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e
		}),
	)
	require.NoError(t, err)

	syncID := uuid.NewString()
	in <- &ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_PLAN_EXEC_TASKS,
		SyncID:        syncID,
	}

	var result *schema.AiOutputEvent
LOOP:
	for {
		select {
		case event := <-out:
			if event != nil && event.IsSync && event.SyncID == syncID && event.NodeId == "plan_exec_tasks" {
				result = event
				break LOOP
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for plan_exec_tasks sync event")
		}
	}

	require.NotNil(t, result)
	require.Equal(t, schema.EVENT_TYPE_STRUCTURED, result.Type)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(result.Content, &payload))
	require.Equal(t, sessionID, utils.InterfaceToString(payload["session_id"]))

	total := utils.InterfaceToInt(payload["total"])
	require.Equal(t, 2, total)

	rawRecords, ok := payload["records"].([]any)
	require.True(t, ok)
	require.Len(t, rawRecords, 2)

	got := make(map[string]bool)
	for _, item := range rawRecords {
		row, ok := item.(map[string]any)
		require.True(t, ok)
		got[utils.InterfaceToString(row["coordinator_id"])] = true
	}
	require.True(t, got[record1.CoordinatorID])
	require.True(t, got[record2.CoordinatorID])
}
