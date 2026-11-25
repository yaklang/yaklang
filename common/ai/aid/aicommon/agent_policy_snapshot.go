package aicommon

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const AgentPolicySnapshotNodeID = "agent_policy_snapshot"
const AgentPolicySnapshotSchemaVersion = 1
const SYNC_TYPE_AGENT_POLICY_SNAPSHOT = "agent_policy_snapshot_sync"

const (
	GoalTimeLimitModeDisabled    = "disabled"
	GoalTimeLimitModeNotStarted  = "not_started"
	GoalTimeLimitModeDeadline    = "deadline"
	GoalTimeLimitModeNeverEnding = "never_ending"
)

type AgentPolicySnapshot struct {
	SchemaVersion int                           `json:"schema_version"`
	Revision      uint64                        `json:"revision"`
	UpdatedAt     int64                         `json:"updated_at"`
	ServerTime    int64                         `json:"server_time"`
	Goal          AgentGoalPolicySnapshot       `json:"goal"`
	MultiAgent    AgentMultiAgentPolicySnapshot `json:"multi_agent"`
	Plan          AgentPlanPolicySnapshot       `json:"plan"`
}

type AgentGoalPolicySnapshot struct {
	Enabled            bool                       `json:"enabled"`
	Effective          bool                       `json:"effective"`
	MinIterations      int64                      `json:"min_iterations"`
	AcceptanceCriteria string                     `json:"acceptance_criteria"`
	TimeLimit          AgentGoalTimeLimitSnapshot `json:"time_limit"`
}

type AgentGoalTimeLimitSnapshot struct {
	Enabled                   bool   `json:"enabled"`
	Mode                      string `json:"mode"`
	DeadlineAt                int64  `json:"deadline_at"`
	ConfiguredDurationSeconds int64  `json:"configured_duration_seconds"`
	Revision                  uint64 `json:"revision"`
}

type AgentMultiAgentPolicySnapshot struct {
	Enabled        bool  `json:"enabled"`
	Effective      bool  `json:"effective"`
	PreferDispatch bool  `json:"prefer_dispatch"`
	MaxConcurrency int64 `json:"max_concurrency"`
}

type AgentPlanPolicySnapshot struct {
	Enabled             bool  `json:"enabled"`
	Effective           bool  `json:"effective"`
	Detached            bool  `json:"detached"`
	TaskConcurrency     int   `json:"task_concurrency"`
	AllowUserInteract   bool  `json:"allow_user_interact"`
	MaxUserInteractions int64 `json:"max_user_interactions"`
	AllowSubagents      bool  `json:"allow_subagents"`
}

func (c *Config) BuildAgentPolicySnapshot() *AgentPolicySnapshot {
	if c == nil {
		return nil
	}
	if c.m == nil {
		c.m = new(sync.Mutex)
	}
	now := time.Now()
	c.m.Lock()
	defer c.m.Unlock()

	c.agentPolicyRevision++
	goalTimeMode := GoalTimeLimitModeDisabled
	goalDeadlineAt := int64(0)
	switch {
	case c.GoalDurationSeconds == -1:
		goalTimeMode = GoalTimeLimitModeNeverEnding
	case c.GoalDurationSeconds > 0 && c.GoalDeadline.IsZero():
		goalTimeMode = GoalTimeLimitModeNotStarted
	case c.GoalDurationSeconds > 0:
		goalTimeMode = GoalTimeLimitModeDeadline
		goalDeadlineAt = c.GoalDeadline.Unix()
	}

	return &AgentPolicySnapshot{
		SchemaVersion: AgentPolicySnapshotSchemaVersion,
		Revision:      c.agentPolicyRevision,
		UpdatedAt:     now.Unix(),
		ServerTime:    now.Unix(),
		Goal: AgentGoalPolicySnapshot{
			Enabled:            c.EnableGoalMode,
			Effective:          c.EnableGoalMode,
			MinIterations:      NormalizeGoalMinIterations(c.GoalMinIterations),
			AcceptanceCriteria: c.GoalAcceptanceCriteria,
			TimeLimit: AgentGoalTimeLimitSnapshot{
				Enabled:                   c.GoalDurationSeconds != 0,
				Mode:                      goalTimeMode,
				DeadlineAt:                goalDeadlineAt,
				ConfiguredDurationSeconds: c.GoalDurationSeconds,
				Revision:                  c.goalTimeRevision,
			},
		},
		MultiAgent: AgentMultiAgentPolicySnapshot{
			Enabled:        c.EnableDispatchSubReactAgents,
			Effective:      c.EnableDispatchSubReactAgents,
			PreferDispatch: c.PreferDispatchSubReactAgents,
			MaxConcurrency: normalizeMaxSubAgentsLocked(c.MaxSubAgents),
		},
		Plan: AgentPlanPolicySnapshot{
			Enabled:             c.EnablePlanAndExec,
			Effective:           c.EnablePlanAndExec,
			Detached:            c.EnableDetachedPlan,
			TaskConcurrency:     normalizePlanExecTaskConcurrencyLocked(c.PlanExecTaskConcurrency),
			AllowUserInteract:   c.AllowPlanUserInteract,
			MaxUserInteractions: c.PlanUserInteractMaxCount,
			AllowSubagents:      c.EnableSubagentsInPlan,
		},
	}
}

func normalizeMaxSubAgentsLocked(n int64) int64 {
	if n <= 0 {
		return DefaultMaxSubAgentConcurrency
	}
	if n > AbsoluteMaxSubAgentConcurrency {
		return AbsoluteMaxSubAgentConcurrency
	}
	return n
}

func normalizePlanExecTaskConcurrencyLocked(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func (c *Config) EmitAgentPolicySnapshot() {
	if c == nil || c.Emitter == nil {
		return
	}
	snapshot := c.BuildAgentPolicySnapshot()
	if snapshot == nil {
		return
	}
	_, _ = c.Emitter.EmitSystemJSON(schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT, AgentPolicySnapshotNodeID, snapshot)
}

func (c *Config) SetAgentPolicySnapshotEmitHandler(handler func()) {
	if c == nil {
		return
	}
	c.m.Lock()
	c.agentPolicyEmitHandler = handler
	c.m.Unlock()
}

func (c *Config) notifyAgentPolicySnapshotEmit() {
	if c == nil {
		return
	}
	c.m.Lock()
	handler := c.agentPolicyEmitHandler
	c.m.Unlock()
	if handler != nil {
		handler()
	}
}

func (c *Config) agentPolicySnapshotEmitOption() ConfigOption {
	return func(target *Config) error {
		if target == c {
			c.EmitAgentPolicySnapshot()
		}
		return nil
	}
}

func (c *Config) HandleSyncAgentPolicySnapshotEvent(event *ypb.AIInputEvent) error {
	snapshot := c.BuildAgentPolicySnapshot()
	c.EmitSyncJSON(schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT, AgentPolicySnapshotNodeID, snapshot, event.GetSyncID())
	return nil
}
