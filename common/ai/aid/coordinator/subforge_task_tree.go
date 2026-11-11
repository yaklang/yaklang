package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// subForgeTaskTree is the display snapshot for one admitted child invocation.
// TaskTree is the latest child plan projection, including its task progress.
type subForgeTaskTree struct {
	ParentCoordinatorID string          `json:"parent_coordinator_id"`
	ParentTaskID        string          `json:"parent_task_id"`
	ParentTaskUUID      string          `json:"parent_task_uuid"`
	CallToolID          string          `json:"call_tool_id"`
	ChildCoordinatorID  string          `json:"child_coordinator_id"`
	ToolName            string          `json:"tool_name"`
	Revision            uint64          `json:"revision"`
	Status              string          `json:"status"`
	TaskTree            json.RawMessage `json:"task_tree"`
}

func (snapshot *subForgeTaskTree) observePlan(event *schema.AiOutputEvent) bool {
	if event.Type != schema.EVENT_TYPE_PLAN || !event.IsJson || event.IsSync {
		return false
	}
	var plan struct {
		Root json.RawMessage `json:"root_task"`
	}
	if json.Unmarshal(event.Content, &plan) != nil || len(plan.Root) == 0 ||
		bytes.Equal(plan.Root, []byte("null")) || bytes.Equal(plan.Root, snapshot.TaskTree) {
		return false
	}
	snapshot.TaskTree = plan.Root
	snapshot.Revision++
	return true
}

func (s *Session) emitSubForgeTaskTree(snapshot subForgeTaskTree) {
	// Store this as a standalone event under the parent session. The tool call
	// ID stays in the payload so recovery does not depend on a tool-start row.
	if _, err := s.GetEmitter().EmitJSON(schema.EVENT_TYPE_SUB_FORGE_TASK_TREE, "sub-forge-task-tree", snapshot); err != nil {
		log.Warnf("emit sub-Forge task tree: %v", err)
	}
}

func subForgeTreeStatus(err error) string {
	switch {
	case err == nil:
		return "succeeded"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed_out"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "failed"
	}
}
