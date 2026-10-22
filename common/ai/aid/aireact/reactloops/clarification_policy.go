package reactloops

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func (r *ReActLoop) clarificationTaskID() string {
	if task := r.GetCurrentTask(); task != nil {
		return task.GetId()
	}
	return ""
}

func (r *ReActLoop) CheckClarificationAllowed(blocking bool) error {
	if r.IsSubAgent() || (r.GetCurrentTask() != nil && r.GetCurrentTask().IsSubAgent()) {
		return fmt.Errorf("subagents must report missing input to their parent, not question the user directly")
	}
	if r.allowUserInteract != nil && !r.allowUserInteract() {
		return fmt.Errorf("user clarification is disabled for this loop")
	}
	if cfg, ok := r.GetConfig().(*aicommon.Config); ok {
		return cfg.CheckClarificationAllowed(r.clarificationTaskID(), blocking)
	}
	return nil
}

func (r *ReActLoop) actionAllowed(name string) bool {
	if !aicommon.IsReActActionAllowed(r.GetConfig(), r.loopName, name) {
		return false
	}
	switch name {
	case "ask_for_clarification":
		return r.CheckClarificationAllowed(true) == nil
	case "ask_for_clarification_async":
		return r.CheckClarificationAllowed(false) == nil
	}
	return true
}
