package aid

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// GetPlanStatusForPrompt reads live task states without changing the plan tree
// or execution lifecycle. Definitions and evidence stay in their own contexts.
func (t *AiTask) GetPlanStatusForPrompt() string {
	if t == nil {
		return ""
	}
	root := t
	for root.ParentTask != nil {
		root = root.ParentTask
	}
	nonce := aicommon.PlanScopedNonce(root.GetId(), "plan_status")
	var body strings.Builder
	fmt.Fprintf(&body, "# PLAN STATUS\n<|PLAN_STATUS_%s|>\n", nonce)
	body.WriteString("计划状态为执行器维护的只读快照。只推进 CURRENT TASK，其他任务状态仅供参考。\n\n")
	fmt.Fprintf(&body, "## CURRENT TASK\n- Task: %s %q\n- Status: %s\n",
		t.GetIndex(), t.Name, t.GetStatus())
	var others strings.Builder
	for _, task := range executableLeafTasks(root) {
		if task == t {
			continue
		}
		fmt.Fprintf(&others, "- %s %q: %s\n", task.GetIndex(), task.Name, task.GetStatus())
	}
	if others.Len() > 0 {
		body.WriteString("\n## OTHER TASKS\n")
		body.WriteString(others.String())
	}
	fmt.Fprintf(&body, "<|PLAN_STATUS_END_%s|>", nonce)
	return body.String()
}
