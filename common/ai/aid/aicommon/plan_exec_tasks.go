package aicommon

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const PlanExecPhaseDetachedPendingApproval = "plan_pending_approval"

func planExecPhaseFromProgress(taskProgress string) string {
	taskProgress = strings.TrimSpace(taskProgress)
	if taskProgress == "" {
		return ""
	}
	var progress map[string]any
	if err := json.Unmarshal([]byte(taskProgress), &progress); err != nil {
		return ""
	}
	return strings.TrimSpace(utils.InterfaceToString(progress["phase"]))
}

// ShouldExposePlanExecTaskRecord reports whether a persisted plan-exec row should
// appear in plan_exec_tasks sync responses. Detached plans stay hidden until the
// user approves execution and the phase moves off plan_pending_approval.
func ShouldExposePlanExecTaskRecord(taskProgress string) bool {
	return planExecPhaseFromProgress(taskProgress) != PlanExecPhaseDetachedPendingApproval
}
