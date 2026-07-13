package aid

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const defaultRecentTextOutputLimit = 10

// TaskRuntimeReportProvider is implemented by the main ReAct invoker.
type TaskRuntimeReportProvider interface {
	CollectTaskRuntimeReport() *TaskRuntimeReport
}

// ReactRuntimeSource exposes ReAct session task state for runtime inspection.
type ReactRuntimeSource interface {
	GetReActID() string
	GetRuntimeTasks() []aicommon.AIStatefulTask
	GetCurrentTask() aicommon.AIStatefulTask
	GetCurrentPlanExecutionTask() aicommon.AIStatefulTask
	GetQueueingTasks() []aicommon.AIStatefulTask
	GetSessionTimeline() *aicommon.Timeline
}

// TaskTextOutput is one recent text line from a task timeline.
type TaskTextOutput struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Content   string `json:"content"`
}

// TaskRuntimeEntry describes one task row in the runtime report.
type TaskRuntimeEntry struct {
	Scope             string           `json:"scope"`
	ReActID           string           `json:"react_id,omitempty"`
	CoordinatorID     string           `json:"coordinator_id,omitempty"`
	TaskID            string           `json:"task_id"`
	TaskIndex         string           `json:"task_index,omitempty"`
	Name              string           `json:"name,omitempty"`
	Status            string           `json:"status"`
	AsyncMode         bool             `json:"async_mode"`
	Executing         bool             `json:"executing"`
	LoopName          string           `json:"loop_name,omitempty"`
	ParentIndex       string           `json:"parent_index,omitempty"`
	Goal              string           `json:"goal,omitempty"`
	HasTimelineFork   bool             `json:"has_timeline_fork"`
	RecentTextOutputs []TaskTextOutput `json:"recent_text_outputs"`
}

// PlanExecutionRuntimeSnapshot summarizes one running plan-and-execute coordinator.
type PlanExecutionRuntimeSnapshot struct {
	CoordinatorID     string             `json:"coordinator_id"`
	RootTaskName      string             `json:"root_task_name,omitempty"`
	CurrentStage      int                `json:"current_stage"`
	ActiveTaskIndexes []string           `json:"active_task_indexes"`
	CurrentTaskIndex  string             `json:"current_task_index,omitempty"`
	TotalTasks        int                `json:"total_tasks"`
	CompletedTasks    int                `json:"completed_tasks"`
	AsyncReactTaskID  string             `json:"async_react_task_id,omitempty"`
	Tasks             []TaskRuntimeEntry `json:"tasks"`
}

// TaskRuntimeReport is the payload returned by list_async_tasks.
type TaskRuntimeReport struct {
	GeneratedAt      string                         `json:"generated_at"`
	ReActID          string                         `json:"react_id,omitempty"`
	AsyncTasks       []TaskRuntimeEntry             `json:"async_tasks"`
	ExecutingTasks   []TaskRuntimeEntry             `json:"executing_tasks"`
	QueuedReactTasks []TaskRuntimeEntry             `json:"queued_react_tasks"`
	PlanExecutions   []PlanExecutionRuntimeSnapshot `json:"plan_executions"`
}

// BuildTaskRuntimeReport aggregates async / executing tasks and recent timeline text.
func BuildTaskRuntimeReport(source ReactRuntimeSource) *TaskRuntimeReport {
	report := &TaskRuntimeReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
	}
	if source == nil {
		return report
	}
	report.ReActID = source.GetReActID()
	sessionTimeline := source.GetSessionTimeline()

	seen := make(map[string]struct{})
	var asyncTasks []TaskRuntimeEntry
	var executingTasks []TaskRuntimeEntry
	var queuedTasks []TaskRuntimeEntry

	addUnique := func(bucket *[]TaskRuntimeEntry, entry TaskRuntimeEntry) {
		key := entry.Scope + "|" + entry.CoordinatorID + "|" + entry.TaskID
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		*bucket = append(*bucket, entry)
	}

	recordReactTask := func(task aicommon.AIStatefulTask, scope string, forceExecuting bool) {
		if task == nil {
			return
		}
		entry := buildStatefulTaskEntry(scope, report.ReActID, "", task, sessionTimeline, forceExecuting)
		if entry.AsyncMode {
			addUnique(&asyncTasks, entry)
		}
		if entry.Executing {
			addUnique(&executingTasks, entry)
		}
	}

	recordReactTask(source.GetCurrentPlanExecutionTask(), "react_plan_execution_holder", false)
	if t := source.GetCurrentPlanExecutionTask(); t != nil && t.IsAsyncMode() {
		recordReactTask(t, "react_async_plan_trigger", true)
	}
	recordReactTask(source.GetCurrentTask(), "react_current", true)
	for _, task := range source.GetRuntimeTasks() {
		recordReactTask(task, "react_runtime", false)
	}
	for _, task := range source.GetQueueingTasks() {
		if task == nil {
			continue
		}
		entry := buildStatefulTaskEntry("react_queue", report.ReActID, "", task, sessionTimeline, false)
		addUnique(&queuedTasks, entry)
	}

	for _, coordinator := range snapshotRunningCoordinators() {
		peSnapshot := buildPlanExecutionSnapshot(coordinator, source.GetCurrentPlanExecutionTask())
		if peSnapshot == nil {
			continue
		}
		report.PlanExecutions = append(report.PlanExecutions, *peSnapshot)
		for _, entry := range peSnapshot.Tasks {
			if entry.AsyncMode {
				addUnique(&asyncTasks, entry)
			}
			if entry.Executing {
				addUnique(&executingTasks, entry)
			}
		}
	}

	report.AsyncTasks = asyncTasks
	report.ExecutingTasks = executingTasks
	report.QueuedReactTasks = queuedTasks
	return report
}

func buildPlanExecutionSnapshot(c *Coordinator, planHolder aicommon.AIStatefulTask) *PlanExecutionRuntimeSnapshot {
	if c == nil || c.Config == nil {
		return nil
	}
	snapshot := &PlanExecutionRuntimeSnapshot{
		CoordinatorID: c.Config.Id,
	}
	if planHolder != nil {
		snapshot.AsyncReactTaskID = planHolder.GetId()
	}
	if c.runtime != nil {
		progress := c.runtime.progressSnapshot()
		snapshot.CurrentStage = progress.currentStage
		snapshot.ActiveTaskIndexes = append([]string(nil), progress.activeTaskIDs...)
		snapshot.CurrentTaskIndex = progress.currentTaskIndex
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
		activeSet := make(map[string]struct{}, len(snapshot.ActiveTaskIndexes))
		for _, idx := range snapshot.ActiveTaskIndexes {
			activeSet[idx] = struct{}{}
		}
		walkAiTaskTree(root, "", func(task *AiTask, parentIndex string) {
			if task == nil {
				return
			}
			_, inActive := activeSet[task.Index]
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

func buildStatefulTaskEntry(scope, reactID, coordinatorID string, task aicommon.AIStatefulTask, fallbackTimeline *aicommon.Timeline, forceExecuting bool) TaskRuntimeEntry {
	entry := TaskRuntimeEntry{
		Scope:         scope,
		ReActID:       reactID,
		CoordinatorID: coordinatorID,
		TaskID:        task.GetId(),
		Status:        string(task.GetStatus()),
		AsyncMode:     task.IsAsyncMode(),
		Executing:     forceExecuting || task.GetStatus() == aicommon.AITaskState_Processing,
	}
	if name := task.GetName(); name != "" {
		entry.Name = name
	}
	if peTask, ok := task.(*AiTask); ok {
		entry = enrichAiTaskEntry(entry, peTask, "")
	} else {
		entry.RecentTextOutputs = recentTextOutputsFromTimeline(fallbackTimeline, defaultRecentTextOutputLimit)
	}
	if entry.AsyncMode && !task.IsFinished() {
		entry.Executing = true
	}
	return entry
}

func buildAiTaskEntry(scope, coordinatorID, reactID string, task *AiTask, parentIndex string, forceExecuting bool) TaskRuntimeEntry {
	entry := TaskRuntimeEntry{
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

func enrichAiTaskEntry(entry TaskRuntimeEntry, task *AiTask, parentIndex string) TaskRuntimeEntry {
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
	entry.RecentTextOutputs = recentTextOutputsFromTimeline(timeline, defaultRecentTextOutputLimit)
	return entry
}

func recentTextOutputsFromTimeline(timeline *aicommon.Timeline, limit int) []TaskTextOutput {
	if timeline == nil || limit <= 0 {
		return nil
	}
	outputs := timeline.ToTimelineItemOutputLastN(limit)
	if len(outputs) == 0 {
		return nil
	}
	result := make([]TaskTextOutput, 0, len(outputs))
	for _, item := range outputs {
		if item == nil {
			continue
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		result = append(result, TaskTextOutput{
			Timestamp: item.Timestamp.Format("2006-01-02 15:04:05"),
			Type:      item.Type,
			Content:   utils.ShrinkString(content, 2000),
		})
	}
	return result
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
