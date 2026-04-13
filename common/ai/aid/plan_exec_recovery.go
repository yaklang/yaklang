package aid

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type recoveredTask struct {
	Index         string           `json:"index"`
	Name          string           `json:"name"`
	Goal          string           `json:"goal"`
	Progress      string           `json:"progress"`
	Summary       string           `json:"summary"`
	StatusSummary string           `json:"status_summary"`
	TaskSummary   string           `json:"task_summary"`
	ShortSummary  string           `json:"short_summary"`
	LongSummary   string           `json:"long_summary"`
	Subtasks      []*recoveredTask `json:"subtasks,omitempty"`
}

func (c *Coordinator) tryRecoverPlanAndExec(startTaskIndex string) (*AiTask, *PlanAndExecProgress, bool, error) {
	if c == nil {
		return nil, nil, false, nil
	}
	coordinatorID := c.GetRuntimeId()
	if coordinatorID == "" {
		return nil, nil, false, nil
	}
	db := c.GetDB()
	if db == nil {
		return nil, nil, false, nil
	}

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	if err != nil || record == nil {
		return nil, nil, false, nil
	}
	if strings.TrimSpace(record.TaskTree) == "" {
		return nil, nil, false, nil
	}

	var progress PlanAndExecProgress
	if record.TaskProgress != "" {
		if err := json.Unmarshal([]byte(record.TaskProgress), &progress); err != nil {
			log.Warnf("recover plan-exec progress parse failed: %v", err)
		}
	}
	if strings.EqualFold(strings.TrimSpace(progress.Phase), "completed") {
		return nil, &progress, false, nil
	}

	var rootRecovered recoveredTask
	if err := json.Unmarshal([]byte(record.TaskTree), &rootRecovered); err != nil {
		log.Warnf("recover plan-exec task tree parse failed: %v", err)
		return nil, &progress, false, nil
	}

	root := c.buildRecoveredTaskTree(&rootRecovered, nil)
	if root == nil {
		return nil, &progress, false, nil
	}
	if err := prepareRecoveryStartTask(root, startTaskIndex); err != nil {
		return nil, &progress, true, err
	}
	return root, &progress, true, nil
}

func (c *Coordinator) buildRecoveredTaskTree(src *recoveredTask, parent *AiTask) *AiTask {
	if src == nil {
		return nil
	}
	task := c.generateAITaskWithName(src.Name, src.Goal)
	task.Index = src.Index
	task.ParentTask = parent
	if src.StatusSummary != "" {
		task.StatusSummary = src.StatusSummary
	}
	if src.TaskSummary != "" {
		task.TaskSummary = src.TaskSummary
	}
	if src.ShortSummary != "" {
		task.ShortSummary = src.ShortSummary
	}
	if src.LongSummary != "" {
		task.LongSummary = src.LongSummary
	}
	if task.TaskSummary == "" && task.ShortSummary == "" && task.LongSummary == "" && task.StatusSummary == "" && src.Summary != "" {
		task.TaskSummary = src.Summary
	}
	if task.Index != "" {
		task.SetID(task.Index)
	}

	switch strings.ToLower(strings.TrimSpace(src.Progress)) {
	case string(aicommon.AITaskState_Completed):
		task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Completed)
	case string(aicommon.AITaskState_Aborted):
		task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Aborted)
	case string(aicommon.AITaskState_Skipped):
		task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Skipped)
	case string(aicommon.AITaskState_Processing):
		task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Processing)
	}

	for _, sub := range src.Subtasks {
		child := c.buildRecoveredTaskTree(sub, task)
		if child != nil {
			task.Subtasks = append(task.Subtasks, child)
		}
	}
	return task
}

// prepareRecoveryStartTask 会把恢复后的任务树整理成“从指定起点重新执行”的状态。
// 规则很直接：
// 1. 起点之前：已经完成的任务保留 completed，未完成的任务统一标记为 skipped，避免执行器再次进入。
// 2. 起点及之后：无论之前是 completed、aborted 还是 skipped，全部重置为 created，确保会重新执行。
func prepareRecoveryStartTask(root *AiTask, startTaskIndex string) error {
	startTaskIndex = strings.TrimSpace(startTaskIndex)
	if root == nil || startTaskIndex == "" {
		return nil
	}

	taskLink := DFSOrderAiTask(root)
	foundStart := false
	for i := 0; i < taskLink.Len(); i++ {
		task, ok := taskLink.Get(i)
		if !ok || task == nil {
			continue
		}
		if task.Index == startTaskIndex {
			// 命中恢复起点后，当前任务和后续任务都按“待重新执行”处理。
			foundStart = true
			resetTaskForRecovery(task)
			continue
		}
		if foundStart {
			resetTaskForRecovery(task)
			continue
		}
		if task.executed() {
			continue
		}
		task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Skipped)
	}
	if !foundStart {
		return utils.Errorf("recovery start task %q not found", startTaskIndex)
	}
	return nil
}

// resetTaskForRecovery 会把任务恢复成未完成状态，让 runtime 不会因为旧状态而跳过它。
func resetTaskForRecovery(task *AiTask) {
	if task == nil {
		return
	}
	task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Created)
}
