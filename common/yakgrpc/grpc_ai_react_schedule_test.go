package yakgrpc

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/ai/aid/aireactscheduler"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/ai/aid/reactservice"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type connectOverrideRuntime struct {
	sessionruntime.ReActSessionRuntime
	connect func(context.Context, sessionruntime.ConnectRequest, sessionruntime.ReActEventHandler) (sessionruntime.ReActConnection, error)
}

func (r *connectOverrideRuntime) Connect(ctx context.Context, req sessionruntime.ConnectRequest, onEvent sessionruntime.ReActEventHandler) (sessionruntime.ReActConnection, error) {
	return r.connect(ctx, req, onEvent)
}

func newScheduleTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := consts.CreateProjectDatabase(filepath.Join(t.TempDir(), "schedule.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIReActSchedule{}, &schema.AISession{}).Error)
	server := &Server{projectDatabase: db}
	t.Cleanup(func() {
		server.stopAIReActScheduler()
		require.NoError(t, db.Close())
	})
	return server
}

func validScheduleRequest(startAt time.Time) *ypb.CreateAIReActScheduleRequest {
	return &ypb.CreateAIReActScheduleRequest{Schedule: &ypb.AIReActSchedule{
		Name:            "daily summary",
		Status:          schema.AIReActScheduleStatusActive,
		TargetMode:      schema.AIReActScheduleTargetNewSession,
		OriginalRequest: "每天汇总安全发现",
		Payload: &ypb.AIReActSchedulePayload{
			Prompt:      "summarize today's security findings",
			StartParams: &ypb.AIStartParams{UseDefaultAIConfig: true, ReviewPolicy: "ai"},
		},
		Schedule: &ypb.AIReActScheduleSpec{
			RRule:    "RRULE:FREQ=DAILY;INTERVAL=1",
			Timezone: "Asia/Shanghai",
			StartAt:  startAt.Unix(),
		},
	}}
}

func TestAIReActScheduleCRUD(t *testing.T) {
	server := newScheduleTestServer(t)
	ctx := context.Background()
	created, err := server.CreateAIReActSchedule(ctx, validScheduleRequest(time.Now().Add(time.Hour)))
	require.NoError(t, err)
	require.NotEmpty(t, created.GetUUID())
	require.Equal(t, "yolo", created.GetPayload().GetStartParams().GetReviewPolicy())
	require.True(t, created.GetPayload().GetStartParams().GetDisallowRequireForUserPrompt())
	require.Equal(t, "ai", created.GetPayload().GetStartParams().GetSource())
	require.Greater(t, created.GetNextRunAt(), time.Now().Unix())
	require.Equal(t, "每天汇总安全发现", created.GetOriginalRequest())

	queried, err := server.QueryAIReActSchedules(ctx, &ypb.QueryAIReActSchedulesRequest{})
	require.NoError(t, err)
	require.Equal(t, int64(1), queried.GetTotal())
	require.Len(t, queried.GetData(), 1)
	var executionCancelled atomic.Bool
	unregisterExecution := aischedule.RegisterExecution(created.GetUUID(), func() {
		executionCancelled.Store(true)
	})
	defer unregisterExecution()

	paused, err := server.SetAIReActScheduleEnabled(ctx, &ypb.SetAIReActScheduleEnabledRequest{UUID: created.GetUUID(), Enabled: false})
	require.NoError(t, err)
	require.Equal(t, schema.AIReActScheduleStatusPaused, paused.GetStatus())
	require.False(t, executionCancelled.Load(), "pausing must not cancel an execution that already started")

	deleted, err := server.DeleteAIReActSchedule(ctx, &ypb.DeleteAIReActScheduleRequest{UUID: created.GetUUID()})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted.GetEffectRows())
	require.False(t, executionCancelled.Load(), "deleting must not cancel an execution that already started")
}

func TestAIReActScheduleForcesUnattendedReviewPolicy(t *testing.T) {
	request := validScheduleRequest(time.Now().Add(time.Hour))
	request.Schedule.Payload.StartParams.ReviewPolicy = "manual"
	record, err := buildScheduleRecord(request.GetSchedule(), nil)
	require.NoError(t, err)
	payload, err := unmarshalSchedulePayload(record)
	require.NoError(t, err)
	require.Equal(t, "yolo", payload.GetStartParams().GetReviewPolicy())
}

func TestAIReActScheduleSupportsBothSessionTargets(t *testing.T) {
	request := validScheduleRequest(time.Now().Add(time.Hour))
	request.Schedule.TargetMode = schema.AIReActScheduleTargetContinueSession
	request.Schedule.TargetSessionID = "busy-user-session"
	record, err := buildScheduleRecord(request.GetSchedule(), nil)
	require.NoError(t, err)
	require.Equal(t, schema.AIReActScheduleTargetContinueSession, record.TargetMode)
	require.Equal(t, "busy-user-session", record.TargetSessionID)

	const runUUID = "17ea0eb4-acde-40c1-965e-3661c62347f2"
	require.Equal(t, "busy-user-session", aireactscheduler.ScheduleExecutionSessionID(record, runUUID))
	require.Equal(t, "ai-schedule-"+runUUID, aireactscheduler.ScheduleExecutionSessionID(&schema.AIReActSchedule{
		TargetMode: schema.AIReActScheduleTargetNewSession,
	}, runUUID))
}

func TestPrepareIsolatedScheduleSessionIsImmediatelyQueryable(t *testing.T) {
	server := newScheduleTestServer(t)
	const sessionID = "ai-schedule-running-session"
	startedAt := time.Now().Add(-time.Second).Truncate(time.Second)
	params := &ypb.AIStartParams{TimelineSessionID: sessionID, Source: "ai", ReviewPolicy: "yolo"}
	schedule := &schema.AIReActSchedule{
		Name:       "成都香年广场天气查询",
		Prompt:     "查询四川省成都市香年广场的最新天气情况",
		TargetMode: schema.AIReActScheduleTargetNewSession,
	}

	require.NoError(t, aireactscheduler.PrepareIsolatedScheduleSession(
		server.GetProjectDatabase(),
		schedule,
		params,
		sessionID,
		startedAt,
	))
	runtime := server.getReActSessionRuntime()
	reservation, err := runtime.ReserveSession(context.Background(), sessionID, "running-execution")
	require.NoError(t, err)

	response, err := server.QueryAISession(context.Background(), &ypb.QueryAISessionRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "last_used_at", Order: "desc"},
		Filter:     &ypb.AISessionFilter{Source: []string{"ai", ""}},
	})
	require.NoError(t, err)
	require.Len(t, response.GetData(), 1)
	visible := response.GetData()[0]
	require.Equal(t, sessionID, visible.GetSessionID())
	require.Equal(t, schedule.Name, visible.GetTitle())
	require.True(t, visible.GetTitleInitialized())
	require.Equal(t, "ai", visible.GetSource())
	require.Equal(t, startedAt.Unix(), visible.GetLastUsedAt())
	require.True(t, visible.GetIsRunning())

	reservation.Release()
	response, err = server.QueryAISession(context.Background(), &ypb.QueryAISessionRequest{
		Pagination: &ypb.Paging{Page: 1, Limit: 10},
		Filter:     &ypb.AISessionFilter{SessionID: []string{sessionID}},
	})
	require.NoError(t, err)
	require.Len(t, response.GetData(), 1)
	require.False(t, response.GetData()[0].GetIsRunning())
}

func TestContinueSessionScheduleKeepsChatReviewPolicy(t *testing.T) {
	server := newScheduleTestServer(t)
	const sessionID = "continue-session-policy"
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(server.GetProjectDatabase(), sessionID, &ypb.AIStartParams{
		ReviewPolicy:              "manual",
		UserQuery:                 "old user query",
		Attach:                    true,
		PreferSessionCachedConfig: true,
	})
	require.NoError(t, err)

	params, err := aireactscheduler.ScheduleRunStartParams(server.GetProjectDatabase(), &schema.AIReActSchedule{
		TargetMode:      schema.AIReActScheduleTargetContinueSession,
		TargetSessionID: sessionID,
	}, &ypb.AIStartParams{ReviewPolicy: "yolo"})
	require.NoError(t, err)
	require.Equal(t, "manual", params.GetReviewPolicy(), "attaching a schedule must not persist YOLO as the chat default")
	require.Empty(t, params.GetUserQuery())
	require.False(t, params.GetAttach())
	require.False(t, params.GetPreferSessionCachedConfig())

	isolated, err := aireactscheduler.ScheduleRunStartParams(server.GetProjectDatabase(), &schema.AIReActSchedule{
		TargetMode: schema.AIReActScheduleTargetNewSession,
	}, &ypb.AIStartParams{ReviewPolicy: "manual"})
	require.NoError(t, err)
	require.Equal(t, "yolo", isolated.GetReviewPolicy())
}

func TestPreviewAIReActScheduleTimes(t *testing.T) {
	server := newScheduleTestServer(t)
	start := time.Now().Add(time.Hour).Truncate(time.Second)
	response, err := server.PreviewAIReActScheduleTimes(context.Background(), &ypb.PreviewAIReActScheduleTimesRequest{
		Schedule: &ypb.AIReActScheduleSpec{RRule: "RRULE:FREQ=HOURLY;COUNT=3", Timezone: "UTC", StartAt: start.Unix()},
		Count:    3,
	})
	require.NoError(t, err)
	require.Len(t, response.GetTimestamps(), 3)
	require.Equal(t, start.Unix(), response.GetTimestamps()[0])
}

func TestAIReActScheduleFiltersAndSessionLifecycle(t *testing.T) {
	server := newScheduleTestServer(t)
	db := server.GetProjectDatabase()
	const sessionID = "schedule-owner-session"
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(db, sessionID, &ypb.AIStartParams{UseDefaultAIConfig: true})
	require.NoError(t, err)

	start := time.Now().Add(time.Hour)
	attachedRequest := validScheduleRequest(start)
	attachedRequest.Schedule.TargetMode = schema.AIReActScheduleTargetContinueSession
	attachedRequest.Schedule.TargetSessionID = sessionID
	attachedRequest.Schedule.CreatedFromSessionID = sessionID
	attached, err := server.CreateAIReActSchedule(context.Background(), attachedRequest)
	require.NoError(t, err)

	isolatedRequest := validScheduleRequest(start)
	isolatedRequest.Schedule.CreatedFromSessionID = sessionID
	isolated, err := server.CreateAIReActSchedule(context.Background(), isolatedRequest)
	require.NoError(t, err)

	filtered, err := server.QueryAIReActSchedules(context.Background(), &ypb.QueryAIReActSchedulesRequest{
		Filter: &ypb.AIReActScheduleFilter{TargetSessionIDs: []string{sessionID}},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), filtered.GetTotal())
	require.Equal(t, attached.GetUUID(), filtered.GetData()[0].GetUUID())

	deleted, err := yakit.DeleteAttachedAIReActSchedules(db, []string{sessionID})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	_, err = getAIReActScheduleRecord(db, attached.GetUUID())
	require.Error(t, err)
	remaining, err := getAIReActScheduleRecord(db, isolated.GetUUID())
	require.NoError(t, err)
	require.Equal(t, schema.AIReActScheduleTargetNewSession, remaining.TargetMode)
}

func TestDeleteRunningScheduleSessionReleasesBeforeRunNow(t *testing.T) {
	server := newScheduleTestServer(t)
	db := server.GetProjectDatabase()
	require.NoError(t, db.AutoMigrate(
		&schema.AIAgentRuntime{},
		&schema.AiCheckpoint{},
		&schema.AiOutputEvent{},
		&schema.AiProcessAndAiEvent{},
		&schema.AISessionPlanAndExec{},
	).Error)

	executionCancelled := make(chan struct{})
	allowShutdown := make(chan struct{})
	var cancellationReported atomic.Bool
	var shutdownReleased atomic.Bool
	releaseShutdown := func() {
		if shutdownReleased.CompareAndSwap(false, true) {
			close(allowShutdown)
		}
	}
	defer releaseShutdown()
	baseRuntime := sessionruntime.New(server.GetProjectDatabase)
	overrideRuntime := &connectOverrideRuntime{ReActSessionRuntime: baseRuntime}
	overrideRuntime.connect = func(ctx context.Context, _ sessionruntime.ConnectRequest, _ sessionruntime.ReActEventHandler) (sessionruntime.ReActConnection, error) {
		<-ctx.Done()
		if cancellationReported.CompareAndSwap(false, true) {
			close(executionCancelled)
		}
		<-allowShutdown
		return nil, ctx.Err()
	}
	service := reactservice.New(server.GetProjectDatabase, reactservice.WithRuntime(overrideRuntime))
	server.reActService = service
	service.StartScheduler()
	defer service.StopScheduler()

	created, err := server.CreateAIReActSchedule(context.Background(), validScheduleRequest(time.Now().Add(time.Hour)))
	require.NoError(t, err)
	_, err = server.RunAIReActScheduleNow(context.Background(), &ypb.RunAIReActScheduleNowRequest{UUID: created.GetUUID()})
	require.NoError(t, err)

	var firstJob aireactscheduler.ActiveExecution
	require.Eventually(t, func() bool {
		var ok bool
		firstJob, ok = service.ActiveExecution(created.GetUUID())
		return ok
	}, time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		_, queryErr := yakit.GetAISessionMetaBySessionID(db, firstJob.SessionID)
		return queryErr == nil
	}, time.Second, 10*time.Millisecond)

	deleteDone := make(chan error, 1)
	go func() {
		_, deleteErr := server.DeleteAISession(context.Background(), &ypb.DeleteAISessionRequest{
			Filter: &ypb.DeleteAISessionFilter{SessionID: []string{firstJob.SessionID}},
		})
		deleteDone <- deleteErr
	}()
	select {
	case <-executionCancelled:
	case <-time.After(time.Second):
		t.Fatal("deleting the session did not cancel its scheduled execution")
	}
	select {
	case deleteErr := <-deleteDone:
		t.Fatalf("DeleteAISession returned before the scheduler handler exited: %v", deleteErr)
	default:
	}

	releaseShutdown()
	select {
	case err = <-deleteDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("DeleteAISession did not finish after scheduler shutdown")
	}
	select {
	case <-firstJob.Done:
	default:
		t.Fatal("DeleteAISession returned before the scheduler job unregistered")
	}
	_, active := service.ActiveExecution(created.GetUUID())
	require.False(t, active)
	require.False(t, baseRuntime.IsSessionBusy(firstJob.SessionID))

	// Reproduce the UI sequence from the bug report: delete the running
	// independent session and immediately run the same schedule again.
	_, err = server.RunAIReActScheduleNow(context.Background(), &ypb.RunAIReActScheduleNowRequest{UUID: created.GetUUID()})
	require.NoError(t, err)
	var secondJob aireactscheduler.ActiveExecution
	require.Eventually(t, func() bool {
		var ok bool
		secondJob, ok = service.ActiveExecution(created.GetUUID())
		return ok && secondJob.ExecutionID != firstJob.ExecutionID
	}, time.Second, 10*time.Millisecond)
	service.CancelSchedule(created.GetUUID())
	select {
	case <-secondJob.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement scheduled execution did not stop")
	}
}
