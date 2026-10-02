package coordinator_legacy

import (
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

func buildPlanExecutionSnapshot(c *Coordinator, planHolder aicommon.AIStatefulTask) *aid.PlanExecutionRuntimeSnapshot {
	if c == nil || c.Config == nil {
		return nil
	}
	snapshot := &aid.PlanExecutionRuntimeSnapshot{
		CoordinatorID: c.Config.Id,
	}
	if planHolder != nil {
		snapshot.AsyncReactTaskID = planHolder.GetId()
	}
	if c.runtime != nil {
		progress := c.runtime.progressSnapshot()
		snapshot.CurrentStage = progress.currentStage
		snapshot.ActiveTaskIDs = append([]string(nil), progress.activeTaskIDs...)
		snapshot.CurrentTaskID = progress.currentTaskID
		snapshot.TotalTasks = progress.totalTasks
		snapshot.CompletedTasks = progress.currentIndex - len(progress.activeTaskIDs)
		if snapshot.CompletedTasks < 0 {
			snapshot.CompletedTasks = 0
		}
	}
	root := c.rootTask
	if root == nil && c.runtime != nil {
		root = c.runtime.RootTask
	}
	if root != nil {
		snapshot.RootTaskName = root.Name
		activeSet := make(map[string]struct{}, len(snapshot.ActiveTaskIDs))
		for _, id := range snapshot.ActiveTaskIDs {
			activeSet[id] = struct{}{}
		}
		walkAiTaskTree(root, "", func(task *AiTask, parentIndex string) {
			if task == nil {
				return
			}
			_, inActive := activeSet[task.TaskId]
			entry := buildAiTaskEntry("plan_exec", snapshot.CoordinatorID, reportReActIDFromCoordinator(c), task, parentIndex, inActive || task.executing())
			snapshot.Tasks = append(snapshot.Tasks, entry)
		})
	}
	return snapshot
}

func reportReActIDFromCoordinator(c *Coordinator) string {
	if c == nil || c.Config == nil {
		return ""
	}
	return c.Config.Id
}

func buildAiTaskEntry(scope, coordinatorID, reactID string, task *AiTask, parentIndex string, forceExecuting bool) aid.TaskRuntimeEntry {
	entry := aid.TaskRuntimeEntry{
		Scope:         scope,
		ReActID:       reactID,
		CoordinatorID: coordinatorID,
		TaskIndex:     task.Index,
		Name:          task.Name,
		Goal:          utils.ShrinkString(task.Goal, 240),
		ParentIndex:   parentIndex,
		Executing:     forceExecuting || task.executing(),
	}
	if task.AIStatefulTaskBase != nil {
		entry.TaskID = task.GetId()
		entry.Status = string(task.GetStatus())
		entry.AsyncMode = task.IsAsyncMode()
		entry.Executing = entry.Executing || task.GetStatus() == aicommon.AITaskState_Processing
		if name := task.GetName(); name != "" && entry.Name == "" {
			entry.Name = name
		}
	} else {
		entry.TaskID = task.Index
		if entry.Name == "" {
			entry.Name = task.Index
		}
	}
	entry = enrichAiTaskEntry(entry, task, parentIndex)
	return entry
}

func enrichAiTaskEntry(entry aid.TaskRuntimeEntry, task *AiTask, parentIndex string) aid.TaskRuntimeEntry {
	if task == nil {
		return entry
	}
	if entry.TaskIndex == "" {
		entry.TaskIndex = task.Index
	}
	if parentIndex != "" {
		entry.ParentIndex = parentIndex
	} else if task.ParentTask != nil {
		entry.ParentIndex = task.ParentTask.Index
	}
	entry.HasTimelineFork = task.timelineFork != nil && task.timelineFork.Branch != nil
	timeline := task.CurrentTimeline()
	if timeline == nil && task.Coordinator != nil && task.Coordinator.Config != nil {
		timeline = task.Coordinator.Config.GetTimeline()
	}
	entry.RecentTextOutputs = aid.RecentTaskTextOutputs(timeline, 10)
	return entry
}

func walkAiTaskTree(task *AiTask, parentIndex string, visit func(task *AiTask, parentIndex string)) {
	if task == nil || visit == nil {
		return
	}
	visit(task, parentIndex)
	for _, sub := range task.Subtasks {
		walkAiTaskTree(sub, task.Index, visit)
	}
}

func CollectPlanExecutionSnapshots(holder aicommon.AIStatefulTask) []aid.PlanExecutionRuntimeSnapshot {
	var result []aid.PlanExecutionRuntimeSnapshot
	for _, c := range snapshotRunningCoordinators() {
		if snapshot := buildPlanExecutionSnapshot(c, holder); snapshot != nil {
			result = append(result, *snapshot)
		}
	}
	return result
}

func (t *AiTask) EnrichTaskRuntimeEntry(entry aid.TaskRuntimeEntry) aid.TaskRuntimeEntry {
	return enrichAiTaskEntry(entry, t, "")
}
