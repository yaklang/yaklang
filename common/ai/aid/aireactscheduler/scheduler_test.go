package aireactscheduler

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func newSchedulerTest(t *testing.T) (*Scheduler, sessionruntime.ReActSessionRuntime, *gorm.DB) {
	t.Helper()
	db, err := consts.CreateProjectDatabase(filepath.Join(t.TempDir(), "schedule.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.AIReActSchedule{}, &schema.AISession{}).Error)
	runtime := sessionruntime.New(func() *gorm.DB { return db })
	scheduler := NewScheduler(runtime, db)
	t.Cleanup(func() {
		scheduler.Stop()
		require.NoError(t, db.Close())
	})
	return scheduler, runtime, db
}

func TestSchedulerConcurrentStartAndStop(t *testing.T) {
	scheduler := NewScheduler(sessionruntime.New(nil), nil)
	var callers sync.WaitGroup
	for i := 0; i < 32; i++ {
		callers.Add(2)
		go func() {
			defer callers.Done()
			scheduler.Start()
		}()
		go func() {
			defer callers.Done()
			scheduler.Stop()
		}()
	}
	callers.Wait()
	scheduler.Stop()
	require.ErrorIs(t, scheduler.ctx.Err(), context.Canceled)
}

func TestAIReActSchedulerSkipsMisfireAndAdvances(t *testing.T) {
	manager, _, db := newSchedulerTest(t)
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
	require.NoError(t, db.Create(schedule).Error)
	manager.dispatchDue()

	manager.jobsMu.Lock()
	require.Empty(t, manager.jobs)
	require.Empty(t, manager.activeBySchedule)
	manager.jobsMu.Unlock()

	updated, err := aischedule.GetRecord(db, schedule.UUID)
	require.NoError(t, err)
	require.NotNil(t, updated.NextRunAt)
	require.True(t, updated.NextRunAt.After(now))
	require.Equal(t, scheduledOutcomeSkipped, updated.LastOutcome)
	require.Equal(t, "misfire", updated.LastSkipReason)
}

func TestAIReActSchedulerSkipsAtTriggerBoundary(t *testing.T) {
	manager, runtime, db := newSchedulerTest(t)
	const sessionID = "starting-user-session"
	_, err := yakit.CreateOrUpdateAISessionMetaStartParams(db, sessionID, &ypb.AIStartParams{})
	require.NoError(t, err)
	reservation, err := runtime.ReserveSession(context.Background(), sessionID, "frontend-start")
	require.NoError(t, err)
	defer reservation.Release()

	schedule := &schema.AIReActSchedule{
		UUID: "busy-boundary", TargetMode: schema.AIReActScheduleTargetContinueSession, TargetSessionID: sessionID,
		Status: schema.AIReActScheduleStatusActive,
	}
	require.NoError(t, db.Create(schedule).Error)
	err = manager.Enqueue(schedule, time.Now(), TriggerSchedule)
	var skip *scheduleEnqueueError
	require.ErrorAs(t, err, &skip)
	require.Equal(t, "session_busy", skip.reason)
	require.Empty(t, manager.jobs)
}

func TestAIReActSchedulerUsesBoundedParallelCapacity(t *testing.T) {
	manager, _, db := newSchedulerTest(t)
	for i := 0; i < aiReActScheduleMaxConcurrent; i++ {
		manager.worker <- struct{}{}
	}
	schedule := &schema.AIReActSchedule{
		UUID: "over-capacity", TargetMode: schema.AIReActScheduleTargetNewSession,
		Status: schema.AIReActScheduleStatusActive,
	}
	require.NoError(t, db.Create(schedule).Error)
	err := manager.Enqueue(schedule, time.Now(), TriggerSchedule)
	var skip *scheduleEnqueueError
	require.ErrorAs(t, err, &skip)
	require.Equal(t, "scheduler_capacity", skip.reason)
}

func TestAIReActSchedulerRejectsEnqueueAfterStop(t *testing.T) {
	manager, _, db := newSchedulerTest(t)
	manager.cancel()

	schedule, err := aischedule.BuildRecord(&ypb.AIReActSchedule{
		Name:       "stopped",
		Status:     schema.AIReActScheduleStatusActive,
		TargetMode: schema.AIReActScheduleTargetNewSession,
		Payload: &ypb.AIReActSchedulePayload{
			Prompt:      "test",
			StartParams: &ypb.AIStartParams{UseDefaultAIConfig: true},
		},
		Schedule: &ypb.AIReActScheduleSpec{
			RRule:    "RRULE:FREQ=DAILY;INTERVAL=1",
			Timezone: "UTC",
			StartAt:  time.Now().Add(time.Hour).Unix(),
		},
	}, nil)
	require.NoError(t, err)
	require.NoError(t, db.Create(schedule).Error)
	err = manager.Enqueue(schedule, time.Now(), TriggerSchedule)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, manager.jobs)
	require.Empty(t, manager.activeBySchedule)
	require.Empty(t, manager.worker)
}

func TestScheduledReActRuntimeErrorPreservesContextClassification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	outcome := scheduledReActRuntimeError(ctx, context.Canceled)
	require.Empty(t, outcome.status)
	require.Empty(t, outcome.errorMessage)

	outcome = scheduledReActRuntimeError(context.Background(), context.Canceled)
	require.Equal(t, scheduledOutcomeFailed, outcome.status)
	require.ErrorContains(t, context.Canceled, outcome.errorMessage)
}

func TestAIReActSchedulerCancelsActiveExecutionInMemory(t *testing.T) {
	manager, runtime, _ := newSchedulerTest(t)
	jobCtx, cancel := context.WithCancel(manager.ctx)
	reservation, err := runtime.ReserveSession(jobCtx, "ai-schedule-active-execution", "active-execution")
	require.NoError(t, err)
	job := &scheduledReActJob{
		executionID:  "active-execution",
		scheduleUUID: "schedule",
		sessionID:    "ai-schedule-active-execution",
		ctx:          reservation.Context(),
		cancel:       cancel,
		reservation:  reservation,
		done:         make(chan struct{}),
	}
	manager.jobs[job.executionID] = job
	manager.activeBySchedule[job.scheduleUUID] = job.executionID
	require.True(t, runtime.IsSessionBusy(job.sessionID))

	manager.CancelSchedule(job.scheduleUUID)
	require.Eventually(t, func() bool { return job.ctx.Err() == context.Canceled }, time.Second, 10*time.Millisecond)
	manager.unregisterJob(job)
	require.Empty(t, manager.jobs)
	require.Empty(t, manager.activeBySchedule)
	require.False(t, runtime.IsSessionBusy(job.sessionID))
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
