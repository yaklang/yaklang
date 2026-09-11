package aireactscheduler

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

const (
	aiReActSchedulePollInterval  = 30 * time.Second
	aiReActScheduleMaxConcurrent = 3

	TriggerSchedule = "schedule"
	TriggerManual   = "manual"

	scheduledOutcomeSucceeded      = "succeeded"
	scheduledOutcomeFailed         = "failed"
	scheduledOutcomeSkipped        = "skipped"
	scheduledOutcomeCancelled      = "cancelled"
	scheduledOutcomeInterrupted    = "interrupted"
	scheduledOutcomeNeedsAttention = "needs_attention"
)

type scheduledReActJob struct {
	executionID         string
	scheduleUUID        string
	sessionID           string
	scheduledAt         time.Time
	trigger             string
	ctx                 context.Context
	cancel              context.CancelFunc
	unregisterExecution func()
	reservation         sessionruntime.SessionReservation
	workerReserved      bool
	done                chan struct{}
}

type scheduleEnqueueError struct {
	reason  string
	message string
}

func (e *scheduleEnqueueError) Error() string { return e.message }

type Scheduler struct {
	runtime sessionruntime.ReActSessionRuntime
	db      *gorm.DB

	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{}
	worker chan struct{}

	lifecycleMu      sync.Mutex
	started          bool
	jobsMu           sync.Mutex
	jobs             map[string]*scheduledReActJob
	activeBySchedule map[string]string
	wg               sync.WaitGroup
}

// ActiveExecution is an immutable view of one in-memory scheduled run.
type ActiveExecution struct {
	ExecutionID string
	SessionID   string
	Done        <-chan struct{}
}

func NewScheduler(runtime sessionruntime.ReActSessionRuntime, db *gorm.DB) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		runtime:          runtime,
		db:               db,
		ctx:              ctx,
		cancel:           cancel,
		wake:             make(chan struct{}, 1),
		worker:           make(chan struct{}, aiReActScheduleMaxConcurrent),
		jobs:             make(map[string]*scheduledReActJob),
		activeBySchedule: make(map[string]string),
	}
}

// Start begins the project-scoped polling loop. A Scheduler is started once;
// callers create a new instance when the active project changes.
func (m *Scheduler) Start() {
	if m == nil {
		return
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.started || m.ctx.Err() != nil {
		return
	}
	// Add while holding the same lifecycle lock used by Stop. This prevents a
	// concurrent Stop from observing a zero WaitGroup and returning immediately
	// before Start publishes the polling goroutine.
	m.started = true
	m.wg.Add(1)
	go m.loop()
}

func (m *Scheduler) Notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Stop waits for the polling loop and all in-memory executions to release
// their resources.
func (m *Scheduler) Stop() {
	if m == nil {
		return
	}
	m.lifecycleMu.Lock()
	m.cancel()
	m.lifecycleMu.Unlock()
	m.jobsMu.Lock()
	for _, job := range m.jobs {
		job.cancel()
	}
	m.jobsMu.Unlock()
	m.wg.Wait()
}

func (m *Scheduler) loop() {
	defer m.wg.Done()
	ticker := time.NewTicker(aiReActSchedulePollInterval)
	defer ticker.Stop()
	for {
		m.dispatchDue()
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		case <-m.wake:
		}
	}
}

func (m *Scheduler) dispatchDue() {
	if m == nil || m.db == nil || m.ctx.Err() != nil {
		return
	}
	now := time.Now().UTC()
	var due []*schema.AIReActSchedule
	err := m.db.Where("status = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", schema.AIReActScheduleStatusActive, now).
		Order("next_run_at ASC").Limit(32).Find(&due).Error
	if err != nil {
		log.Errorf("query due AI ReAct schedules failed: %v", err)
		return
	}
	for _, schedule := range due {
		if schedule == nil || schedule.NextRunAt == nil {
			continue
		}
		occurrence := schedule.NextRunAt.UTC()
		advanced, err := m.advanceSchedule(schedule, occurrence, now)
		if err != nil {
			log.Errorf("advance AI ReAct schedule %s failed: %v", schedule.UUID, err)
			continue
		}
		if !advanced {
			continue
		}
		if schedule.MisfireGraceSeconds > 0 && now.Sub(occurrence) > time.Duration(schedule.MisfireGraceSeconds)*time.Second {
			log.Infof("skip misfired AI ReAct schedule %s occurrence %s", schedule.UUID, occurrence.Format(time.RFC3339))
			m.recordScheduleSkipped(schedule.UUID, "misfire", "scheduled occurrence exceeded its misfire grace period")
			continue
		}
		if err := m.Enqueue(schedule, occurrence, TriggerSchedule); err != nil {
			if skip, ok := err.(*scheduleEnqueueError); ok {
				if skip.reason != "schedule_inactive" {
					m.recordScheduleSkipped(schedule.UUID, skip.reason, skip.message)
				}
				log.Infof("skip AI ReAct schedule %s occurrence %s: %s", schedule.UUID, occurrence.Format(time.RFC3339), skip.message)
			} else {
				log.Errorf("enqueue AI ReAct schedule %s failed: %v", schedule.UUID, err)
			}
		}
	}
}

func (m *Scheduler) advanceSchedule(schedule *schema.AIReActSchedule, occurrence, now time.Time) (bool, error) {
	rule, err := aischedule.Parse(schedule.RRule, schedule.Timezone, schedule.StartAt)
	if err != nil {
		result := m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ? AND status = ?", schedule.UUID, schema.AIReActScheduleStatusActive).Updates(map[string]any{
			"status":       schema.AIReActScheduleStatusPaused,
			"pause_reason": "invalid recurrence rule",
			"last_error":   err.Error(),
			"next_run_at":  nil,
		})
		return false, result.Error
	}
	next, ok := rule.Next(now)
	values := map[string]any{"last_run_at": occurrence}
	if ok {
		values["next_run_at"] = next
	} else {
		values["next_run_at"] = nil
		values["status"] = schema.AIReActScheduleStatusCompleted
	}
	result := m.db.Model(&schema.AIReActSchedule{}).
		Where("uuid = ? AND status = ? AND next_run_at = ?", schedule.UUID, schema.AIReActScheduleStatusActive, occurrence).
		Updates(values)
	return result.RowsAffected > 0, result.Error
}

func (m *Scheduler) Enqueue(schedule *schema.AIReActSchedule, scheduledAt time.Time, trigger string) error {
	if schedule == nil {
		return utils.Error("schedule is nil")
	}
	if trigger == TriggerSchedule {
		latest, err := aischedule.GetRecord(m.db, schedule.UUID)
		if err != nil {
			return err
		}
		if latest.Status == schema.AIReActScheduleStatusPaused {
			return &scheduleEnqueueError{reason: "schedule_inactive", message: "schedule is no longer active"}
		}
		schedule = latest
	}
	targetSessionID := ""
	if schedule.TargetMode == schema.AIReActScheduleTargetContinueSession {
		targetSessionID = strings.TrimSpace(schedule.TargetSessionID)
		if targetSessionID == "" {
			return utils.Error("target session id is required")
		}
		if _, err := yakit.GetAISessionMetaBySessionID(m.db, targetSessionID); err != nil {
			message := "target AI session no longer exists"
			_ = m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", schedule.UUID).Updates(map[string]any{
				"status":           schema.AIReActScheduleStatusPaused,
				"pause_reason":     message,
				"last_error":       err.Error(),
				"last_outcome":     scheduledOutcomeFailed,
				"last_skip_reason": "",
				"last_finished_at": time.Now().UTC(),
			}).Error
			return utils.Errorf("%s: %v", message, err)
		}
	}
	executionID := uuid.NewString()
	sessionID := ScheduleExecutionSessionID(schedule, executionID)
	jobCtx, cancel := context.WithCancel(m.ctx)
	job := &scheduledReActJob{
		executionID:  executionID,
		scheduleUUID: schedule.UUID,
		sessionID:    sessionID,
		scheduledAt:  scheduledAt.UTC(),
		trigger:      trigger,
		ctx:          jobCtx,
		cancel:       cancel,
		done:         make(chan struct{}),
	}
	m.jobsMu.Lock()
	if err := m.ctx.Err(); err != nil {
		m.jobsMu.Unlock()
		cancel()
		return err
	}
	if _, active := m.activeBySchedule[schedule.UUID]; active {
		m.jobsMu.Unlock()
		cancel()
		return &scheduleEnqueueError{reason: "schedule_overlap", message: "schedule already has a queued or running execution"}
	}
	select {
	case m.worker <- struct{}{}:
		job.workerReserved = true
	default:
		m.jobsMu.Unlock()
		cancel()
		return &scheduleEnqueueError{reason: "scheduler_capacity", message: "scheduled execution capacity is full"}
	}
	reservation, err := m.runtime.ReserveSession(jobCtx, sessionID, executionID)
	if err != nil {
		contextErr := jobCtx.Err()
		<-m.worker
		job.workerReserved = false
		m.jobsMu.Unlock()
		cancel()
		if contextErr != nil {
			// A concurrent scheduler stop is cancellation, not a skipped occurrence
			// caused by a busy target session.
			return contextErr
		}
		return &scheduleEnqueueError{reason: "session_busy", message: "target session is busy"}
	}
	job.reservation = reservation
	job.ctx = reservation.Context()
	m.jobs[job.executionID] = job
	m.activeBySchedule[schedule.UUID] = job.executionID
	job.unregisterExecution = aischedule.RegisterExecution(schedule.UUID, cancel)
	// Reserve the execution wait slot while jobsMu still serializes enqueue with
	// stop. Once stop has acquired jobsMu, no later WaitGroup.Add can race with
	// its Wait.
	m.wg.Add(1)
	m.jobsMu.Unlock()
	if trigger == TriggerManual {
		if err := m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", schedule.UUID).
			UpdateColumn("last_run_at", scheduledAt.UTC()).Error; err != nil {
			cancel()
			m.unregisterJob(job)
			m.wg.Done()
			return err
		}
	}
	go m.execute(job)
	return nil
}

func (m *Scheduler) unregisterJob(job *scheduledReActJob) {
	if job == nil {
		return
	}
	if job.unregisterExecution != nil {
		job.unregisterExecution()
	}
	m.jobsMu.Lock()
	delete(m.jobs, job.executionID)
	if activeID := m.activeBySchedule[job.scheduleUUID]; activeID == job.executionID {
		delete(m.activeBySchedule, job.scheduleUUID)
	}
	if job.workerReserved {
		select {
		case <-m.worker:
		default:
			log.Errorf("AI ReAct scheduler worker accounting underflow for execution %s", job.executionID)
		}
	}
	m.jobsMu.Unlock()
	if job.reservation != nil {
		job.reservation.Release()
	}
	if job.done != nil {
		close(job.done)
	}
}

func (m *Scheduler) CancelSchedule(scheduleUUID string) {
	aischedule.CancelExecution(scheduleUUID)
	m.jobsMu.Lock()
	activeID := m.activeBySchedule[strings.TrimSpace(scheduleUUID)]
	job := m.jobs[activeID]
	m.jobsMu.Unlock()
	if job != nil {
		job.cancel()
	}
}

func (m *Scheduler) ActiveExecution(scheduleUUID string) (ActiveExecution, bool) {
	if m == nil {
		return ActiveExecution{}, false
	}
	m.jobsMu.Lock()
	defer m.jobsMu.Unlock()
	activeID := m.activeBySchedule[strings.TrimSpace(scheduleUUID)]
	job := m.jobs[activeID]
	if job == nil {
		return ActiveExecution{}, false
	}
	return ActiveExecution{
		ExecutionID: job.executionID,
		SessionID:   job.sessionID,
		Done:        job.done,
	}, true
}

type scheduledReActOutcome struct {
	status       string
	errorMessage string
	skipReason   string
	reactTaskID  string
}

func (m *Scheduler) execute(job *scheduledReActJob) {
	defer m.wg.Done()
	defer func() {
		job.cancel()
		m.unregisterJob(job)
		yakit.BroadcastAISessionChanged(yakit.AISessionPushActionFinished, job.sessionID)
	}()

	schedule, err := aischedule.GetRecord(m.db, job.scheduleUUID)
	if err != nil {
		log.Infof("scheduled AI ReAct execution %s ended before start: %v", job.executionID, err)
		return
	}
	startedAt := time.Now().UTC()
	_ = m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", schedule.UUID).Updates(map[string]any{
		"last_started_at":  startedAt,
		"last_finished_at": nil,
		"last_outcome":     "",
		"last_skip_reason": "",
		"last_error":       "",
	}).Error
	maxRuntime := schedule.MaxRuntimeSeconds
	if maxRuntime <= 0 {
		maxRuntime = aischedule.DefaultMaxRuntime
	}
	runCtx, runCancel := context.WithTimeout(job.ctx, time.Duration(maxRuntime)*time.Second)
	defer runCancel()
	sessionID := job.sessionID
	if sessionID == "" {
		sessionID = ScheduleExecutionSessionID(schedule, job.executionID)
	}

	outcome := m.runReAct(runCtx, schedule, job, sessionID)
	if runCtx.Err() != nil && outcome.status == "" {
		if job.ctx.Err() != nil {
			outcome.status = scheduledOutcomeCancelled
			outcome.errorMessage = "cancelled"
		} else {
			outcome.status = scheduledOutcomeInterrupted
			outcome.errorMessage = "maximum runtime exceeded"
		}
	}
	if outcome.status == "" {
		outcome.status = scheduledOutcomeFailed
		outcome.errorMessage = "AI ReAct stopped without a terminal task event"
	}
	m.finishScheduleExecution(schedule.UUID, outcome)
	log.Infof("scheduled AI ReAct execution %s finished with status %s", job.executionID, outcome.status)
}

func isolatedScheduleSessionID(executionID string) string {
	return "ai-schedule-" + strings.TrimSpace(executionID)
}

func ScheduleExecutionSessionID(schedule *schema.AIReActSchedule, executionID string) string {
	if schedule != nil && schedule.TargetMode == schema.AIReActScheduleTargetContinueSession {
		return strings.TrimSpace(schedule.TargetSessionID)
	}
	return isolatedScheduleSessionID(executionID)
}

func (m *Scheduler) finishScheduleExecution(scheduleUUID string, outcome scheduledReActOutcome) {
	if strings.TrimSpace(scheduleUUID) == "" {
		return
	}
	lastError := ""
	switch outcome.status {
	case scheduledOutcomeFailed, scheduledOutcomeInterrupted, scheduledOutcomeNeedsAttention:
		lastError = outcome.errorMessage
	}
	if err := m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", scheduleUUID).Updates(map[string]any{
		"last_error":       lastError,
		"last_outcome":     outcome.status,
		"last_skip_reason": outcome.skipReason,
		"last_finished_at": time.Now().UTC(),
	}).Error; err != nil {
		log.Errorf("update AI ReAct schedule %s execution result failed: %v", scheduleUUID, err)
	}
}

func (m *Scheduler) recordScheduleSkipped(scheduleUUID, reason, message string) {
	if m == nil || m.db == nil || strings.TrimSpace(scheduleUUID) == "" {
		return
	}
	now := time.Now().UTC()
	if err := m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", scheduleUUID).Updates(map[string]any{
		"last_outcome":     scheduledOutcomeSkipped,
		"last_skip_reason": strings.TrimSpace(reason),
		"last_error":       "",
		"last_started_at":  nil,
		"last_finished_at": now,
	}).Error; err != nil {
		log.Errorf("record skipped AI ReAct schedule %s failed: %v", scheduleUUID, err)
	}
}

func ScheduleRunStartParams(db *gorm.DB, schedule *schema.AIReActSchedule, captured *ypb.AIStartParams) (*ypb.AIStartParams, error) {
	params, err := aischedule.NormalizeStartParams(captured)
	if err != nil {
		return nil, err
	}
	if schedule == nil || schedule.TargetMode != schema.AIReActScheduleTargetContinueSession {
		return params, nil
	}
	// Attaching with the schedule's YOLO start params would persist YOLO as the
	// user's chat default. Reuse the chat's own cached config and rely on the
	// task-level schedule source override for unattended execution.
	cached, cacheErr := yakit.GetAISessionMetaStartParamsBySessionID(db, schedule.TargetSessionID)
	if cacheErr != nil {
		return nil, utils.Errorf("load target AI session configuration failed: %v", cacheErr)
	}
	if cached == nil {
		return nil, utils.Error("target AI session has no cached configuration")
	}
	params = proto.Clone(cached).(*ypb.AIStartParams)
	params.CoordinatorId = ""
	params.Sequence = 0
	params.UserQuery = ""
	params.Attach = false
	params.PreferSessionCachedConfig = false
	return params, nil
}

// PrepareIsolatedScheduleSession makes a new-session-per-run execution visible
// to history queries before the ReAct loop starts producing output. Interactive
// sessions are announced optimistically by the renderer; scheduled sessions
// persist the same durable row and then push an invalidation to the renderer.
// Use the schedule name as the stable title instead of waiting for asynchronous
// title generation near the end of the run.
func PrepareIsolatedScheduleSession(
	db *gorm.DB,
	schedule *schema.AIReActSchedule,
	params *ypb.AIStartParams,
	sessionID string,
	startedAt time.Time,
) error {
	if schedule == nil || schedule.TargetMode == schema.AIReActScheduleTargetContinueSession {
		return nil
	}
	if _, err := yakit.CreateOrUpdateAISessionMetaOnStart(db, sessionID, params, startedAt); err != nil {
		return utils.Errorf("persist isolated scheduled session start failed: %v", err)
	}
	title := strings.TrimSpace(schedule.Name)
	if title == "" {
		title = strings.TrimSpace(schedule.Prompt)
	}
	if _, err := yakit.InitAISessionTitleIfNeeded(db, sessionID, title); err != nil {
		return utils.Errorf("initialize isolated scheduled session title failed: %v", err)
	}
	return nil
}

func (m *Scheduler) runReAct(
	ctx context.Context,
	schedule *schema.AIReActSchedule,
	job *scheduledReActJob,
	sessionID string,
) scheduledReActOutcome {
	payload, err := aischedule.UnmarshalPayload(schedule)
	if err != nil {
		return scheduledReActOutcome{status: scheduledOutcomeFailed, errorMessage: err.Error()}
	}
	params, err := ScheduleRunStartParams(m.db, schedule, payload.GetStartParams())
	if err != nil {
		return scheduledReActOutcome{status: scheduledOutcomeFailed, errorMessage: err.Error()}
	}
	params.TimelineSessionID = sessionID
	if err := PrepareIsolatedScheduleSession(m.db, schedule, params, sessionID, time.Now()); err != nil {
		// Keep the execution semantics consistent with interactive ReAct startup:
		// metadata persistence failure is observable in logs but does not suppress
		// the actual task.
		log.Warnf("prepare isolated scheduled session %s failed: %v", sessionID, err)
	}
	yakit.BroadcastAISessionChanged(yakit.AISessionPushActionStarted, sessionID)

	outcomeCh := make(chan scheduledReActOutcome, 1)
	var stateMu sync.Mutex
	state := scheduledReActOutcome{}
	pendingReviewIDs := make(map[string]struct{})
	onOutput := func(output *schema.AiOutputEvent) error {
		if output == nil {
			return nil
		}
		event := output.ToGRPC()
		if event == nil {
			return nil
		}
		stateMu.Lock()
		defer stateMu.Unlock()
		if event.GetNodeId() == "react_task_dequeue" && state.reactTaskID == "" {
			var dequeued struct {
				TaskID        string `json:"react_task_id"`
				UserInputUUID string `json:"react_task_user_input_uuid"`
				InputSource   string `json:"react_task_input_source"`
			}
			if json.Unmarshal(event.GetContent(), &dequeued) == nil &&
				dequeued.UserInputUUID == job.executionID &&
				dequeued.InputSource == aicommon.USER_INPUT_SOURCE_SCHEDULE {
				state.reactTaskID = dequeued.TaskID
				if state.reactTaskID == "" {
					state.reactTaskID = event.GetTaskId()
				}
			}
		}
		if needsAttention, message := scheduleAttentionForEvent(event, state.reactTaskID != "", pendingReviewIDs); needsAttention {
			state.status = scheduledOutcomeNeedsAttention
			state.errorMessage = message
			_ = m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", schedule.UUID).Updates(map[string]any{
				"status":       schema.AIReActScheduleStatusPaused,
				"pause_reason": "execution needs user attention",
				"last_error":   message,
			}).Error
			select {
			case outcomeCh <- state:
			default:
			}
			return nil
		}
		if event.GetNodeId() != "react_task_status_changed" {
			return nil
		}
		var content struct {
			TaskID string `json:"react_task_id"`
			Status string `json:"react_task_now_status"`
		}
		if json.Unmarshal(event.GetContent(), &content) != nil || content.Status == "" {
			return nil
		}
		if state.reactTaskID == "" || content.TaskID != state.reactTaskID {
			return nil
		}
		switch content.Status {
		case "completed":
			state.status = scheduledOutcomeSucceeded
			state.errorMessage = ""
		case "aborted":
			state.status = scheduledOutcomeFailed
			state.errorMessage = "AI ReAct task aborted"
		case "skipped":
			state.status = scheduledOutcomeSkipped
			state.errorMessage = "AI ReAct task skipped"
		default:
			return nil
		}
		select {
		case outcomeCh <- state:
		default:
		}
		return nil
	}
	attachedResources := append([]*ypb.AttachedResourceInfo(nil), payload.GetAttachedResourceInfos()...)
	attachedResources = append(attachedResources,
		&ypb.AttachedResourceInfo{
			Type:  aicommon.USER_FREE_INPUT_UUID,
			Value: job.executionID,
		},
		&ypb.AttachedResourceInfo{
			Type:  aicommon.USER_INPUT_SOURCE,
			Key:   aicommon.USER_INPUT_SOURCE_KEY,
			Value: aicommon.USER_INPUT_SOURCE_SCHEDULE,
		},
		&ypb.AttachedResourceInfo{Type: aicommon.USER_INPUT_SCHEDULE_CONTEXT, Key: aicommon.USER_INPUT_SCHEDULE_UUID, Value: schedule.UUID},
		&ypb.AttachedResourceInfo{Type: aicommon.USER_INPUT_SCHEDULE_CONTEXT, Key: aicommon.USER_INPUT_SCHEDULE_NAME, Value: schedule.Name},
		&ypb.AttachedResourceInfo{Type: aicommon.USER_INPUT_SCHEDULE_CONTEXT, Key: aicommon.USER_INPUT_SCHEDULED_AT, Value: job.scheduledAt.Format(time.RFC3339)},
		&ypb.AttachedResourceInfo{Type: aicommon.USER_INPUT_SCHEDULE_CONTEXT, Key: aicommon.USER_INPUT_SCHEDULE_TRIGGER, Value: job.trigger},
	)
	connection, err := m.runtime.Connect(ctx, sessionruntime.ConnectRequest{
		StartParams: params,
		Reservation: job.reservation,
	}, onOutput)
	if err != nil {
		return scheduledReActRuntimeError(ctx, err)
	}
	// Do not unregister the job while its Runtime connection is alive.
	// DeleteAISession may remove the session immediately after job.done.
	defer connection.Close()
	if err := connection.Send(&ypb.AIInputEvent{
		IsFreeInput:          true,
		FreeInput:            payload.GetPrompt(),
		AttachedResourceInfo: attachedResources,
		FocusModeLoop:        payload.GetFocusModeLoop(),
	}); err != nil {
		return scheduledReActRuntimeError(ctx, err)
	}
	select {
	case outcome := <-outcomeCh:
		return outcome
	case <-connection.Done():
		stateMu.Lock()
		defer stateMu.Unlock()
		return state
	case <-ctx.Done():
		stateMu.Lock()
		defer stateMu.Unlock()
		return state
	}
}

func scheduledReActRuntimeError(ctx context.Context, err error) scheduledReActOutcome {
	if ctx != nil && ctx.Err() != nil {
		// Preserve the former in-process stream behavior: cancellation and timeout
		// are classified by execute from the run context, not as runtime failures.
		return scheduledReActOutcome{}
	}
	return scheduledReActOutcome{status: scheduledOutcomeFailed, errorMessage: err.Error()}
}

func scheduleAttentionForEvent(event *ypb.AIOutputEvent, taskStarted bool, pendingReviewIDs map[string]struct{}) (bool, string) {
	if event == nil || !taskStarted {
		return false, ""
	}
	switch schema.EventType(event.GetType()) {
	case schema.EVENT_TYPE_REQUIRE_USER_INTERACTIVE:
		return true, "AI ReAct requested user interaction"
	case schema.EVENT_TYPE_TASK_REVIEW_REQUIRE,
		schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE,
		schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE,
		schema.EVENT_TYPE_EXEC_AIFORGE_REVIEW_REQUIRE:
		var request struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(event.GetContent(), &request) == nil && request.ID != "" {
			pendingReviewIDs[request.ID] = struct{}{}
		}
		// A review request is emitted before AI risk control runs. Low and
		// medium risk are automatically approved, so the request itself does not
		// mean a human is needed.
		return false, ""
	case schema.EVENT_TYPE_AI_REVIEW_END:
		var result struct {
			InteractiveID string `json:"interactive_id"`
			RequiresUser  bool   `json:"requires_user"`
			Reason        string `json:"reason"`
		}
		if json.Unmarshal(event.GetContent(), &result) != nil || result.InteractiveID == "" {
			return false, ""
		}
		if _, ok := pendingReviewIDs[result.InteractiveID]; !ok {
			return false, ""
		}
		delete(pendingReviewIDs, result.InteractiveID)
		if !result.RequiresUser {
			return false, ""
		}
		message := "AI review escalated execution to the user"
		if reason := strings.TrimSpace(result.Reason); reason != "" {
			message += ": " + reason
		}
		return true, message
	default:
		return false, ""
	}
}
