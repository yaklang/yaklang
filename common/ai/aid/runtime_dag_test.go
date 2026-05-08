package aid

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func buildManualExecutableGraph(t *testing.T, nodes ...*executableTaskNode) *executableTaskGraph {
	t.Helper()

	stageByTaskIndex, stages, err := calculateStrictExecutableStages(nodes)
	require.NoError(t, err)

	nodeByID := make(map[string]*executableTaskNode, len(nodes))
	dependents := make(map[string][]string, len(nodes))
	order := make([]*AiTask, 0, len(nodes))
	orderIndexByTask := make(map[string]int, len(nodes))
	for i, node := range nodes {
		require.NotNil(t, node)
		nodeByID[node.id] = node
		node.stage = stageByTaskIndex[node.id]
		order = append(order, node.task)
		orderIndexByTask[node.id] = i
		for _, depID := range node.deps {
			dependents[depID] = append(dependents[depID], node.id)
		}
	}

	return &executableTaskGraph{
		nodes:            nodes,
		nodeByID:         nodeByID,
		dependents:       dependents,
		stages:           stages,
		stageByTaskIndex: stageByTaskIndex,
		order:            order,
		orderIndexByTask: orderIndexByTask,
	}
}

func TestRuntimeStageBarrierWaitsWholeStage(t *testing.T) {
	coordinator := newTestCoordinator(t)
	coordinator.Config = aicommon.NewConfig(context.Background(), aicommon.WithPlanExecTaskConcurrency(2))

	fast := newStateTask(coordinator, "fast")
	slow := newStateTask(coordinator, "slow")
	next := newStateTask(coordinator, "next")
	fast.Index = "1-1"
	slow.Index = "1-2"
	next.Index = "1-3"

	graph := buildManualExecutableGraph(t,
		&executableTaskNode{task: fast, id: fast.Index, deps: nil, order: 0},
		&executableTaskNode{task: slow, id: slow.Index, deps: nil, order: 1},
		&executableTaskNode{task: next, id: next.Index, deps: []string{fast.Index}, order: 2},
	)

	r := &runtime{config: coordinator, RootTask: fast, execGraph: graph, currentStage: -1}

	fastStarted := make(chan struct{})
	fastDone := make(chan struct{})
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	nextStarted := make(chan struct{})
	stageFinished := make(chan struct{})
	stageErr := make(chan error, 1)

	go func() {
		defer close(stageFinished)
		_, err := r.executeStageWithHandler(0, graph.stages[0], graph.TotalTasks(), graph.TotalStages(), func(task *AiTask) error {
			switch task.Index {
			case fast.Index:
				close(fastStarted)
				close(fastDone)
				return nil
			case slow.Index:
				close(slowStarted)
				<-releaseSlow
				return nil
			case next.Index:
				close(nextStarted)
				return nil
			default:
				return nil
			}
		})
		if err != nil {
			stageErr <- err
			return
		}

		_, err = r.executeStageWithHandler(1, graph.stages[1], graph.TotalTasks(), graph.TotalStages(), func(task *AiTask) error {
			if task.Index == next.Index {
				close(nextStarted)
			}
			return nil
		})
		stageErr <- err
	}()

	<-fastStarted
	<-slowStarted
	<-fastDone

	select {
	case <-nextStarted:
		t.Fatal("next stage started before the slow task in the previous stage finished")
	case <-time.After(150 * time.Millisecond):
	}

	close(releaseSlow)

	select {
	case <-nextStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("next stage did not start after the previous stage finished")
	}
	<-stageFinished
	require.NoError(t, <-stageErr)
}

func TestRuntimeStageConcurrencyRespectsConfigLimit(t *testing.T) {
	coordinator := newTestCoordinator(t)
	coordinator.Config = aicommon.NewConfig(context.Background(), aicommon.WithPlanExecTaskConcurrency(2))

	nodes := make([]*executableTaskNode, 0, 4)
	for i := 0; i < 4; i++ {
		task := newStateTask(coordinator, "task")
		task.Index = "1-" + string(rune('1'+i))
		nodes = append(nodes, &executableTaskNode{
			task:  task,
			id:    task.Index,
			deps:  nil,
			order: i,
		})
	}
	graph := buildManualExecutableGraph(t, nodes...)
	r := &runtime{config: coordinator, RootTask: nodes[0].task, execGraph: graph, currentStage: -1}

	var active int32
	var maxActive int32
	started := make(chan struct{}, len(nodes))
	release := make(chan struct{})

	done := make(chan struct{})
	stageErr := make(chan error, 1)
	go func() {
		defer close(done)
		_, err := r.executeStageWithHandler(0, graph.stages[0], graph.TotalTasks(), graph.TotalStages(), func(task *AiTask) error {
			current := atomic.AddInt32(&active, 1)
			for {
				seen := atomic.LoadInt32(&maxActive)
				if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			atomic.AddInt32(&active, -1)
			return nil
		})
		stageErr <- err
	}()

	<-started
	<-started
	select {
	case <-started:
		t.Fatal("third task started before concurrency slot was released")
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	<-done
	require.NoError(t, <-stageErr)
	require.Equal(t, int32(2), atomic.LoadInt32(&maxActive))
}

func TestRuntimeStageConcurrencyOneExecutesStableOrder(t *testing.T) {
	coordinator := newTestCoordinator(t)
	coordinator.Config = aicommon.NewConfig(context.Background(), aicommon.WithPlanExecTaskConcurrency(1))

	nodes := make([]*executableTaskNode, 0, 3)
	expected := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		task := newStateTask(coordinator, "task")
		task.Index = "1-" + string(rune('1'+i))
		expected = append(expected, task.Index)
		nodes = append(nodes, &executableTaskNode{
			task:  task,
			id:    task.Index,
			deps:  nil,
			order: i,
		})
	}
	graph := buildManualExecutableGraph(t, nodes...)
	r := &runtime{config: coordinator, RootTask: nodes[0].task, execGraph: graph, currentStage: -1}

	actual := make([]string, 0, len(nodes))
	_, err := r.executeStageWithHandler(0, graph.stages[0], graph.TotalTasks(), graph.TotalStages(), func(task *AiTask) error {
		actual = append(actual, task.Index)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestRuntimeExecuteStageReturnsFirstErrorAfterWholeStage(t *testing.T) {
	coordinator := newTestCoordinator(t)
	coordinator.Config = aicommon.NewConfig(context.Background(), aicommon.WithPlanExecTaskConcurrency(2))

	a := newStateTask(coordinator, "a")
	b := newStateTask(coordinator, "b")
	a.Index = "1-1"
	b.Index = "1-2"

	graph := buildManualExecutableGraph(t,
		&executableTaskNode{task: a, id: a.Index, order: 0},
		&executableTaskNode{task: b, id: b.Index, order: 1},
	)
	r := &runtime{config: coordinator, RootTask: a, execGraph: graph, currentStage: -1}

	var finished sync.Map
	_, err := r.executeStageWithHandler(0, graph.stages[0], graph.TotalTasks(), graph.TotalStages(), func(task *AiTask) error {
		finished.Store(task.Index, true)
		if task.Index == a.Index {
			return context.Canceled
		}
		return nil
	})
	require.Error(t, err)
	_, ok := finished.Load(a.Index)
	require.True(t, ok)
	_, ok = finished.Load(b.Index)
	require.True(t, ok, "stage executor should wait for all tasks in the stage to finish before returning")
}
