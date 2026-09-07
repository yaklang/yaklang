package scheduletools

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestScheduleManagementTools(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}, &schema.AIReActSchedule{}).Error)
	const sessionID = "manage-schedule-session"
	_, err = yakit.CreateOrUpdateAISessionMetaStartParams(db, sessionID, &ypb.AIStartParams{})
	require.NoError(t, err)
	runtimeConfig := &aitool.ToolRuntimeConfig{ProjectDatabase: db, PersistentSessionID: sessionID}

	create, err := createScheduleTool(func() time.Time { return time.Now().UTC() })
	require.NoError(t, err)
	createdRaw, err := create.Callback(context.Background(), aitool.InvokeParams{
		"name": "weather", "task_prompt": "query weather", "rrule": "RRULE:FREQ=HOURLY;INTERVAL=1", "timezone": "UTC",
	}, runtimeConfig, &bytes.Buffer{}, &bytes.Buffer{})
	require.NoError(t, err)
	uuid := createdRaw.(map[string]any)["schedule_uuid"].(string)

	list, err := listSchedulesTool()
	require.NoError(t, err)
	listedRaw, err := list.Callback(context.Background(), nil, runtimeConfig, &bytes.Buffer{}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, 1, listedRaw.(map[string]any)["count"])

	update, err := updateSchedulesTool()
	require.NoError(t, err)
	_, err = update.Callback(context.Background(), aitool.InvokeParams{
		"schedule_uuid": uuid, "task_prompt": "query Chengdu weather",
	}, runtimeConfig, &bytes.Buffer{}, &bytes.Buffer{})
	require.NoError(t, err)
	record := &schema.AIReActSchedule{}
	require.NoError(t, db.Where("uuid = ?", uuid).First(record).Error)
	require.Equal(t, "query Chengdu weather", record.Prompt)
	require.Equal(t, "query weather", record.OriginalRequest)

	setEnabled, err := setScheduleEnabledTool()
	require.NoError(t, err)
	var executionCancelled atomic.Bool
	unregisterExecution := aischedule.RegisterExecution(uuid, func() {
		executionCancelled.Store(true)
	})
	defer unregisterExecution()
	_, err = setEnabled.Callback(context.Background(), aitool.InvokeParams{
		"schedule_uuid": uuid, "enabled": false,
	}, runtimeConfig, &bytes.Buffer{}, &bytes.Buffer{})
	require.NoError(t, err)
	require.NoError(t, db.Where("uuid = ?", uuid).First(record).Error)
	require.Equal(t, schema.AIReActScheduleStatusPaused, record.Status)
	require.False(t, executionCancelled.Load(), "pausing must not cancel an execution that already started")

	deleteTool, err := deleteScheduleTool()
	require.NoError(t, err)
	_, err = deleteTool.Callback(context.Background(), aitool.InvokeParams{"schedule_uuid": uuid}, runtimeConfig, &bytes.Buffer{}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Error(t, db.Where("uuid = ?", uuid).First(&schema.AIReActSchedule{}).Error)
	require.False(t, executionCancelled.Load(), "deleting must not cancel an execution that already started")
}
