package aid

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
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

func (c *Coordinator) tryRecoverPlanAndExec() (*AiTask, *PlanAndExecProgress, bool) {
	if c == nil {
		return nil, nil, false
	}
	coordinatorID := c.GetRuntimeId()
	if coordinatorID == "" {
		return nil, nil, false
	}
	db := c.GetDB()
	if db == nil {
		return nil, nil, false
	}

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	if err != nil || record == nil {
		return nil, nil, false
	}
	if strings.TrimSpace(record.TaskTree) == "" {
		return nil, nil, false
	}

	var progress PlanAndExecProgress
	if record.TaskProgress != "" {
		if err := json.Unmarshal([]byte(record.TaskProgress), &progress); err != nil {
			log.Warnf("recover plan-exec progress parse failed: %v", err)
		}
	}
	if strings.EqualFold(strings.TrimSpace(progress.Phase), "completed") {
		return nil, &progress, false
	}

	var rootRecovered recoveredTask
	if err := json.Unmarshal([]byte(record.TaskTree), &rootRecovered); err != nil {
		log.Warnf("recover plan-exec task tree parse failed: %v", err)
		return nil, &progress, false
	}

	root := c.buildRecoveredTaskTree(&rootRecovered, nil)
	if root == nil {
		return nil, &progress, false
	}
	return root, &progress, true
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
