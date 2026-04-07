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

func prepareRecoveryStartTask(root *AiTask, startTaskIndex string) error {
	startTaskIndex = strings.TrimSpace(startTaskIndex)
	if root == nil || startTaskIndex == "" {
		return nil
	}
	if findTaskByIndex(root, startTaskIndex) == nil {
		return utils.Errorf("recovery start task %q not found", startTaskIndex)
	}

	taskLink := DFSOrderAiTask(root)
	for i := 0; i < taskLink.Len(); i++ {
		task, ok := taskLink.Get(i)
		if !ok || task == nil {
			continue
		}
		if task.Index == startTaskIndex {
			return nil
		}
		if isAncestorTaskIndex(task.Index, startTaskIndex) {
			continue
		}
		markTaskTreeSkippedForRecovery(task)
	}
	return nil
}

func isAncestorTaskIndex(ancestorIndex, taskIndex string) bool {
	ancestorIndex = strings.TrimSpace(ancestorIndex)
	taskIndex = strings.TrimSpace(taskIndex)
	if ancestorIndex == "" || taskIndex == "" || ancestorIndex == taskIndex {
		return false
	}
	return strings.HasPrefix(taskIndex, ancestorIndex+"-")
}

func markTaskTreeSkippedForRecovery(task *AiTask) {
	if task == nil {
		return
	}
	for _, sub := range task.Subtasks {
		markTaskTreeSkippedForRecovery(sub)
	}
	if task.executed() || task.GetStatus() == aicommon.AITaskState_Skipped {
		return
	}
	task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Skipped)
}
