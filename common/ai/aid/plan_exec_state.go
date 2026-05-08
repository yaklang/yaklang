package aid

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	Phase_NotCompleted = "NotCompleted" // 未完成
	Phase_Completed    = "Completed"    // 已完成
)

type PlanAndExecProgress struct {
	TotalTasks        int      `json:"total_tasks"`
	CompletedTasks    int      `json:"completed_tasks"`
	SkippedTasks      int      `json:"skipped_tasks"`
	AbortedTasks      int      `json:"aborted_tasks"`
	TotalStages       int      `json:"total_stages"`
	CompletedStages   int      `json:"completed_stages"`
	CurrentStage      int      `json:"current_stage"`
	CurrentIndex      int      `json:"current_index"`
	CurrentTaskIndex  string   `json:"current_task_index"`
	CurrentTask       string   `json:"current_task"`
	CurrentGoal       string   `json:"current_goal"`
	ActiveTaskIndexes []string `json:"active_task_indexes,omitempty"`
	Phase             string   `json:"phase"`
	UpdatedAt         int64    `json:"updated_at"`
}

func (c *Coordinator) savePlanAndExecState(phase string, currentTask *AiTask) {
	if c == nil {
		return
	}
	if c.PersistentSessionId == "" {
		return
	}
	db := c.GetDB()
	if db == nil {
		return
	}

	root := c.rootTask
	if root == nil && c.runtime != nil {
		root = c.runtime.RootTask
	}
	if root == nil {
		return
	}

	progress := c.buildPlanAndExecProgress(root, currentTask, phase)
	record := &schema.AISessionPlanAndExec{
		SessionID:     c.PersistentSessionId,
		CoordinatorID: c.GetRuntimeId(),
		TaskTree:      string(utils.Jsonify(root)),
		TaskProgress:  string(utils.Jsonify(progress)),
	}
	if err := yakit.CreateOrUpdateAISessionPlanAndExec(db, record); err != nil {
		log.Warnf("save plan-exec state failed: %v", err)
	}
}

func (c *Coordinator) buildPlanAndExecProgress(root *AiTask, currentTask *AiTask, phase string) *PlanAndExecProgress {
	if root == nil {
		return &PlanAndExecProgress{
			Phase:     phase,
			UpdatedAt: time.Now().Unix(),
		}
	}

	total, completed, skipped, aborted := countTaskStats(root)
	snapshot := runtimeProgressSnapshot{}
	if c.runtime != nil {
		snapshot = c.runtime.progressSnapshot()
	}

	progress := &PlanAndExecProgress{
		TotalTasks:        total,
		CompletedTasks:    completed,
		SkippedTasks:      skipped,
		AbortedTasks:      aborted,
		TotalStages:       snapshot.totalStages,
		CompletedStages:   snapshot.completedStages,
		CurrentStage:      snapshot.currentStage,
		CurrentIndex:      snapshot.currentIndex,
		ActiveTaskIndexes: snapshot.activeTaskIDs,
		Phase:             phase,
		UpdatedAt:         time.Now().Unix(),
	}

	if c.runtime != nil {
		if representative := c.runtime.representativeTask(); representative != nil {
			progress.CurrentTaskIndex = representative.Index
			progress.CurrentTask = representative.Name
			progress.CurrentGoal = representative.Goal
		}
	}
	if currentTask != nil {
		progress.CurrentTaskIndex = currentTask.Index
		progress.CurrentTask = currentTask.Name
		progress.CurrentGoal = currentTask.Goal
	} else if progress.CurrentTaskIndex == "" && snapshot.currentTaskIndex != "" {
		progress.CurrentTaskIndex = snapshot.currentTaskIndex
	}
	return progress
}

func countTaskStats(root *AiTask) (total, completed, skipped, aborted int) {
	if root == nil {
		return 0, 0, 0, 0
	}
	for _, task := range executableLeafTasks(root) {
		if task == nil {
			continue
		}
		total++
		switch task.GetStatus() {
		case aicommon.AITaskState_Skipped:
			skipped++
			continue
		case aicommon.AITaskState_Aborted:
			aborted++
			continue
		}
		if task.executed() {
			completed++
		}
	}
	return total, completed, skipped, aborted
}
