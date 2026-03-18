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
	TotalTasks       int    `json:"total_tasks"`
	CompletedTasks   int    `json:"completed_tasks"`
	SkippedTasks     int    `json:"skipped_tasks"`
	AbortedTasks     int    `json:"aborted_tasks"`
	CurrentIndex     int    `json:"current_index"`
	CurrentTaskIndex string `json:"current_task_index"`
	CurrentTask      string `json:"current_task"`
	CurrentGoal      string `json:"current_goal"`
	Phase            string `json:"phase"`
	UpdatedAt        int64  `json:"updated_at"`
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
	currentIndex := 0
	if c.runtime != nil {
		currentIndex = c.runtime.currentProgressIndex()
	}

	progress := &PlanAndExecProgress{
		TotalTasks:     total,
		CompletedTasks: completed,
		SkippedTasks:   skipped,
		AbortedTasks:   aborted,
		CurrentIndex:   currentIndex,
		Phase:          phase,
		UpdatedAt:      time.Now().Unix(),
	}

	if currentTask != nil {
		progress.CurrentTaskIndex = currentTask.Index
		progress.CurrentTask = currentTask.Name
		progress.CurrentGoal = currentTask.Goal
	}
	return progress
}

func countTaskStats(root *AiTask) (total, completed, skipped, aborted int) {
	if root == nil {
		return 0, 0, 0, 0
	}
	taskLink := DFSOrderAiTask(root)
	for i := 0; i < taskLink.Len(); i++ {
		task, ok := taskLink.Get(i)
		if !ok || task == nil {
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
