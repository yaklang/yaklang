package aid

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/workflowdag"
)

type runtime struct {
	RootTask *AiTask
	config   *Coordinator

	statusMutex       sync.Mutex
	execGraph         *executableTaskGraph
	graphDirty        bool
	currentStage      int
	stageAnchorTaskID string
	activeTaskIndexes []string
}

type runtimeProgressSnapshot struct {
	totalTasks       int
	totalStages      int
	completedStages  int
	currentStage     int
	currentIndex     int
	currentTaskIndex string
	activeTaskIDs    []string
}

type stageExecutionResult struct {
	task *AiTask
	err  error
}

func isExecutableTaskTerminal(task *AiTask) bool {
	if task == nil {
		return true
	}
	switch task.GetStatus() {
	case aicommon.AITaskState_Skipped, aicommon.AITaskState_Aborted:
		return true
	}
	return task.executed()
}

func (r *runtime) currentIndex() int {
	if idx := r.currentProgressIndex(); idx <= 0 {
		return 0
	} else {
		return idx - 1
	}
}

func (r *runtime) currentProgressIndex() int {
	return r.progressSnapshot().currentIndex
}

func (c *Coordinator) createRuntime() *runtime {
	return &runtime{
		config:       c,
		currentStage: -1,
	}
}

func (t *AiTask) dumpProgressEx(i int, w io.Writer, details bool) {
	prefix := strings.Repeat(" ", i)

	executing := false
	finished := false
	if len(t.Subtasks) > 0 {
		allFinished := true
		haveExecutedTask := false
		for _, subtask := range t.Subtasks {
			if !subtask.executed() {
				allFinished = false
			} else if !haveExecutedTask && subtask.executed() {
				haveExecutedTask = true
			}
		}
		if haveExecutedTask && !allFinished {
			executing = true
		} else if allFinished {
			finished = true
		}
	} else {
		finished = t.executed()
	}

	var fill = " "
	var note string
	var finishedSummary string
	if finished {
		fill = "x"
		if t.LongSummary != "" {
			finishedSummary = t.LongSummary
		} else if t.TaskSummary != "" {
			finishedSummary = t.TaskSummary
		} else if t.ShortSummary != "" {
			finishedSummary = t.ShortSummary
		}
	} else if executing {
		fill = "~"
		note = " (部分完成)"
	}

	if t.executing() {
		fill = "-"
		note = " (执行中)"
		if ret := t.SingleLineStatusSummary(); ret != "" {
			note += fmt.Sprintf(" (status:%s)", ret)
		}
	}

	taskNameShow := strconv.Quote(t.Name)
	if details {
		if t.Goal != "" {
			taskNameShow = taskNameShow + "(目标:" + strconv.Quote(t.Goal) + ")"
		}
		if t.Index != "" {
			taskNameShow = t.Index + ". " + taskNameShow
		}
	}
	if strings.TrimSpace(note) == "" && finishedSummary == "" {
		note = "(未开始)"
	}
	_, _ = fmt.Fprintf(w, "%s-[%v] %s%v\n", prefix, fill, taskNameShow, note)

	if finishedSummary != "" {
		summaryPrefix := strings.Repeat(" ", i+1)
		_, _ = fmt.Fprintf(w, "%sFinished: %s\n", summaryPrefix, finishedSummary)
	}

	if len(t.Subtasks) > 0 {
		for _, subtask := range t.Subtasks {
			subtask.dumpProgressEx(i+1, w, details)
		}
	}
}

func (t *AiTask) dumpProgress(i int, w io.Writer) {
	t.dumpProgressEx(i, w, true)
}

func (t *AiTask) Progress() string {
	if t == nil {
		return ""
	}
	var buf bytes.Buffer
	t.dumpProgress(0, &buf)
	return buf.String()
}

func (t *AiTask) ProgressWithDetail() string {
	if t == nil {
		return ""
	}
	var buf bytes.Buffer
	t.dumpProgressEx(0, &buf, true)
	return buf.String()
}

func (r *runtime) Progress() string {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	if r.RootTask == nil {
		return ""
	}
	var buf bytes.Buffer
	r.RootTask.dumpProgress(0, &buf)
	return buf.String()
}

func (r *runtime) markExecutionGraphDirty() {
	r.statusMutex.Lock()
	r.graphDirty = true
	r.statusMutex.Unlock()
}

func (r *runtime) updateTaskLink() {
	r.markExecutionGraphDirty()
}

func (r *runtime) ensureExecutionGraph() (*executableTaskGraph, error) {
	r.statusMutex.Lock()
	root := r.RootTask
	graph := r.execGraph
	dirty := r.graphDirty
	r.statusMutex.Unlock()

	if root == nil {
		return nil, workflowdag.ErrEmptyDAG
	}
	if graph != nil && !dirty {
		return graph, nil
	}

	built, err := buildStrictExecutableTaskGraph(root)
	if err != nil {
		return nil, err
	}

	r.statusMutex.Lock()
	r.execGraph = built
	r.graphDirty = false
	r.statusMutex.Unlock()
	return built, nil
}

func (r *runtime) progressSnapshot() runtimeProgressSnapshot {
	r.statusMutex.Lock()
	graph := r.execGraph
	root := r.RootTask
	currentStage := r.currentStage
	activeTaskIDs := append([]string(nil), r.activeTaskIndexes...)
	r.statusMutex.Unlock()

	if graph == nil && root != nil {
		if built, err := buildStrictExecutableTaskGraph(root); err == nil {
			graph = built
		}
	}

	snapshot := runtimeProgressSnapshot{
		currentStage:  currentStage,
		activeTaskIDs: activeTaskIDs,
	}
	if graph == nil {
		return snapshot
	}

	snapshot.totalTasks = graph.TotalTasks()
	snapshot.totalStages = graph.TotalStages()

	completedTasks := 0
	completedStages := 0
	for stageIdx, stage := range graph.stages {
		stageDone := len(stage) > 0
		for _, node := range stage {
			if node == nil || node.task == nil {
				continue
			}
			if isExecutableTaskTerminal(node.task) {
				completedTasks++
				continue
			}
			stageDone = false
		}
		if stageDone {
			completedStages = stageIdx + 1
			continue
		}
		break
	}
	snapshot.completedStages = completedStages
	snapshot.currentIndex = completedTasks + len(activeTaskIDs)
	if snapshot.currentIndex > snapshot.totalTasks {
		snapshot.currentIndex = snapshot.totalTasks
	}

	switch {
	case len(activeTaskIDs) > 0:
		snapshot.currentTaskIndex = activeTaskIDs[0]
	case completedStages < snapshot.totalStages && completedStages >= 0:
		stage := graph.stages[completedStages]
		if len(stage) > 0 && stage[0] != nil {
			snapshot.currentStage = completedStages
			snapshot.currentTaskIndex = stage[0].id
		}
	}
	if snapshot.currentStage < 0 {
		if len(activeTaskIDs) > 0 {
			if stage, ok := graph.StageOf(activeTaskIDs[0]); ok {
				snapshot.currentStage = stage
			}
		} else if snapshot.currentTaskIndex != "" {
			if stage, ok := graph.StageOf(snapshot.currentTaskIndex); ok {
				snapshot.currentStage = stage
			}
		}
	}

	return snapshot
}

func (r *runtime) setActiveStage(stageIdx int, nodes []*executableTaskNode) {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	r.currentStage = stageIdx
	r.activeTaskIndexes = r.activeTaskIndexes[:0]
	r.stageAnchorTaskID = ""
	for _, node := range nodes {
		if node == nil || node.task == nil {
			continue
		}
		if r.stageAnchorTaskID == "" {
			r.stageAnchorTaskID = node.id
		}
		r.activeTaskIndexes = append(r.activeTaskIndexes, node.id)
	}
}

func (r *runtime) finishActiveTask(taskIndex string) {
	r.statusMutex.Lock()
	defer r.statusMutex.Unlock()

	if taskIndex == "" || len(r.activeTaskIndexes) == 0 {
		return
	}
	filtered := r.activeTaskIndexes[:0]
	for _, active := range r.activeTaskIndexes {
		if active == taskIndex {
			continue
		}
		filtered = append(filtered, active)
	}
	r.activeTaskIndexes = filtered
}

func (r *runtime) clearActiveStage() {
	r.statusMutex.Lock()
	r.currentStage = -1
	r.stageAnchorTaskID = ""
	r.activeTaskIndexes = nil
	r.statusMutex.Unlock()
}

func (r *runtime) representativeTask() *AiTask {
	graph, err := r.ensureExecutionGraph()
	if err != nil {
		return nil
	}
	snapshot := r.progressSnapshot()
	if snapshot.currentTaskIndex == "" {
		return nil
	}
	node, ok := graph.Node(snapshot.currentTaskIndex)
	if !ok || node == nil {
		return nil
	}
	return node.task
}

func (r *runtime) currentInteractiveTask() (*AiTask, error) {
	graph, err := r.ensureExecutionGraph()
	if err != nil {
		return nil, err
	}
	snapshot := r.progressSnapshot()
	if len(snapshot.activeTaskIDs) == 0 {
		return nil, utils.Errorf("no active task")
	}
	if len(snapshot.activeTaskIDs) > 1 {
		log.Warnf("more than one active task found")
	}
	node, ok := graph.Node(snapshot.activeTaskIDs[0])
	if !ok || node == nil || node.task == nil {
		return nil, utils.Errorf("active task %q not found", snapshot.activeTaskIDs[0])
	}
	return node.task, nil
}

func (r *runtime) nextPendingStage(graph *executableTaskGraph) (int, []*executableTaskNode, bool) {
	if graph == nil {
		return 0, nil, false
	}
	for stageIdx, stage := range graph.stages {
		pending := make([]*executableTaskNode, 0, len(stage))
		for _, node := range stage {
			if node == nil || node.task == nil || isExecutableTaskTerminal(node.task) {
				continue
			}
			pending = append(pending, node)
		}
		if len(pending) > 0 {
			sort.Slice(pending, func(i, j int) bool {
				return pending[i].order < pending[j].order
			})
			return stageIdx, pending, true
		}
	}
	return 0, nil, false
}

func (r *runtime) invokeTask(current *AiTask) error {
	if current == nil {
		return nil
	}

	if current.GetStatus() == aicommon.AITaskState_Skipped {
		r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
		r.config.EmitInfo("subtask %s was skipped by user, moving to next task", current.Name)
		return nil
	}
	if current.executed() {
		r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已完成 / Task [%s] Completed", current.Index, current.Index))
		r.config.EmitInfo("subtask %s already completed, moving to next task", current.Name)
		return nil
	}
	if r.config.IsCtxDone() {
		r.config.planLoadingStatus("执行已取消 / Execution Cancelled")
		return utils.Errorf("coordinator context is done")
	}
	if current.IsCtxDone() {
		if current.GetStatus() == aicommon.AITaskState_Skipped {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
			r.config.EmitInfo("subtask %s context cancelled (skipped), moving to next task", current.Name)
			return nil
		}
		return utils.Errorf("task context is done")
	}

	r.config.planLoadingStatus(fmt.Sprintf("准备执行任务 [%s]: %s / Preparing Task [%s]: %s",
		current.Index, current.Name, current.Index, current.Name))

	r.config.EmitInfo("invoke subtask: %v", current.Name)
	current.ForceSetStatus(aicommon.AITaskState_Processing) // recovery 时允许从终态强制回到执行中
	r.config.EmitPushTask(current)
	defer func() {
		r.config.EmitUpdateTaskStatus(current)
		r.config.EmitPopTask(current)
	}()

	return current.executeTaskPushTaskIndex()
}

func (r *runtime) executeStageWithHandler(stageIdx int, stageNodes []*executableTaskNode, totalTasks, totalStages int, handler func(*AiTask) error) (*AiTask, error) {
	if len(stageNodes) == 0 {
		return nil, nil
	}
	sort.Slice(stageNodes, func(i, j int) bool {
		return stageNodes[i].order < stageNodes[j].order
	})

	r.setActiveStage(stageIdx, stageNodes)
	representative := r.representativeTask()
	r.config.planLoadingStatus(fmt.Sprintf(
		"执行阶段 %d/%d（%d 个任务） / Executing Stage %d/%d (%d Tasks)",
		stageIdx+1, totalStages, len(stageNodes), stageIdx+1, totalStages, len(stageNodes),
	))
	r.config.savePlanAndExecState(Phase_NotCompleted, representative)

	concurrency := r.config.GetPlanExecTaskConcurrency()
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(stageNodes) {
		concurrency = len(stageNodes)
	}
	if concurrency == 1 {
		var failedTask *AiTask
		var firstErr error
		for _, node := range stageNodes {
			result := stageExecutionResult{task: node.task, err: handler(node.task)}
			if result.task != nil {
				r.finishActiveTask(result.task.Index)
			}
			r.config.planLoadingStatus(fmt.Sprintf(
				"执行进度: %d/%d - 当前阶段 %d/%d / Progress: %d/%d - Stage %d/%d",
				r.currentProgressIndex(), totalTasks, stageIdx+1, totalStages,
				r.currentProgressIndex(), totalTasks, stageIdx+1, totalStages,
			))
			r.config.savePlanAndExecState(Phase_NotCompleted, r.representativeTask())
			if result.err != nil && firstErr == nil {
				failedTask = result.task
				firstErr = result.err
			}
		}
		r.clearActiveStage()
		r.config.savePlanAndExecState(Phase_NotCompleted, nil)
		return failedTask, firstErr
	}

	jobs := make(chan *executableTaskNode, len(stageNodes))
	results := make(chan stageExecutionResult, len(stageNodes))
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for node := range jobs {
				if node == nil {
					continue
				}
				results <- stageExecutionResult{
					task: node.task,
					err:  handler(node.task),
				}
			}
		}()
	}
	for _, node := range stageNodes {
		jobs <- node
	}
	close(jobs)

	var failedTask *AiTask
	var firstErr error
	for range stageNodes {
		result := <-results
		if result.task != nil {
			r.finishActiveTask(result.task.Index)
		}
		r.config.planLoadingStatus(fmt.Sprintf(
			"执行进度: %d/%d - 当前阶段 %d/%d / Progress: %d/%d - Stage %d/%d",
			r.currentProgressIndex(), totalTasks, stageIdx+1, totalStages,
			r.currentProgressIndex(), totalTasks, stageIdx+1, totalStages,
		))
		r.config.savePlanAndExecState(Phase_NotCompleted, r.representativeTask())
		if result.err != nil && firstErr == nil {
			failedTask = result.task
			firstErr = result.err
		}
	}
	workers.Wait()
	r.clearActiveStage()
	r.config.savePlanAndExecState(Phase_NotCompleted, nil)
	return failedTask, firstErr
}

func (r *runtime) executeStage(stageIdx int, stageNodes []*executableTaskNode, totalTasks, totalStages int) (*AiTask, error) {
	return r.executeStageWithHandler(stageIdx, stageNodes, totalTasks, totalStages, r.invokeTask)
}

func (r *runtime) Invoke(task *AiTask, startTaskIndex string) (retErr error) {
	if r.RootTask == nil {
		r.RootTask = task
	}
	r.markExecutionGraphDirty()

	var currentTask *AiTask
	phase := Phase_NotCompleted
	defer func() {
		r.config.savePlanAndExecState(phase, currentTask)
	}()

	validatedStartTask := strings.TrimSpace(startTaskIndex)
	for {
		if r.RootTask != nil {
			r.config.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
				"root_task": r.RootTask,
			})
		}

		graph, err := r.ensureExecutionGraph()
		if err != nil {
			return err
		}
		if validatedStartTask != "" {
			if _, ok := graph.StageOf(validatedStartTask); !ok {
				return utils.Errorf("runtime: start task %q not found in executable DAG", validatedStartTask)
			}
			validatedStartTask = ""
		}

		stageIdx, stageNodes, ok := r.nextPendingStage(graph)
		if !ok {
			r.config.planLoadingStatus("所有任务执行完成 / All Tasks Completed")
			phase = Phase_Completed
			currentTask = nil
			return nil
		}

		failedTask, err := r.executeStage(stageIdx, stageNodes, graph.TotalTasks(), graph.TotalStages())
		currentTask = failedTask
		if err != nil {
			if failedTask != nil {
				isSkipped := failedTask.GetStatus() == aicommon.AITaskState_Skipped
				isContextCanceled := strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context done")
				if isSkipped || (isContextCanceled && failedTask.GetStatus() == aicommon.AITaskState_Skipped) {
					r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 用户跳过,继续下一个 / Task [%s] User Skipped, Continuing", failedTask.Index, failedTask.Index))
					r.config.EmitInfo("task %s was skipped by user, continuing to next task", failedTask.Name)
					continue
				}
			}
			if r.config.IsCtxDone() {
				r.config.planLoadingStatus("用户终止执行 / User Terminated Execution")
				r.config.EmitInfo("coordinator context cancelled, stopping execution")
				return err
			}
			if failedTask != nil {
				r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 执行失败 / Task [%s] Failed", failedTask.Index, failedTask.Index))
				r.config.EmitPlanExecFail("invoke task[%s] failed: %v", failedTask.Name, err)
			} else {
				r.config.planLoadingStatus("阶段执行失败 / Stage Execution Failed")
				r.config.EmitPlanExecFail("invoke stage failed: %v", err)
			}
			r.config.EmitError("invoke subtask failed: %v", err)
			log.Errorf("invoke subtask failed: %v", err)
			return err
		}
	}
}
