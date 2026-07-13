package aicommon

import (
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const SessionSnapshotNodeID = "session_snapshot"

const (
	SessionSnapshotSectionExecution           = "execution"
	SessionSnapshotSectionPerception          = "perception"
	SessionSnapshotSectionCapabilities        = "capabilities"
	SessionSnapshotSectionBackgroundProcesses = "background_processes"

	SessionSnapshotProcessTypeBrowser   = "browser"
	SessionSnapshotProcessStatusRunning = "running"
)

type sessionSnapshotEmitHandler func()

// SessionSnapshot is the unified real-time sidebar payload for frontend consumption.
// TaskId / TaskIndex are carried on the outer AIOutputEvent envelope, not duplicated here.
type SessionSnapshot struct {
	Revision            int64                              `json:"revision"`
	UpdatedAt           int64                              `json:"updated_at"`
	Execution           *SessionSnapshotExecution          `json:"execution"`
	Perception          *SessionSnapshotPerception         `json:"perception"`
	Capabilities        []CapabilityInventoryItem          `json:"capabilities"`
	BackgroundProcesses []SessionSnapshotBackgroundProcess `json:"background_processes"`
}

// SessionSnapshotBackgroundProcess describes a long-lived background resource owned by the session.
// Process types are extensible; only "browser" is supported today.
type SessionSnapshotBackgroundProcess struct {
	Type        string `json:"type"`
	ProcessID   string `json:"process_id"`
	ProcessName string `json:"process_name"`
	Status      string `json:"status"`
	StartedAt   int64  `json:"started_at"`
}

type SessionSnapshotExecution struct {
	TaskName          string `json:"task_name"`
	Status            string `json:"status"`
	StartedAt         int64  `json:"started_at"`
	EndedAt           int64  `json:"ended_at"`
	ToolCallSuccess   int    `json:"tool_call_success"`
	ToolCallFailed    int    `json:"tool_call_failed"`
	ToolCallTotal     int    `json:"tool_call_total"`
	ExecutionMinutes  int    `json:"execution_minutes"`
	HTTPFlowCount     int    `json:"http_flow_count"`
	RiskCount         int    `json:"risk_count"`
	ModifiedFileCount int    `json:"modified_file_count"`
}

type SessionSnapshotPerception struct {
	Summary      string   `json:"summary"`
	Topics       []string `json:"topics"`
	Keywords     []string `json:"keywords"`
	Confidence   float64  `json:"confidence"`
	Changed      bool     `json:"changed"`
	Epoch        int      `json:"epoch"`
	LastTrigger  string   `json:"last_trigger"`
	IntentShift  string   `json:"intent_shift"`
	LastUpdateAt int64    `json:"last_update_at"`

	CapabilityMatches *SessionSnapshotPerceptionCapabilityMatches `json:"capability_matches"`
	Knowledge         *SessionSnapshotPerceptionKnowledge         `json:"knowledge"`
}

type SessionSnapshotPerceptionCapabilityMatches struct {
	Query                   string   `json:"query"`
	MatchedToolNames        []string `json:"matched_tool_names"`
	MatchedForgeNames       []string `json:"matched_forge_names"`
	MatchedSkillNames       []string `json:"matched_skill_names"`
	MatchedFocusModeNames   []string `json:"matched_focus_mode_names"`
	RecommendedCapabilities []string `json:"recommended_capabilities"`
}

type SessionSnapshotPerceptionKnowledge struct {
	Query          string   `json:"query"`
	KnowledgeBases []string `json:"knowledge_bases"`
	Content        string   `json:"content"`
}

type sessionSnapshotPerceptionExtras struct {
	CapabilityMatches *SessionSnapshotPerceptionCapabilityMatches
	Knowledge         *SessionSnapshotPerceptionKnowledge
}

type sessionExecutionTracker struct {
	mu               sync.Mutex
	stats            SessionSnapshotExecution
	callToolIDs      map[string]struct{}
	firstEmitPending bool
}

func isSessionSnapshotExecutionTerminal(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "aborted", "skipped":
		return true
	default:
		return false
	}
}

type sessionSnapshotState struct {
	mu                   sync.Mutex
	revision             int64
	perceptionExtras     sessionSnapshotPerceptionExtras
	backgroundProcesses  map[string]SessionSnapshotBackgroundProcess
	execution            sessionExecutionTracker
	emitHandler          sessionSnapshotEmitHandler
	debounceTimer        *time.Timer
	legacySeparateEvents bool
}

func (c *Config) ensureSessionSnapshotState() *sessionSnapshotState {
	if c == nil {
		return nil
	}
	if c.m == nil {
		c.m = &sync.Mutex{}
	}
	c.m.Lock()
	defer c.m.Unlock()
	if c.sessionSnapshot == nil {
		c.sessionSnapshot = &sessionSnapshotState{
			legacySeparateEvents: true,
			backgroundProcesses:  make(map[string]SessionSnapshotBackgroundProcess),
			execution: sessionExecutionTracker{
				callToolIDs: make(map[string]struct{}),
			},
		}
	}
	return c.sessionSnapshot
}

func (c *Config) SetSessionSnapshotEmitHandler(handler sessionSnapshotEmitHandler) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	state.emitHandler = handler
	state.mu.Unlock()
}

func (c *Config) EmitLegacySessionSnapshotEvents() bool {
	if c == nil {
		return true
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.legacySeparateEvents
}

func (c *Config) SetEmitLegacySessionSnapshotEvents(enabled bool) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	state.legacySeparateEvents = enabled
	state.mu.Unlock()
}

func (c *Config) NotifySessionSnapshotEmit(immediate ...bool) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	force := len(immediate) > 0 && immediate[0]
	if force {
		c.flushSessionSnapshotEmit()
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.emitHandler == nil {
		return
	}
	if state.debounceTimer != nil {
		state.debounceTimer.Stop()
	}
	state.debounceTimer = time.AfterFunc(time.Second, func() {
		c.flushSessionSnapshotEmit()
	})
}

func (c *Config) flushSessionSnapshotEmit() {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	handler := state.emitHandler
	state.mu.Unlock()
	if handler != nil {
		handler()
	}
}

func (c *Config) NextSessionSnapshotRevision() int64 {
	if c == nil {
		return 0
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return 0
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.revision++
	return state.revision
}

func (c *Config) GetSessionSnapshotPerceptionExtras() (capabilityMatches *SessionSnapshotPerceptionCapabilityMatches, knowledge *SessionSnapshotPerceptionKnowledge) {
	if c == nil {
		return nil, nil
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return nil, nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.perceptionExtras.CapabilityMatches != nil {
		copied := *state.perceptionExtras.CapabilityMatches
		capabilityMatches = &copied
	}
	if state.perceptionExtras.Knowledge != nil {
		copied := *state.perceptionExtras.Knowledge
		knowledge = &copied
	}
	return capabilityMatches, knowledge
}

func (c *Config) SetSessionSnapshotPerceptionCapabilityMatches(matches *SessionSnapshotPerceptionCapabilityMatches) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if matches == nil {
		state.perceptionExtras.CapabilityMatches = nil
		return
	}
	copied := *matches
	state.perceptionExtras.CapabilityMatches = &copied
}

func (c *Config) AddSessionSnapshotBackgroundProcess(processType, processID, processName string) {
	processType = strings.TrimSpace(processType)
	processID = strings.TrimSpace(processID)
	processName = strings.TrimSpace(processName)
	if c == nil || processType == "" || processID == "" {
		return
	}
	if processName == "" {
		processName = processID
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.backgroundProcesses == nil {
		state.backgroundProcesses = make(map[string]SessionSnapshotBackgroundProcess)
	}
	state.backgroundProcesses[processID] = SessionSnapshotBackgroundProcess{
		Type:        processType,
		ProcessID:   processID,
		ProcessName: processName,
		Status:      SessionSnapshotProcessStatusRunning,
		StartedAt:   time.Now().Unix(),
	}
}

func (c *Config) RemoveSessionSnapshotBackgroundProcess(processID string) {
	processID = strings.TrimSpace(processID)
	if c == nil || processID == "" {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.backgroundProcesses == nil {
		return
	}
	delete(state.backgroundProcesses, processID)
}

func (c *Config) BuildSessionSnapshotBackgroundProcesses() []SessionSnapshotBackgroundProcess {
	if c == nil {
		return []SessionSnapshotBackgroundProcess{}
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return []SessionSnapshotBackgroundProcess{}
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.backgroundProcesses) == 0 {
		return []SessionSnapshotBackgroundProcess{}
	}
	out := make([]SessionSnapshotBackgroundProcess, 0, len(state.backgroundProcesses))
	for _, proc := range state.backgroundProcesses {
		out = append(out, proc)
	}
	return out
}

func (c *Config) SetSessionSnapshotPerceptionKnowledge(knowledge *SessionSnapshotPerceptionKnowledge) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if knowledge == nil {
		state.perceptionExtras.Knowledge = nil
		return
	}
	copied := *knowledge
	state.perceptionExtras.Knowledge = &copied
}

func (c *Config) ResetSessionSnapshotExecution(taskName, status string, startedAt time.Time) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.execution.mu.Lock()
	defer state.execution.mu.Unlock()
	startedUnix := startedAt.Unix()
	state.execution.stats = SessionSnapshotExecution{
		TaskName:  strings.TrimSpace(taskName),
		Status:    strings.TrimSpace(status),
		StartedAt: startedUnix,
		EndedAt:   startedUnix,
	}
	state.execution.firstEmitPending = true
	state.execution.callToolIDs = make(map[string]struct{})
}

func (c *Config) FinalizeSessionSnapshotExecution(status string, endedAt time.Time) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.execution.mu.Lock()
	defer state.execution.mu.Unlock()
	state.execution.stats.Status = strings.TrimSpace(status)
	state.execution.firstEmitPending = false
	if isSessionSnapshotExecutionTerminal(state.execution.stats.Status) {
		if !endedAt.IsZero() {
			state.execution.stats.EndedAt = endedAt.Unix()
		}
	}
	c.refreshSessionSnapshotDurationLocked(state, true)
}

func (c *Config) RecordSessionSnapshotToolCall(result *aitool.ToolResult) {
	if c == nil || result == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}

	state.execution.mu.Lock()
	defer state.execution.mu.Unlock()

	if result.Success {
		state.execution.stats.ToolCallSuccess++
	} else {
		state.execution.stats.ToolCallFailed++
	}
	state.execution.stats.ToolCallTotal = state.execution.stats.ToolCallSuccess + state.execution.stats.ToolCallFailed

	if callToolID := strings.TrimSpace(result.ToolCallID); callToolID != "" {
		state.execution.callToolIDs[callToolID] = struct{}{}
	}

	c.refreshSessionSnapshotRuntimeCountsLocked(state)
	c.refreshSessionSnapshotDurationLocked(state, false)
}

func (c *Config) RecordSessionSnapshotFileWrite(path string) {
	path = strings.TrimSpace(path)
	if c == nil || path == "" {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}

	state.execution.mu.Lock()
	defer state.execution.mu.Unlock()
	state.execution.stats.ModifiedFileCount++
}

func (c *Config) RefreshSessionSnapshotRuntimeCounts(callToolID string) {
	if c == nil {
		return
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return
	}
	state.execution.mu.Lock()
	defer state.execution.mu.Unlock()
	if id := strings.TrimSpace(callToolID); id != "" {
		state.execution.callToolIDs[id] = struct{}{}
	}
	c.refreshSessionSnapshotRuntimeCountsLocked(state)
	c.refreshSessionSnapshotDurationLocked(state, false)
}

func emptySessionSnapshotExecution() *SessionSnapshotExecution {
	return &SessionSnapshotExecution{
		Status: "processing",
	}
}

// NormalizeSessionSnapshot ensures every emit carries a full payload so the
// frontend can replace planDetails sections without merging missing keys.
func NormalizeSessionSnapshot(snapshot *SessionSnapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.Execution == nil {
		snapshot.Execution = emptySessionSnapshotExecution()
	}
	if snapshot.Perception == nil {
		snapshot.Perception = &SessionSnapshotPerception{}
	}
	if snapshot.Capabilities == nil {
		snapshot.Capabilities = []CapabilityInventoryItem{}
	}
	if snapshot.BackgroundProcesses == nil {
		snapshot.BackgroundProcesses = []SessionSnapshotBackgroundProcess{}
	}
}

func (c *Config) BuildSessionSnapshotExecution(task AIStatefulTask) *SessionSnapshotExecution {
	if c == nil {
		return buildSessionSnapshotExecutionFromTask(task)
	}
	state := c.ensureSessionSnapshotState()
	if state == nil {
		return buildSessionSnapshotExecutionFromTask(task)
	}

	state.execution.mu.Lock()
	defer state.execution.mu.Unlock()

	if state.execution.stats.ToolCallTotal == 0 && task != nil {
		c.syncExecutionToolCountsLocked(state, task)
	}
	c.refreshSessionSnapshotRuntimeCountsLocked(state)
	if strings.TrimSpace(state.execution.stats.Status) == "" {
		state.execution.stats.Status = SessionSnapshotStatusFromTask(task)
	}
	c.prepareSessionSnapshotEndedAtForEmitLocked(state)
	c.refreshSessionSnapshotDurationLocked(state, true)

	copied := state.execution.stats
	return &copied
}

func (c *Config) refreshSessionSnapshotRuntimeCountsLocked(state *sessionSnapshotState) {
	if state == nil {
		return
	}
	db := consts.GetGormProjectDatabase()
	if db == nil {
		return
	}
	httpTotal := 0
	riskTotal := 0
	for callToolID := range state.execution.callToolIDs {
		httpTotal += yakit.CountHTTPFlowByRuntimeID(db, callToolID)
		riskTotal += func() int {
			count, _ := yakit.CountRiskByRuntimeId(db, callToolID)
			return count
		}()
	}
	state.execution.stats.HTTPFlowCount = httpTotal
	state.execution.stats.RiskCount = riskTotal
}

func (c *Config) prepareSessionSnapshotEndedAtForEmitLocked(state *sessionSnapshotState) {
	if state == nil || state.execution.stats.StartedAt <= 0 {
		return
	}
	if isSessionSnapshotExecutionTerminal(state.execution.stats.Status) {
		return
	}
	if state.execution.firstEmitPending {
		state.execution.stats.EndedAt = state.execution.stats.StartedAt
		state.execution.firstEmitPending = false
		return
	}
	state.execution.stats.EndedAt = time.Now().Unix()
}

func (c *Config) refreshSessionSnapshotDurationLocked(state *sessionSnapshotState, useStoredEndedAt bool) {
	if state == nil || state.execution.stats.StartedAt <= 0 {
		return
	}
	var endAt time.Time
	switch {
	case isSessionSnapshotExecutionTerminal(state.execution.stats.Status) && state.execution.stats.EndedAt > 0:
		endAt = time.Unix(state.execution.stats.EndedAt, 0)
	case useStoredEndedAt && state.execution.stats.EndedAt > 0:
		endAt = time.Unix(state.execution.stats.EndedAt, 0)
	default:
		endAt = time.Now()
	}
	state.execution.stats.ExecutionMinutes = int(endAt.Sub(time.Unix(state.execution.stats.StartedAt, 0)).Minutes())
}

func (c *Config) syncExecutionToolCountsLocked(state *sessionSnapshotState, task AIStatefulTask) {
	if task == nil {
		return
	}
	success := 0
	failed := 0
	for _, result := range task.GetAllToolCallResults() {
		if result == nil {
			continue
		}
		if result.Success {
			success++
		} else {
			failed++
		}
		if callToolID := strings.TrimSpace(result.ToolCallID); callToolID != "" {
			state.execution.callToolIDs[callToolID] = struct{}{}
		}
	}
	state.execution.stats.ToolCallSuccess = success
	state.execution.stats.ToolCallFailed = failed
	state.execution.stats.ToolCallTotal = success + failed
	if state.execution.stats.TaskName == "" {
		state.execution.stats.TaskName = strings.TrimSpace(task.GetName())
	}
}

func buildSessionSnapshotExecutionFromTask(task AIStatefulTask) *SessionSnapshotExecution {
	if task == nil {
		return emptySessionSnapshotExecution()
	}
	stats := &SessionSnapshotExecution{
		TaskName: strings.TrimSpace(task.GetName()),
		Status:   SessionSnapshotStatusFromTask(task),
	}
	for _, result := range task.GetAllToolCallResults() {
		if result == nil {
			continue
		}
		if result.Success {
			stats.ToolCallSuccess++
		} else {
			stats.ToolCallFailed++
		}
	}
	stats.ToolCallTotal = stats.ToolCallSuccess + stats.ToolCallFailed
	return stats
}

func ConfigFromAICaller(cfg AICallerConfigIf) *Config {
	if cfg == nil {
		return nil
	}
	if c, ok := cfg.(*Config); ok {
		return c
	}
	return nil
}

func NotifySessionSnapshotToolCall(cfg AICallerConfigIf, result *aitool.ToolResult) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.RecordSessionSnapshotToolCall(result)
		c.NotifySessionSnapshotEmit()
	}
}

func NotifySessionSnapshotRuntimeRefresh(cfg AICallerConfigIf, callToolID string) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.RefreshSessionSnapshotRuntimeCounts(callToolID)
		c.NotifySessionSnapshotEmit()
	}
}

func NotifySessionSnapshotFileWrite(cfg AICallerConfigIf, path string) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.RecordSessionSnapshotFileWrite(path)
		c.NotifySessionSnapshotEmit()
	}
}

func NotifySessionSnapshotEmit(cfg AICallerConfigIf, immediate ...bool) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.NotifySessionSnapshotEmit(immediate...)
	}
}

func NotifySessionSnapshotBrowserOpened(cfg AICallerConfigIf, browserID, processName string) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.AddSessionSnapshotBackgroundProcess(SessionSnapshotProcessTypeBrowser, browserID, processName)
		c.NotifySessionSnapshotEmit(true)
	}
}

func NotifySessionSnapshotBrowserClosed(cfg AICallerConfigIf, browserID string) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.RemoveSessionSnapshotBackgroundProcess(browserID)
		c.NotifySessionSnapshotEmit(true)
	}
}

func SetSessionSnapshotPerceptionCapabilityMatches(cfg AICallerConfigIf, matches *SessionSnapshotPerceptionCapabilityMatches) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.SetSessionSnapshotPerceptionCapabilityMatches(matches)
	}
}

func SetSessionSnapshotPerceptionKnowledge(cfg AICallerConfigIf, knowledge *SessionSnapshotPerceptionKnowledge) {
	if c := ConfigFromAICaller(cfg); c != nil {
		c.SetSessionSnapshotPerceptionKnowledge(knowledge)
	}
}

// SessionSnapshotStatusFromTask maps task runtime status to session_snapshot execution status.
func SessionSnapshotStatusFromTask(task AIStatefulTask) string {
	if task == nil {
		return "processing"
	}
	switch task.GetStatus() {
	case AITaskState_Completed:
		return "completed"
	case AITaskState_Aborted:
		return "aborted"
	case AITaskState_Skipped:
		return "skipped"
	default:
		return "processing"
	}
}

// BeginSessionSnapshotExecutionForTask resets execution stats and emits an immediate snapshot.
func BeginSessionSnapshotExecutionForTask(c *Config, task AIStatefulTask, startedAt time.Time) {
	if c == nil || task == nil {
		return
	}
	taskName := strings.TrimSpace(task.GetName())
	if taskName == "" {
		taskName = "task"
	}
	c.ResetSessionSnapshotExecution(taskName, "processing", startedAt)
	c.NotifySessionSnapshotEmit(true)
}

// FinalizeSessionSnapshotExecutionForTask marks execution ended and refreshes duration fields.
func FinalizeSessionSnapshotExecutionForTask(c *Config, task AIStatefulTask, endedAt time.Time) {
	if c == nil || task == nil {
		return
	}
	c.FinalizeSessionSnapshotExecution(SessionSnapshotStatusFromTask(task), endedAt)
}
