package scheduletools

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestCreateScheduleToolEveryFiveMinutes(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AISession{}, &schema.AIReActSchedule{}).Error)

	const sessionID = "chat-schedule-five-minutes"
	_, err = yakit.CreateOrUpdateAISessionMetaStartParams(db, sessionID, &ypb.AIStartParams{
		UseDefaultAIConfig: true,
		ReviewPolicy:       "manual",
	})
	require.NoError(t, err)

	location, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Now().In(location).Add(time.Hour).Truncate(time.Second)
	tool, err := createScheduleTool(func() time.Time { return now })
	require.NoError(t, err)
	require.False(t, tool.NoNeedUserReview)

	result, err := tool.Callback(context.Background(), aitool.InvokeParams{
		"name":        "香年广场天气",
		"task_prompt": "查询四川省成都市香年广场的最新天气情况",
		"rrule":       "RRULE:FREQ=MINUTELY;INTERVAL=5",
		"timezone":    "Asia/Shanghai",
	}, &aitool.ToolRuntimeConfig{
		ProjectDatabase:      db,
		PersistentSessionID:  sessionID,
		CurrentTaskUserInput: "每5分钟帮我查询四川省成都市香年广场的最新天气情况",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	require.NoError(t, err)
	require.Equal(t, true, result.(map[string]any)["created"])

	record := &schema.AIReActSchedule{}
	require.NoError(t, db.First(record).Error)
	require.Equal(t, "香年广场天气", record.Name)
	require.Equal(t, "查询四川省成都市香年广场的最新天气情况", record.Prompt)
	require.Equal(t, "RRULE:FREQ=MINUTELY;INTERVAL=5", record.RRule)
	require.Equal(t, sessionID, record.TargetSessionID)
	require.Equal(t, schema.AIReActScheduleTargetContinueSession, record.TargetMode)
	require.Equal(t, sessionID, record.CreatedFromSessionID)
	require.Equal(t, "每5分钟帮我查询四川省成都市香年广场的最新天气情况", record.OriginalRequest)
	require.Equal(t, now.Add(5*time.Minute).Unix(), record.StartAt.Unix())
	require.NotNil(t, record.NextRunAt)
	require.Equal(t, record.StartAt.Unix(), record.NextRunAt.Unix())

	startParams, err := yakit.UnmarshalAISessionStartParams(record.StartParams)
	require.NoError(t, err)
	require.Equal(t, "yolo", startParams.GetReviewPolicy())
	require.True(t, startParams.GetDisallowRequireForUserPrompt())
	require.False(t, startParams.GetAllowPlanUserInteract())
}

func TestCreateScheduleToolRequiresPersistentSessionForContinueMode(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIReActSchedule{}).Error)
	tool, err := createScheduleTool(time.Now)
	require.NoError(t, err)

	_, err = tool.Callback(context.Background(), aitool.InvokeParams{
		"name":        "test",
		"task_prompt": "do work",
		"rrule":       "RRULE:FREQ=HOURLY;INTERVAL=1",
	}, &aitool.ToolRuntimeConfig{ProjectDatabase: db}, &bytes.Buffer{}, &bytes.Buffer{})
	require.ErrorContains(t, err, "no persistent session id")
}
