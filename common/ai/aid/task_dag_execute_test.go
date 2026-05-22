package aid

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestExecutePlanTaskDAG_DiamondParallel(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)
	coordinator.Config.PlanTaskConcurrency = 3

	root := &AiTask{Name: "Root", Goal: "root", Coordinator: coordinator}
	init := &AiTask{Name: "Init", Goal: "init", ParentTask: root}
	branchA := &AiTask{Name: "BranchA", Goal: "a", ParentTask: root, DependsOn: []string{"1-1"}}
	branchB := &AiTask{Name: "BranchB", Goal: "b", ParentTask: root, DependsOn: []string{"1-1"}}
	merge := &AiTask{Name: "Merge", Goal: "merge", ParentTask: root, DependsOn: []string{"1-2", "1-3"}}
	root.Subtasks = []*AiTask{init, branchA, branchB, merge}
	coordinator.standardizeTaskTree(root)

	var (
		mu        sync.Mutex
		order     []string
		inFlight  atomic.Int32
		maxFlight atomic.Int32
	)

	origHook := planLeafTaskExecutorHook
	planLeafTaskExecutorHook = func(_ *runtime, _ context.Context, task *AiTask) error {
		current := inFlight.Add(1)
		for {
			old := maxFlight.Load()
			if current <= old || maxFlight.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)

		mu.Lock()
		order = append(order, task.Index)
		mu.Unlock()

		task.SetStatus(aicommon.AITaskState_Completed)
		return nil
	}
	t.Cleanup(func() { planLeafTaskExecutorHook = origHook })

	rt := coordinator.createRuntime()
	rt.RootTask = root
	err := rt.executePlanTaskDAG(context.Background(), "")
	require.NoError(t, err)

	require.Len(t, order, 4)
	assert.Equal(t, "1-1", order[0])
	assert.ElementsMatch(t, []string{"1-2", "1-3"}, order[1:3])
	assert.Equal(t, "1-4", order[3])
	assert.GreaterOrEqual(t, int(maxFlight.Load()), 2, "branch tasks should run concurrently")
}

func TestExecutePlanTaskDAG_SerialChain(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)
	coordinator.Config.PlanTaskConcurrency = 3

	root := &AiTask{Name: "Root", Goal: "root", Coordinator: coordinator}
	step1 := &AiTask{Name: "Step1", Goal: "s1", ParentTask: root}
	step2 := &AiTask{Name: "Step2", Goal: "s2", ParentTask: root}
	step3 := &AiTask{Name: "Step3", Goal: "s3", ParentTask: root}
	root.Subtasks = []*AiTask{step1, step2, step3}
	coordinator.standardizeTaskTree(root)

	var (
		mu    sync.Mutex
		order []string
	)

	origHook := planLeafTaskExecutorHook
	planLeafTaskExecutorHook = func(_ *runtime, _ context.Context, task *AiTask) error {
		mu.Lock()
		order = append(order, task.Index)
		mu.Unlock()
		task.SetStatus(aicommon.AITaskState_Completed)
		return nil
	}
	t.Cleanup(func() { planLeafTaskExecutorHook = origHook })

	rt := coordinator.createRuntime()
	rt.RootTask = root
	err := rt.executePlanTaskDAG(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, []string{"1-1", "1-2", "1-3"}, order)
}

func TestExecutePlanTaskDAG_DependOnParentContainer(t *testing.T) {
	coordinator := newTaskGraphTestCoordinator(t, t.TempDir(), nil)
	coordinator.Config.PlanTaskConcurrency = 2

	root := &AiTask{Name: "Root", Goal: "root", Coordinator: coordinator}
	collect := &AiTask{Name: "Collect", Goal: "collect", ParentTask: root}
	parse := &AiTask{Name: "Parse", Goal: "parse", ParentTask: collect}
	review := &AiTask{Name: "Review", Goal: "review", ParentTask: root, DependsOn: []string{"1-1"}}
	collect.Subtasks = []*AiTask{parse}
	root.Subtasks = []*AiTask{collect, review}
	coordinator.standardizeTaskTree(root)

	var (
		mu    sync.Mutex
		order []string
	)

	origHook := planLeafTaskExecutorHook
	planLeafTaskExecutorHook = func(_ *runtime, _ context.Context, task *AiTask) error {
		mu.Lock()
		order = append(order, task.Index)
		mu.Unlock()
		task.SetStatus(aicommon.AITaskState_Completed)
		return nil
	}
	t.Cleanup(func() { planLeafTaskExecutorHook = origHook })

	rt := coordinator.createRuntime()
	rt.RootTask = root
	err := rt.executePlanTaskDAG(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, []string{"1-1-1", "1-2"}, order)
}
