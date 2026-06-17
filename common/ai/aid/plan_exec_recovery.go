package aid

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/workflowdag"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type recoveredTask struct {
	TaskId             string           `json:"task_id,omitempty"`
	Index              string           `json:"index"`
	Name               string           `json:"name"`
	Goal               string           `json:"goal"`
	SemanticIdentifier string           `json:"semantic_identifier,omitempty"`
	DependsOn          []string         `json:"depends_on,omitempty"`
	Progress           string           `json:"progress"`
	Summary            string           `json:"summary"`
	StatusSummary      string           `json:"status_summary"`
	TaskSummary        string           `json:"task_summary"`
	ShortSummary       string           `json:"short_summary"`
	LongSummary        string           `json:"long_summary"`
	Subtasks           []*recoveredTask `json:"subtasks,omitempty"`
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
	c.standardizeTaskTree(root)
	effectiveStartTaskIndex := strings.TrimSpace(startTaskIndex)
	if effectiveStartTaskIndex == "" {
		effectiveStartTaskIndex = strings.TrimSpace(progress.CurrentTaskIndex)
		if effectiveStartTaskIndex == "" && len(progress.ActiveTaskIndexes) > 0 {
			effectiveStartTaskIndex = strings.TrimSpace(progress.ActiveTaskIndexes[0])
		}
	}
	if err := prepareRecoveryStartTask(root, effectiveStartTaskIndex); err != nil {
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
	task.DependsOn = normalizeDependencyRefs(src.DependsOn)
	if src.SemanticIdentifier != "" {
		task.SetSemanticIdentifier(src.SemanticIdentifier)
	}
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
	restoreRecoveredTaskID(task, src)

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

func restoreRecoveredTaskID(task *AiTask, src *recoveredTask) {
	if task == nil || src == nil {
		return
	}
	taskID := strings.TrimSpace(src.TaskId)
	if taskID == "" && strings.TrimSpace(src.Index) != "" {
		// Backward compatibility for task trees persisted before task_id was saved.
		taskID = fmt.Sprintf("pe-task-%s", src.Index)
	}
	if taskID == "" {
		return
	}
	task.TaskId = taskID
	if task.AIStatefulTaskBase != nil {
		task.SetID(taskID)
	}
}

func prepareRecoveryStartTask(root *AiTask, startTaskIndex string) error {
	startTaskIndex = strings.TrimSpace(startTaskIndex)
	if root == nil {
		return nil
	}

	graph, err := buildStrictExecutableTaskGraph(root)
	if err != nil {
		return err
	}

	if startTaskIndex == "" {
		if autoTaskIndex, ok := locateAutoRecoveryTask(graph); ok {
			startTaskIndex = autoTaskIndex
		} else {
			return nil
		}
	}
	targetOrder, err := locateRecoveryTaskOrder(root, graph, startTaskIndex)
	if err != nil {
		return err
	}

	for _, task := range graph.order {
		if task == nil || task.Index == "" {
			continue
		}
		order, ok := graph.OrderOf(task.Index)
		if !ok {
			continue
		}
		if order < targetOrder {
			if task.executed() {
				continue
			}
			task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Skipped)
			continue
		}
		resetTaskForRecovery(task)
	}
	return nil
}

func locateAutoRecoveryTask(graph *executableTaskGraph) (string, bool) {
	if graph == nil {
		return "", false
	}
	for _, task := range graph.order {
		if task == nil {
			continue
		}
		if task.executed() || task.GetStatus() == aicommon.AITaskState_Skipped {
			continue
		}
		return task.Index, true
	}
	return "", false
}

func locateRecoveryTaskOrder(root *AiTask, graph *executableTaskGraph, startTaskIndex string) (int, error) {
	if graph == nil {
		return 0, workflowdag.ErrEmptyDAG
	}

	startTaskIndex = strings.TrimSpace(startTaskIndex)
	if order, ok := graph.OrderOf(startTaskIndex); ok {
		return order, nil
	}

	references := buildTaskReferenceMap(root)
	targetIndex, ok := references[startTaskIndex]
	if !ok {
		return 0, utils.Errorf("recovery start task %q not found", startTaskIndex)
	}
	if order, ok := graph.OrderOf(targetIndex); ok {
		return order, nil
	}
	return 0, utils.Errorf("recovery start task %q is not an executable task node", startTaskIndex)
}

// resetTaskForRecovery 会把任务恢复成未完成状态，让 runtime 不会因为旧状态而跳过它。
func resetTaskForRecovery(task *AiTask) {
	if task == nil {
		return
	}
	task.AIStatefulTaskBase.RestoreStatus(aicommon.AITaskState_Created)
}
