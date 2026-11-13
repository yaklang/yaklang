package aicommon

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

// AsyncExecutionSnapshot is the session-wide background execution view shared
// by runtime push events and sync responses.
type AsyncExecutionSnapshot struct {
	Revision      uint64                     `json:"revision"`
	SessionID     string                     `json:"session_id,omitempty"`
	CurrentTaskID string                     `json:"current_task_id,omitempty"`
	RunningCount  int                        `json:"running_count"`
	UpdatedAt     int64                      `json:"updated_at"`
	Executions    []asyncexecmanager.Summary `json:"executions"`
	Execution     *asyncexecmanager.Summary  `json:"execution,omitempty"`
	Error         string                     `json:"error,omitempty"`
}

func BuildAsyncExecutionSnapshot(overview asyncexecmanager.Overview, sessionID, currentTaskID string) AsyncExecutionSnapshot {
	executions := overview.Executions
	if executions == nil {
		executions = make([]asyncexecmanager.Summary, 0)
	}
	return AsyncExecutionSnapshot{
		Revision:      overview.Revision,
		SessionID:     sessionID,
		CurrentTaskID: currentTaskID,
		RunningCount:  overview.RunningCount(),
		UpdatedAt:     time.Now().UnixMilli(),
		Executions:    executions,
	}
}
