package yakgrpc

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aischedule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const (
	aiReActSchedulePollInterval  = 30 * time.Second
	aiReActScheduleMaxConcurrent = 3
	legacyAIReActRunTable        = "ai_react_schedule_runs_v1"

	aiReActScheduleTriggerSchedule = "schedule"
	aiReActScheduleTriggerManual   = "manual"

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
	targetSessionID     string
	scheduledAt         time.Time
	trigger             string
	ctx                 context.Context
	cancel              context.CancelFunc
	unregisterExecution func()
	workerReserved      bool
}

type scheduleEnqueueError struct {
	reason  string
	message string
}

func (e *scheduleEnqueueError) Error() string { return e.message }

type aiReActScheduler struct {
	server *Server
	db     *gorm.DB

	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{}
	worker chan struct{}

	jobsMu           sync.Mutex
	jobs             map[string]*scheduledReActJob
	activeBySchedule map[string]string
	activeBySession  map[string]string
	wg               sync.WaitGroup
}

func newAIReActScheduler(server *Server, db *gorm.DB) *aiReActScheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &aiReActScheduler{
		server:           server,
		db:               db,
		ctx:              ctx,
		cancel:           cancel,
		wake:             make(chan struct{}, 1),
		worker:           make(chan struct{}, aiReActScheduleMaxConcurrent),
		jobs:             make(map[string]*scheduledReActJob),
		activeBySchedule: make(map[string]string),
		activeBySession:  make(map[string]string),
	}
}

func dropLegacyAIReActScheduleRuns(db *gorm.DB) {
	if db == nil {
		return
	}
	if !db.HasTable(legacyAIReActRunTable) {
		return
	}
	if err := db.DropTableIfExists(legacyAIReActRunTable).Error; err != nil {
		log.Warnf("drop legacy AI ReAct schedule run history failed: %v", err)
	}
}

func backfillAIReActScheduleProvenance(db *gorm.DB) {
	if db == nil || !db.HasTable((&schema.AIReActSchedule{}).TableName()) {
		return
	}
	if err := db.Exec(`UPDATE ai_react_schedules_v1 SET original_request = prompt WHERE original_request IS NULL OR TRIM(original_request) = ''`).Error; err != nil {
		log.Warnf("backfill AI ReAct schedule original request failed: %v", err)
	}
	if err := db.Exec(`UPDATE ai_react_schedules_v1 SET created_from_session_id = target_session_id WHERE target_mode = ? AND (created_from_session_id IS NULL OR TRIM(created_from_session_id) = '')`, schema.AIReActScheduleTargetContinueSession).Error; err != nil {
		log.Warnf("backfill AI ReAct schedule source session failed: %v", err)
	}
}

func cleanupOrphanedAttachedAIReActSchedules(db *gorm.DB) {
	if db == nil || !db.HasTable((&schema.AIReActSchedule{}).TableName()) || !db.HasTable((&schema.AISession{}).TableName()) {
		return
	}
	result := db.Where(
		`target_mode = ? AND (TRIM(target_session_id) = '' OR NOT EXISTS (`+
			`SELECT 1 FROM ai_sessions_v1 AS session_owner `+
			`WHERE session_owner.deleted_at IS NULL AND session_owner.session_id = ai_react_schedules_v1.target_session_id))`,
		schema.AIReActScheduleTargetContinueSession,
	).Delete(&schema.AIReActSchedule{})
	if result.Error != nil {
		log.Warnf("cleanup orphaned attached AI ReAct schedules failed: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Infof("removed %d orphaned attached AI ReAct schedules", result.RowsAffected)
	}
}

// StartAIReActScheduler starts the project-scoped scheduler. Task definitions
// are durable, while active execution state intentionally remains in memory.
// It is safe to call more than once and is independent of UI streams.
func (s *Server) StartAIReActScheduler() {
	s.ensureAIReActScheduler()
}

func (s *Server) ensureAIReActScheduler() {
	if s == nil {
		return
	}
	s.aiReActSchedulerMu.Lock()
	defer s.aiReActSchedulerMu.Unlock()
	if s.aiReActScheduler != nil && s.aiReActScheduler.ctx.Err() == nil {
		return
	}
	manager := newAIReActScheduler(s, s.GetProjectDatabase())
	dropLegacyAIReActScheduleRuns(manager.db)
	backfillAIReActScheduleProvenance(manager.db)
	cleanupOrphanedAttachedAIReActSchedules(manager.db)
	s.aiReActScheduler = manager
	manager.wg.Add(1)
	go manager.loop()
}

func (s *Server) stopAIReActScheduler() {
	if s == nil {
		return
	}
	s.aiReActSchedulerMu.Lock()
	manager := s.aiReActScheduler
	s.aiReActScheduler = nil
	s.aiReActSchedulerMu.Unlock()
	if manager != nil {
		manager.stop()
	}
}

func (s *Server) wakeAIReActScheduler() {
	s.aiReActSchedulerMu.Lock()
	manager := s.aiReActScheduler
	s.aiReActSchedulerMu.Unlock()
	if manager != nil {
		manager.notify()
	}
}

func (s *Server) currentAIReActScheduler() *aiReActScheduler {
	s.aiReActSchedulerMu.Lock()
	defer s.aiReActSchedulerMu.Unlock()
	return s.aiReActScheduler
}

func (s *Server) enqueueAIReActSchedule(schedule *schema.AIReActSchedule, scheduledAt time.Time, trigger string) error {
	manager := s.currentAIReActScheduler()
	if manager == nil {
		return utils.Error("AI ReAct scheduler is not running")
	}
	return manager.enqueue(schedule, scheduledAt, trigger)
}

func (s *Server) cancelAIReActScheduleExecution(scheduleUUID string) {
	aischedule.CancelExecution(scheduleUUID)
	if manager := s.currentAIReActScheduler(); manager != nil {
		manager.cancelSchedule(scheduleUUID)
	}
}

func (m *aiReActScheduler) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *aiReActScheduler) stop() {
	m.cancel()
	m.jobsMu.Lock()
	for _, job := range m.jobs {
		job.cancel()
	}
	m.jobsMu.Unlock()
	m.wg.Wait()
}

func (m *aiReActScheduler) loop() {
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

func (m *aiReActScheduler) dispatchDue() {
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
		if err := m.enqueue(schedule, occurrence, aiReActScheduleTriggerSchedule); err != nil {
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

func (m *aiReActScheduler) advanceSchedule(schedule *schema.AIReActSchedule, occurrence, now time.Time) (bool, error) {
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

func (m *aiReActScheduler) enqueue(schedule *schema.AIReActSchedule, scheduledAt time.Time, trigger string) error {
	if schedule == nil {
		return utils.Error("schedule is nil")
	}
	if trigger == aiReActScheduleTriggerSchedule {
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
	jobCtx, cancel := context.WithCancel(m.ctx)
	job := &scheduledReActJob{
		executionID:     uuid.NewString(),
		scheduleUUID:    schedule.UUID,
		targetSessionID: targetSessionID,
		scheduledAt:     scheduledAt.UTC(),
		trigger:         trigger,
		ctx:             jobCtx,
		cancel:          cancel,
	}
	m.jobsMu.Lock()
	if _, active := m.activeBySchedule[schedule.UUID]; active {
		m.jobsMu.Unlock()
		cancel()
		return &scheduleEnqueueError{reason: "schedule_overlap", message: "schedule already has a queued or running execution"}
	}
	if targetSessionID != "" {
		if _, active := m.activeBySession[targetSessionID]; active || isAIReActSessionBusy(targetSessionID) {
			m.jobsMu.Unlock()
			cancel()
			return &scheduleEnqueueError{reason: "session_busy", message: "target session is busy"}
		}
	}
	select {
	case m.worker <- struct{}{}:
		job.workerReserved = true
	default:
		m.jobsMu.Unlock()
		cancel()
		return &scheduleEnqueueError{reason: "scheduler_capacity", message: "scheduled execution capacity is full"}
	}
	m.jobs[job.executionID] = job
	m.activeBySchedule[schedule.UUID] = job.executionID
	if targetSessionID != "" {
		m.activeBySession[targetSessionID] = job.executionID
	}
	job.unregisterExecution = aischedule.RegisterExecution(schedule.UUID, cancel)
	m.jobsMu.Unlock()
	if trigger == aiReActScheduleTriggerManual {
		if err := m.db.Model(&schema.AIReActSchedule{}).Where("uuid = ?", schedule.UUID).
			UpdateColumn("last_run_at", scheduledAt.UTC()).Error; err != nil {
			cancel()
			m.unregisterJob(job)
			return err
		}
	}
	m.wg.Add(1)
	go m.execute(job)
	return nil
}

func (m *aiReActScheduler) unregisterJob(job *scheduledReActJob) {
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
	if activeID := m.activeBySession[job.targetSessionID]; job.targetSessionID != "" && activeID == job.executionID {
		delete(m.activeBySession, job.targetSessionID)
	}
	if job.workerReserved {
		select {
		case <-m.worker:
		default:
			log.Errorf("AI ReAct scheduler worker accounting underflow for execution %s", job.executionID)
		}
	}
	m.jobsMu.Unlock()
}

func (m *aiReActScheduler) cancelSchedule(scheduleUUID string) {
	m.jobsMu.Lock()
	activeID := m.activeBySchedule[strings.TrimSpace(scheduleUUID)]
	job := m.jobs[activeID]
	m.jobsMu.Unlock()
	if job != nil {
		job.cancel()
	}
}

func (m *aiReActScheduler) isSessionReserved(sessionID string) bool {
	if m == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	m.jobsMu.Lock()
	_, ok := m.activeBySession[strings.TrimSpace(sessionID)]
	m.jobsMu.Unlock()
	return ok
}

type scheduledReActOutcome struct {
	status       string
	errorMessage string
	skipReason   string
	reactTaskID  string
}

func (m *aiReActScheduler) execute(job *scheduledReActJob) {
	defer m.wg.Done()
	defer func() {
		job.cancel()
		m.unregisterJob(job)
	}()

	schedule, err := getAIReActScheduleRecord(m.db, job.scheduleUUID)
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
		maxRuntime = defaultAIReActScheduleRuntime
	}
	runCtx, runCancel := context.WithTimeout(job.ctx, time.Duration(maxRuntime)*time.Second)
	defer runCancel()
	sessionID := scheduleExecutionSessionID(schedule, job.executionID)

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

func scheduleExecutionSessionID(schedule *schema.AIReActSchedule, executionID string) string {
	if schedule != nil && schedule.TargetMode == schema.AIReActScheduleTargetContinueSession {
		return strings.TrimSpace(schedule.TargetSessionID)
	}
	return isolatedScheduleSessionID(executionID)
}

func isAIReActSessionBusy(sessionID string) bool {
	return aireact.IsSessionBusy(strings.TrimSpace(sessionID))
}

func (m *aiReActScheduler) finishScheduleExecution(scheduleUUID string, outcome scheduledReActOutcome) {
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

func (m *aiReActScheduler) recordScheduleSkipped(scheduleUUID, reason, message string) {
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

func scheduleRunStartParams(db *gorm.DB, schedule *schema.AIReActSchedule, captured *ypb.AIStartParams) (*ypb.AIStartParams, error) {
	params, err := normalizeScheduleStartParams(captured)
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

func (m *aiReActScheduler) runReAct(
	ctx context.Context,
	schedule *schema.AIReActSchedule,
	job *scheduledReActJob,
	sessionID string,
) scheduledReActOutcome {
	payload, err := unmarshalSchedulePayload(schedule)
	if err != nil {
		return scheduledReActOutcome{status: scheduledOutcomeFailed, errorMessage: err.Error()}
	}
	params, err := scheduleRunStartParams(m.db, schedule, payload.GetStartParams())
	if err != nil {
		return scheduledReActOutcome{status: scheduledOutcomeFailed, errorMessage: err.Error()}
	}
	params.TimelineSessionID = sessionID

	outcomeCh := make(chan scheduledReActOutcome, 1)
	serverErrCh := make(chan error, 1)
	var stateMu sync.Mutex
	state := scheduledReActOutcome{}
	pendingReviewIDs := make(map[string]struct{})
	stream := newInProcessAIReActStream(ctx, func(event *ypb.AIOutputEvent) {
		if event == nil {
			return
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
			return
		}
		if event.GetNodeId() != "react_task_status_changed" {
			return
		}
		var content struct {
			TaskID string `json:"react_task_id"`
			Status string `json:"react_task_now_status"`
		}
		if json.Unmarshal(event.GetContent(), &content) != nil || content.Status == "" {
			return
		}
		if state.reactTaskID == "" || content.TaskID != state.reactTaskID {
			return
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
			return
		}
		select {
		case outcomeCh <- state:
		default:
		}
	})
	stream.push(&ypb.AIInputEvent{IsStart: true, Params: params})
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
	stream.push(&ypb.AIInputEvent{
		IsFreeInput:          true,
		FreeInput:            payload.GetPrompt(),
		AttachedResourceInfo: attachedResources,
		FocusModeLoop:        payload.GetFocusModeLoop(),
	})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		serverErrCh <- m.server.StartAIReAct(stream)
	}()
	select {
	case outcome := <-outcomeCh:
		stream.cancel()
		return outcome
	case err := <-serverErrCh:
		stateMu.Lock()
		defer stateMu.Unlock()
		if err != nil && ctx.Err() == nil {
			state.status = scheduledOutcomeFailed
			state.errorMessage = err.Error()
		}
		return state
	case <-ctx.Done():
		stream.cancel()
		stateMu.Lock()
		defer stateMu.Unlock()
		return state
	}
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

type inProcessAIReActStream struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	input    chan *ypb.AIInputEvent
	onOutput func(*ypb.AIOutputEvent)
}

func newInProcessAIReActStream(parent context.Context, onOutput func(*ypb.AIOutputEvent)) *inProcessAIReActStream {
	ctx, cancel := context.WithCancel(parent)
	return &inProcessAIReActStream{ctx: ctx, cancelFn: cancel, input: make(chan *ypb.AIInputEvent, 2), onOutput: onOutput}
}

func (s *inProcessAIReActStream) push(event *ypb.AIInputEvent) {
	select {
	case s.input <- event:
	case <-s.ctx.Done():
	}
}

func (s *inProcessAIReActStream) cancel() { s.cancelFn() }

func (s *inProcessAIReActStream) Recv() (*ypb.AIInputEvent, error) {
	select {
	case event := <-s.input:
		return event, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *inProcessAIReActStream) Send(event *ypb.AIOutputEvent) error {
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if s.onOutput != nil {
		s.onOutput(event)
	}
	return nil
}

func (s *inProcessAIReActStream) SetHeader(metadata.MD) error  { return nil }
func (s *inProcessAIReActStream) SendHeader(metadata.MD) error { return nil }
func (s *inProcessAIReActStream) SetTrailer(metadata.MD)       {}
func (s *inProcessAIReActStream) Context() context.Context     { return s.ctx }

func (s *inProcessAIReActStream) SendMsg(message any) error {
	event, ok := message.(*ypb.AIOutputEvent)
	if !ok {
		return utils.Error("invalid AI ReAct output message")
	}
	return s.Send(event)
}

func (s *inProcessAIReActStream) RecvMsg(message any) error {
	event, err := s.Recv()
	if err != nil {
		if err == context.Canceled {
			return io.EOF
		}
		return err
	}
	target, ok := message.(*ypb.AIInputEvent)
	if !ok {
		return utils.Error("invalid AI ReAct input message")
	}
	proto.Reset(target)
	proto.Merge(target, event)
	return nil
}
