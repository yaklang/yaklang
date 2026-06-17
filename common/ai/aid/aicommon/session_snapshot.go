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
	SessionSnapshotSectionExecution    = "execution"
	SessionSnapshotSectionPerception   = "perception"
	SessionSnapshotSectionCapabilities = "capabilities"
)

type sessionSnapshotEmitHandler func()

// SessionSnapshot is the unified real-time sidebar payload for frontend consumption.
// TaskId / TaskIndex are carried on the outer AIOutputEvent envelope, not duplicated here.
type SessionSnapshot struct {
	Revision     int64                      `json:"revision"`
	UpdatedAt    int64                      `json:"updated_at"`
	Execution    *SessionSnapshotExecution    `json:"execution,omitempty"`
	Perception   *SessionSnapshotPerception   `json:"perception,omitempty"`
	Capabilities []CapabilityInventoryItem    `json:"capabilities"`
}

type SessionSnapshotExecution struct {
	TaskName          string `json:"task_name,omitempty"`
	Status            string `json:"status"`
	StartedAt         int64  `json:"started_at,omitempty"`
	EndedAt           int64  `json:"ended_at,omitempty"`
	ToolCallSuccess   int    `json:"tool_call_success"`
	ToolCallFailed    int    `json:"tool_call_failed"`
	ToolCallTotal     int    `json:"tool_call_total"`
	ExecutionMinutes  int    `json:"execution_minutes"`
	HTTPFlowCount     int    `json:"http_flow_count"`
	RiskCount         int    `json:"risk_count"`
	ModifiedFileCount int    `json:"modified_file_count"`
}

type SessionSnapshotPerception struct {
	Summary      string  `json:"summary"`
	Topics       []string `json:"topics,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	Confidence   float64 `json:"confidence"`
	Changed      bool    `json:"changed"`
	Epoch        int     `json:"epoch"`
	LastTrigger  string  `json:"last_trigger"`
	IntentShift  string  `json:"intent_shift,omitempty"`
	LastUpdateAt int64   `json:"last_update_at"`

	CapabilityMatches *SessionSnapshotPerceptionCapabilityMatches `json:"capability_matches,omitempty"`
	Knowledge         *SessionSnapshotPerceptionKnowledge         `json:"knowledge,omitempty"`
}

type SessionSnapshotPerceptionCapabilityMatches struct {
	Query                    string   `json:"query"`
	MatchedToolNames         []string `json:"matched_tool_names,omitempty"`
	MatchedForgeNames        []string `json:"matched_forge_names,omitempty"`
	MatchedSkillNames        []string `json:"matched_skill_names,omitempty"`
	MatchedFocusModeNames    []string `json:"matched_focus_mode_names,omitempty"`
	RecommendedCapabilities  []string `json:"recommended_capabilities,omitempty"`
}

type SessionSnapshotPerceptionKnowledge struct {
	Query           string   `json:"query"`
	KnowledgeBases  []string `json:"knowledge_bases,omitempty"`
	Content         string   `json:"content"`
}

type sessionSnapshotPerceptionExtras struct {
	CapabilityMatches *SessionSnapshotPerceptionCapabilityMatches
	Knowledge         *SessionSnapshotPerceptionKnowledge
}

type sessionExecutionTracker struct {
	mu              sync.Mutex
	stats           SessionSnapshotExecution
	callToolIDs     map[string]struct{}
}

type sessionSnapshotState struct {
	mu                       sync.Mutex
	revision                 int64
	perceptionExtras         sessionSnapshotPerceptionExtras
	execution                sessionExecutionTracker
	emitHandler              sessionSnapshotEmitHandler
	debounceTimer            *time.Timer
	legacySeparateEvents     bool
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
	state.execution.stats = SessionSnapshotExecution{
		TaskName:  strings.TrimSpace(taskName),
		Status:    strings.TrimSpace(status),
		StartedAt: startedAt.Unix(),
	}
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
	state.execution.stats.EndedAt = endedAt.Unix()
	if !endedAt.IsZero() && state.execution.stats.StartedAt > 0 {
		state.execution.stats.ExecutionMinutes = int(endedAt.Sub(time.Unix(state.execution.stats.StartedAt, 0)).Minutes())
	}
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
	c.refreshSessionSnapshotDurationLocked(state)
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
	c.refreshSessionSnapshotDurationLocked(state)
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
	c.refreshSessionSnapshotDurationLocked(state)

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

func (c *Config) refreshSessionSnapshotDurationLocked(state *sessionSnapshotState) {
	if state == nil || state.execution.stats.StartedAt <= 0 {
		return
	}
	endAt := time.Now()
	if state.execution.stats.EndedAt > 0 {
		endAt = time.Unix(state.execution.stats.EndedAt, 0)
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
		return nil
	}
	stats := &SessionSnapshotExecution{
		TaskName: strings.TrimSpace(task.GetName()),
		Status:   "processing",
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
	if stats.ToolCallTotal == 0 && stats.TaskName == "" {
		return nil
	}
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
