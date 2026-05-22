package aid

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type planTaskExecutionResult struct {
	taskIndex string
	err       error
}

func buildTaskIndexMap(root *AiTask) map[string]*AiTask {
	result := make(map[string]*AiTask)
	if root == nil {
		return result
	}
	order := DFSOrderAiTask(root)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil || task.Index == "" {
			continue
		}
		result[task.Index] = task
	}
	return result
}

func buildTaskDFSPositionMap(root *AiTask) map[string]int {
	result := make(map[string]int)
	if root == nil {
		return result
	}
	order := DFSOrderAiTask(root)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil || task.Index == "" {
			continue
		}
		result[task.Index] = i
	}
	return result
}

func collectLeafTasks(root *AiTask) []*AiTask {
	if root == nil {
		return nil
	}
	order := DFSOrderAiTask(root)
	leaves := make([]*AiTask, 0)
	for i := 0; i < order.Len(); i++ {
		task, ok := order.Get(i)
		if !ok || task == nil {
			continue
		}
		if len(task.Subtasks) == 0 {
			leaves = append(leaves, task)
		}
	}
	return leaves
}

func isPlanDependencyReady(task *AiTask) bool {
	if task == nil {
		return true
	}
	switch task.GetStatus() {
	case aicommon.AITaskState_Skipped, aicommon.AITaskState_Aborted:
		return true
	}
	if len(task.Subtasks) > 0 {
		return task.executed()
	}
	return task.GetStatus() == aicommon.AITaskState_Completed
}

func isPlanDependencyFailed(task *AiTask) bool {
	if task == nil {
		return false
	}
	return task.GetStatus() == aicommon.AITaskState_Aborted
}

func isAncestorTask(ancestor, task *AiTask) bool {
	if ancestor == nil || task == nil {
		return false
	}
	for parent := task.ParentTask; parent != nil; parent = parent.ParentTask {
		if parent.Index == ancestor.Index {
			return true
		}
	}
	return false
}

func resolvePlanTaskDependencies(task *AiTask, references map[string]string, taskMap map[string]*AiTask) []string {
	if task == nil {
		return nil
	}
	deps := normalizeDependencyRefs(task.DependsOn)
	resolved := make([]string, 0, len(deps))
	seen := make(map[string]struct{}, len(deps))
	for _, depRef := range deps {
		depIndex, ok := references[depRef]
		if !ok || depIndex == "" {
			continue
		}
		depTask, exists := taskMap[depIndex]
		if !exists || depTask == nil {
			continue
		}
		// Default serial depends_on may point at container/ancestor nodes.
		// Those are satisfied implicitly by sibling ordering and should not block leaf startup.
		if isAncestorTask(depTask, task) {
			continue
		}
		if _, dup := seen[depIndex]; dup {
			continue
		}
		seen[depIndex] = struct{}{}
		resolved = append(resolved, depIndex)
	}
	return resolved
}

func buildPlanLeafDependents(resolvedDeps map[string][]string) map[string][]string {
	dependents := make(map[string][]string)
	for leafID, deps := range resolvedDeps {
		for _, depID := range deps {
			dependents[depID] = append(dependents[depID], leafID)
		}
	}
	return dependents
}

func (r *runtime) collectSchedulableLeaves(startTaskIndex string) []*AiTask {
	if r == nil || r.RootTask == nil {
		return nil
	}

	posMap := buildTaskDFSPositionMap(r.RootTask)
	startPos := -1
	if strings.TrimSpace(startTaskIndex) != "" {
		if pos, ok := posMap[startTaskIndex]; ok {
			startPos = pos
		}
	}

	leaves := collectLeafTasks(r.RootTask)
	result := make([]*AiTask, 0, len(leaves))
	for _, task := range leaves {
		if task == nil {
			continue
		}
		if task.executed() || task.skiped() {
			continue
		}
		if startPos >= 0 {
			if pos, ok := posMap[task.Index]; !ok || pos < startPos {
				continue
			}
		}
		result = append(result, task)
	}
	return result
}

func (r *runtime) executePlanTaskDAG(ctx context.Context, startTaskIndex string) error {
	if r == nil || r.RootTask == nil {
		return nil
	}

	concurrency := 2
	if r.config != nil {
		concurrency = r.config.GetPlanTaskConcurrency()
	}
	if concurrency <= 0 {
		concurrency = 1
	}

	taskMap := buildTaskIndexMap(r.RootTask)
	references := buildTaskReferenceMap(r.RootTask)
	posMap := buildTaskDFSPositionMap(r.RootTask)
	startPos := -1
	if strings.TrimSpace(startTaskIndex) != "" {
		if pos, ok := posMap[startTaskIndex]; ok {
			startPos = pos
		}
	}

	type schedulerState struct {
		remainingDeps map[string]int
		resolvedDeps  map[string][]string
		queued        map[string]bool
		terminal      map[string]bool
	}

	leafTasks := collectLeafTasks(r.RootTask)
	if len(leafTasks) == 0 {
		return nil
	}

	state := &schedulerState{
		remainingDeps: make(map[string]int, len(leafTasks)),
		resolvedDeps:  make(map[string][]string, len(leafTasks)),
		queued:        make(map[string]bool, len(leafTasks)),
		terminal:      make(map[string]bool, len(leafTasks)),
	}

	activeLeafIDs := make([]string, 0, len(leafTasks))
	for _, task := range leafTasks {
		if task == nil || task.Index == "" {
			continue
		}
		if task.executed() || task.skiped() {
			state.terminal[task.Index] = true
			continue
		}
		if startPos >= 0 {
			if pos, ok := posMap[task.Index]; ok && pos < startPos {
				state.terminal[task.Index] = true
				continue
			}
		}
		activeLeafIDs = append(activeLeafIDs, task.Index)
		resolved := resolvePlanTaskDependencies(task, references, taskMap)
		state.resolvedDeps[task.Index] = resolved
		remaining := 0
		for _, depIndex := range resolved {
			if !isPlanDependencyReady(taskMap[depIndex]) {
				remaining++
			}
		}
		state.remainingDeps[task.Index] = remaining
	}

	if len(activeLeafIDs) == 0 {
		return nil
	}

	dependents := buildPlanLeafDependents(state.resolvedDeps)
	ready := make(chan *AiTask, len(activeLeafIDs))
	results := make(chan planTaskExecutionResult, len(activeLeafIDs))

	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for task := range ready {
				if task == nil {
					continue
				}
				err := r.invokePlanLeafTask(ctx, task)
				results <- planTaskExecutionResult{taskIndex: task.Index, err: err}
			}
		}()
	}

	markQueued := func(task *AiTask) {
		if task == nil || task.Index == "" {
			return
		}
		if state.queued[task.Index] || state.terminal[task.Index] {
			return
		}
		state.queued[task.Index] = true
		r.trackPlanTaskStarted(task)
		ready <- task
	}

	markSkipped := func(startID string) {
		queue := []string{startID}
		for len(queue) > 0 {
			taskID := queue[0]
			queue = queue[1:]

			if state.terminal[taskID] {
				continue
			}
			task := taskMap[taskID]
			if task == nil {
				continue
			}
			state.terminal[taskID] = true
			if task.GetStatus() != aicommon.AITaskState_Skipped {
				task.SetStatus(aicommon.AITaskState_Skipped)
			}
			r.trackPlanTaskFinished()

			for _, dependentID := range dependents[taskID] {
				if state.terminal[dependentID] || state.queued[dependentID] {
					continue
				}
				state.remainingDeps[dependentID]--
				if state.remainingDeps[dependentID] > 0 {
					continue
				}
				queue = append(queue, dependentID)
			}
		}
	}

	seeded := 0
	for _, taskID := range activeLeafIDs {
		if state.remainingDeps[taskID] == 0 {
			markQueued(taskMap[taskID])
			seeded++
		}
	}
	if seeded == 0 {
		for _, taskID := range activeLeafIDs {
			if state.terminal[taskID] || state.queued[taskID] {
				continue
			}
			if state.remainingDeps[taskID] > 0 {
				continue
			}
			markQueued(taskMap[taskID])
		}
	}

	inFlight := 0
	for _, queued := range state.queued {
		if queued {
			inFlight++
		}
	}

	countFinished := func() int {
		n := 0
		for _, taskID := range activeLeafIDs {
			if state.terminal[taskID] {
				n++
			}
		}
		return n
	}

	completed := countFinished()
	totalActive := len(activeLeafIDs)
	var finalErr error

	for completed < totalActive {
		if inFlight == 0 {
			break
		}

		result := <-results
		inFlight--
		state.terminal[result.taskIndex] = true
		completed = countFinished()
		r.trackPlanTaskFinished()

		task := taskMap[result.taskIndex]
		if task == nil {
			continue
		}

		if result.err != nil && task.GetStatus() != aicommon.AITaskState_Skipped && finalErr == nil {
			finalErr = result.err
		}

		for _, dependentID := range activeLeafIDs {
			if state.terminal[dependentID] || state.queued[dependentID] {
				continue
			}

			remaining := 0
			for _, depIndex := range state.resolvedDeps[dependentID] {
				if !isPlanDependencyReady(taskMap[depIndex]) {
					remaining++
				}
			}
			state.remainingDeps[dependentID] = remaining
			if remaining > 0 {
				continue
			}

			dependencyFailed := false
			for _, depIndex := range state.resolvedDeps[dependentID] {
				if isPlanDependencyFailed(taskMap[depIndex]) {
					dependencyFailed = true
					break
				}
			}
			if dependencyFailed {
				markSkipped(dependentID)
				completed = countFinished()
				continue
			}

			markQueued(taskMap[dependentID])
			inFlight++
		}
	}

	close(ready)
	workers.Wait()
	close(results)

	return finalErr
}

func (r *runtime) trackPlanTaskStarted(task *AiTask) {
	if r == nil {
		return
	}
	r.inFlightCount.Add(1)
	if r.config != nil && task != nil {
		r.config.planLoadingStatus(fmt.Sprintf("执行进度: 启动任务 [%s]: %s / Starting Task [%s]: %s",
			task.Index, task.Name, task.Index, task.Name))
		r.config.savePlanAndExecState(Phase_NotCompleted, task)
	}
}

func (r *runtime) trackPlanTaskFinished() {
	if r == nil {
		return
	}
	r.inFlightCount.Add(-1)
	r.completedCount.Add(1)
}

var planLeafTaskExecutorHook func(r *runtime, ctx context.Context, task *AiTask) error

func (r *runtime) invokePlanLeafTask(ctx context.Context, current *AiTask) error {
	if planLeafTaskExecutorHook != nil {
		return planLeafTaskExecutorHook(r, ctx, current)
	}
	if current == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if current.GetStatus() == aicommon.AITaskState_Skipped {
		if r.config != nil {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
			r.config.EmitInfo("subtask %s was skipped by user, moving to next task", current.Name)
		}
		return nil
	}
	if current.executed() {
		if r.config != nil {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已完成 / Task [%s] Completed", current.Index, current.Index))
			r.config.EmitInfo("subtask %s already completed, moving to next task", current.Name)
		}
		return nil
	}
	if r.config != nil && r.config.IsCtxDone() {
		r.config.planLoadingStatus("执行已取消 / Execution Cancelled")
		return utils.Errorf("coordinator context is done")
	}
	if current.IsCtxDone() {
		if current.GetStatus() == aicommon.AITaskState_Skipped {
			if r.config != nil {
				r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 已跳过 / Task [%s] Skipped", current.Index, current.Index))
				r.config.EmitInfo("subtask %s context cancelled (skipped), moving to next task", current.Name)
			}
			return nil
		}
		return utils.Errorf("task context is done")
	}

	if r.config != nil {
		if r.RootTask != nil {
			r.config.EmitJSON(schema.EVENT_TYPE_PLAN, "system", map[string]any{
				"root_task": r.RootTask,
			})
		}
		r.config.planLoadingStatus(fmt.Sprintf("准备执行任务 [%s]: %s / Preparing Task [%s]: %s",
			current.Index, current.Name, current.Index, current.Name))
		r.config.EmitInfo("invoke subtask: %v", current.Name)
	}

	current.SetStatus(aicommon.AITaskState_Processing)
	if r.config != nil {
		r.config.EmitPushTask(current)
	}
	defer func() {
		if r.config != nil {
			r.config.EmitUpdateTaskStatus(current)
			r.config.EmitPopTask(current)
		}
	}()

	err := current.executeTaskPushTaskIndex()
	if err != nil {
		if r.config == nil {
			return err
		}
		isSkipped := current.GetStatus() == aicommon.AITaskState_Skipped
		isContextCanceled := strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context done")
		if isSkipped || (isContextCanceled && current.GetStatus() == aicommon.AITaskState_Skipped) {
			r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 用户跳过,继续下一个 / Task [%s] User Skipped, Continuing", current.Index, current.Index))
			r.config.EmitInfo("task %s was skipped by user, continuing to next task", current.Name)
			return nil
		}
		if r.config.IsCtxDone() {
			r.config.planLoadingStatus("用户终止执行 / User Terminated Execution")
			r.config.EmitInfo("coordinator context cancelled, stopping execution")
			return err
		}
		r.config.planLoadingStatus(fmt.Sprintf("任务 [%s] 执行失败 / Task [%s] Failed", current.Index, current.Index))
		r.config.EmitPlanExecFail("invoke task[%s] failed: %v", current.Name, err)
		r.config.EmitError("invoke subtask failed: %v", err)
		log.Errorf("invoke subtask failed: %v", err)
		return err
	}
	return nil
}
