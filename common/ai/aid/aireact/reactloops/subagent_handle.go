package reactloops

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// registerHandle creates a SubAgentHandle for the given sub-task and registers
// it in the registry, returning the handle (or nil when registry is nil). The
// returned handle.SubLoop field must be set by the caller once the sub-loop is
// created. Callers MUST call unregisterHandle(...) afterwards to remove the
// handle and mark it finished when the sub-loop completes or fails.
func registerHandle(
	registry *ProgressRegistry,
	subTaskID, identifier string,
	subTask aicommon.AIStatefulTask,
	startedAt time.Time,
) *SubAgentHandle {
	if registry == nil {
		return nil
	}
	return registry.Register(NewSubAgentHandle(subTaskID, identifier, subTask, startedAt))
}

// unregisterHandle removes the handle from the registry (if any) and marks it
// finished with the given error. Safe to call with a nil handle / nil registry.
func unregisterHandle(handle *SubAgentHandle, registry *ProgressRegistry, subTaskID string, execErr error) {
	if handle == nil || registry == nil {
		return
	}
	registry.Unregister(subTaskID, execErr)
}

// deriveScopeName picks a non-empty display name from the supplied candidates,
// falling back to the given default. Each candidate is trimmed before use.
// Used by nested-loop helpers (RunNestedLoop / runNestedInPlace) to derive a
// sub-task scope name from job.TaskName / job.Identifier / job.LoopName.
func deriveScopeName(def string, candidates ...string) string {
	for _, c := range candidates {
		if s := strings.TrimSpace(c); s != "" {
			return s
		}
	}
	return def
}
