package yakgrpc

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

	paused, err := server.SetAIReActScheduleEnabled(ctx, &ypb.SetAIReActScheduleEnabledRequest{UUID: created.GetUUID(), Enabled: false})
	require.NoError(t, err)
	require.Equal(t, schema.AIReActScheduleStatusPaused, paused.GetStatus())

	deleted, err := server.DeleteAIReActSchedule(ctx, &ypb.DeleteAIReActScheduleRequest{UUID: created.GetUUID()})
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted.GetEffectRows())
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
	require.Equal(t, "busy-user-session", scheduleExecutionSessionID(record, runUUID))
	require.Equal(t, "ai-schedule-"+runUUID, scheduleExecutionSessionID(&schema.AIReActSchedule{
		TargetMode: schema.AIReActScheduleTargetNewSession,
	}, runUUID))

	legacy, err := scheduleToGRPC(&schema.AIReActSchedule{
		UUID:            "legacy-schedule",
		Name:            "legacy",
		Status:          schema.AIReActScheduleStatusActive,
		TargetMode:      schema.AIReActScheduleTargetContinueSession,
		TargetSessionID: "busy-user-session",
		StartParams:     `{"ReviewPolicy":"ai"}`,
	})
	require.NoError(t, err)
	require.Equal(t, schema.AIReActScheduleTargetContinueSession, legacy.GetTargetMode())
	require.Equal(t, "busy-user-session", legacy.GetTargetSessionID())
	require.Equal(t, "yolo", legacy.GetPayload().GetStartParams().GetReviewPolicy())
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

	params, err := scheduleRunStartParams(server.GetProjectDatabase(), &schema.AIReActSchedule{
		TargetMode:      schema.AIReActScheduleTargetContinueSession,
		TargetSessionID: sessionID,
	}, &ypb.AIStartParams{ReviewPolicy: "yolo"})
	require.NoError(t, err)
	require.Equal(t, "manual", params.GetReviewPolicy(), "attaching a schedule must not persist YOLO as the chat default")
	require.Empty(t, params.GetUserQuery())
	require.False(t, params.GetAttach())
	require.False(t, params.GetPreferSessionCachedConfig())

	isolated, err := scheduleRunStartParams(server.GetProjectDatabase(), &schema.AIReActSchedule{
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

func TestAIReActSchedulerSkipsMisfireAndAdvances(t *testing.T) {
	server := newScheduleTestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(-10 * time.Minute)
	schedule := &schema.AIReActSchedule{
		UUID:                  "misfire-schedule",
		Name:                  "misfire",
		Status:                schema.AIReActScheduleStatusActive,
		TargetMode:            schema.AIReActScheduleTargetNewSession,
		Prompt:                "test",
		StartParams:           `{}`,
		AttachedResourceInfos: `[]`,
		RRule:                 "RRULE:FREQ=HOURLY;INTERVAL=1",
		Timezone:              "UTC",
		StartAt:               now.Add(-time.Hour),
		NextRunAt:             &due,
		MisfireGraceSeconds:   1,
		MaxRuntimeSeconds:     60,
	}
	require.NoError(t, server.GetProjectDatabase().Create(schedule).Error)
	manager := newAIReActScheduler(server, server.GetProjectDatabase())
	defer manager.cancel()
	manager.dispatchDue()

	manager.jobsMu.Lock()
	require.Empty(t, manager.jobs)
	require.Empty(t, manager.activeBySchedule)
	manager.jobsMu.Unlock()

	updated, err := getAIReActScheduleRecord(server.GetProjectDatabase(), schedule.UUID)
	require.NoError(t, err)
	require.NotNil(t, updated.NextRunAt)
	require.True(t, updated.NextRunAt.After(now))
	require.Equal(t, scheduledOutcomeSkipped, updated.LastOutcome)
	require.Equal(t, "misfire", updated.LastSkipReason)
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

func TestCleanupOrphanedAttachedAIReActSchedules(t *testing.T) {
	server := newScheduleTestServer(t)
	db := server.GetProjectDatabase()
	attached := &schema.AIReActSchedule{
		UUID: "orphan-attached", Name: "orphan", Status: schema.AIReActScheduleStatusActive,
		TargetMode: schema.AIReActScheduleTargetContinueSession, TargetSessionID: "missing-session", Prompt: "work",
	}
	isolated := &schema.AIReActSchedule{
		UUID: "orphan-isolated", Name: "isolated", Status: schema.AIReActScheduleStatusActive,
		TargetMode: schema.AIReActScheduleTargetNewSession, CreatedFromSessionID: "missing-session", Prompt: "work",
	}
	require.NoError(t, db.Create(attached).Error)
	require.NoError(t, db.Create(isolated).Error)

	cleanupOrphanedAttachedAIReActSchedules(db)
	_, err := getAIReActScheduleRecord(db, attached.UUID)
	require.Error(t, err)
	_, err = getAIReActScheduleRecord(db, isolated.UUID)
	require.NoError(t, err, "isolated schedules do not depend on their creation chat")
}

func TestAIReActSchedulerSkipsAtTriggerBoundary(t *testing.T) {
	server := newScheduleTestServer(t)
	manager := newAIReActScheduler(server, server.GetProjectDatabase())
	defer manager.cancel()
	const sessionID = "starting-user-session"
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(server.GetProjectDatabase(), sessionID, &ypb.AIStartParams{})
	require.NoError(t, err)
	release, ok := aireact.TryBeginSessionStart(sessionID)
	require.True(t, ok)
	defer release()

	schedule := &schema.AIReActSchedule{
		UUID: "busy-boundary", TargetMode: schema.AIReActScheduleTargetContinueSession, TargetSessionID: sessionID,
		Status: schema.AIReActScheduleStatusActive,
	}
	require.NoError(t, server.GetProjectDatabase().Create(schedule).Error)
	err = manager.enqueue(schedule, time.Now(), aiReActScheduleTriggerSchedule)
	var skip *scheduleEnqueueError
	require.ErrorAs(t, err, &skip)
	require.Equal(t, "session_busy", skip.reason)
	require.Empty(t, manager.jobs)
}

func TestAIReActSchedulerUsesBoundedParallelCapacity(t *testing.T) {
	server := newScheduleTestServer(t)
	manager := newAIReActScheduler(server, server.GetProjectDatabase())
	defer manager.cancel()
	for i := 0; i < aiReActScheduleMaxConcurrent; i++ {
		manager.worker <- struct{}{}
	}
	schedule := &schema.AIReActSchedule{
		UUID: "over-capacity", TargetMode: schema.AIReActScheduleTargetNewSession,
		Status: schema.AIReActScheduleStatusActive,
	}
	require.NoError(t, server.GetProjectDatabase().Create(schedule).Error)
	err := manager.enqueue(schedule, time.Now(), aiReActScheduleTriggerSchedule)
	var skip *scheduleEnqueueError
	require.ErrorAs(t, err, &skip)
	require.Equal(t, "scheduler_capacity", skip.reason)
}

func TestDropLegacyAIReActScheduleRuns(t *testing.T) {
	server := newScheduleTestServer(t)
	db := server.GetProjectDatabase()
	require.NoError(t, db.Exec(`CREATE TABLE ai_react_schedule_runs_v1 (id INTEGER PRIMARY KEY, status TEXT)`).Error)
	require.True(t, db.HasTable(legacyAIReActRunTable))
	dropLegacyAIReActScheduleRuns(db)
	require.False(t, db.HasTable(legacyAIReActRunTable))
}

func TestAIReActSchedulerCancelsActiveExecutionInMemory(t *testing.T) {
	server := newScheduleTestServer(t)
	manager := newAIReActScheduler(server, server.GetProjectDatabase())
	defer manager.cancel()

	jobCtx, cancel := context.WithCancel(manager.ctx)
	job := &scheduledReActJob{
		executionID:  "active-execution",
		scheduleUUID: "schedule",
		ctx:          jobCtx,
		cancel:       cancel,
	}
	manager.jobs[job.executionID] = job
	manager.activeBySchedule[job.scheduleUUID] = job.executionID

	manager.cancelSchedule(job.scheduleUUID)
	require.Eventually(t, func() bool { return job.ctx.Err() == context.Canceled }, time.Second, 10*time.Millisecond)
	manager.unregisterJob(job)
	require.Empty(t, manager.jobs)
	require.Empty(t, manager.activeBySchedule)
}

func TestScheduleAttentionWaitsForActualAIEscalation(t *testing.T) {
	pending := make(map[string]struct{})
	reviewRequest := &ypb.AIOutputEvent{
		Type:    string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE),
		Content: json.RawMessage(`{"id":"review-1"}`),
	}
	attention, _ := scheduleAttentionForEvent(reviewRequest, true, pending)
	require.False(t, attention, "starting AI review must not pause a scheduled run")
	require.Contains(t, pending, "review-1")

	autoApproved := &ypb.AIOutputEvent{
		Type:    string(schema.EVENT_TYPE_AI_REVIEW_END),
		Content: json.RawMessage(`{"interactive_id":"review-1","level":"low","requires_user":false}`),
	}
	attention, _ = scheduleAttentionForEvent(autoApproved, true, pending)
	require.False(t, attention, "low-risk AI review should continue unattended")
	require.NotContains(t, pending, "review-1")

	attention, _ = scheduleAttentionForEvent(&ypb.AIOutputEvent{
		Type:    string(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE),
		Content: json.RawMessage(`{"id":"review-2"}`),
	}, true, pending)
	require.False(t, attention)

	attention, message := scheduleAttentionForEvent(&ypb.AIOutputEvent{
		Type: string(schema.EVENT_TYPE_AI_REVIEW_END),
		Content: json.RawMessage(
			`{"interactive_id":"review-2","level":"high","requires_user":true,"reason":"dangerous operation"}`,
		),
	}, true, pending)
	require.True(t, attention, "only an explicit high-risk escalation should need the user")
	require.Contains(t, message, "dangerous operation")
}

func TestScheduleAttentionForExplicitUserInteraction(t *testing.T) {
	attention, message := scheduleAttentionForEvent(&ypb.AIOutputEvent{
		Type: string(schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE),
	}, true, make(map[string]struct{}))
	require.True(t, attention)
	require.Contains(t, message, "user interaction")
}
