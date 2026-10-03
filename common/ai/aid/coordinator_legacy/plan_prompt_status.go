package coordinator_legacy

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
	body.WriteString("PLAN 任务状态由执行器维护，只推进 CURRENT PLAN TASK；created 表示 PLAN 任务未开始，queueing 表示等待执行。\n\n")
	fmt.Fprintf(&body, "## CURRENT PLAN TASK\n- Task: %s %q\n- Status: %s\n",
		t.GetIndex(), t.Name, t.GetStatus())
	var others strings.Builder
	for _, task := range executableLeafTasks(root) {
		if task == t {
			continue
		}
		fmt.Fprintf(&others, "- %s %q: %s\n", task.GetIndex(), task.Name, task.GetStatus())
	}
	if others.Len() > 0 {
		body.WriteString("\n## OTHER PLAN TASKS\n")
		body.WriteString(others.String())
	}
	fmt.Fprintf(&body, "<|PLAN_STATUS_END_%s|>", nonce)
	return body.String()
}
