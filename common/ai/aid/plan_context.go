package aid

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func formatTaskPlanEvidenceLabel(task *AiTask) string {
	if task == nil {
		return "当前任务"
	}
	index := strings.TrimSpace(task.GetIndex())
	name := strings.TrimSpace(task.GetName())
	if index == "" && name == "" {
		return "当前任务"
	}
	if index == "" {
		return name
	}
	if name == "" {
		return "子任务 " + index
	}
	return fmt.Sprintf("子任务 %s %s", index, name)
}

func buildVerificationCarryoverEvidenceOps(task *AiTask, reasoning string) []aicommon.EvidenceOperation {
	var ops []aicommon.EvidenceOperation
	taskLabel := formatTaskPlanEvidenceLabel(task)

	reasoning = strings.TrimSpace(reasoning)
	if reasoning != "" {
		ops = append(ops, aicommon.EvidenceOperation{
			Op:      "add",
			ID:      taskEvidenceID("verify", task),
			Content: fmt.Sprintf("[%s] 核实: %s", taskLabel, reasoning),
		})
	}
	return ops
}

func buildSummaryEvidenceOps(task *AiTask, summary string) []aicommon.EvidenceOperation {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	taskLabel := formatTaskPlanEvidenceLabel(task)
	return []aicommon.EvidenceOperation{
		{
			Op:      "add",
			ID:      taskEvidenceID("summary", task),
			Content: fmt.Sprintf("[%s] 总结: %s", taskLabel, summary),
		},
	}
}

// Task IDs survive recovery and index changes. Plan-local indices can repeat
// in the next plan and must not overwrite another task's session evidence.
func taskEvidenceID(kind string, task *AiTask) string {
	id := strings.TrimSpace(task.TaskId)
	if id == "" {
		id = task.GetIndex()
	}
	return kind + "-" + id
}
