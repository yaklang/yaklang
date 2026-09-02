package reactloops

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// slotArmedLoopInvoker 供测试注入 AIRuntimeInvokerGetter：只记录构建发生的时间点。
type slotArmedLoopInvoker struct {
	*mockcfg.MockInvoker
	cfg *aicommon.Config
}

func (c *slotArmedLoopInvoker) GetConfig() aicommon.AICallerConfigIf { return c.cfg }

// TestDispatchSubAgents_TimeoutArmsOnSlotAcquisition 验证 job 的 wall-clock
// timeout 从"上槽"而非"prepare"起算：
//
// 3 个 job、并发 1、每 job 预算 2s、每个子 loop 执行 1.2s。
// 旧实现（prepare 期 WithTimeout）下第 2/3 个 job 排队 1.2s/2.4s 后上槽，
// 预算已被烧掉，子 loop 内 sleep 即触发 context deadline exceeded。
// 新实现下 3 个 job 都应成功，且每个 job 上槽时剩余完整预算。
//
// 关键词: timeout 上槽起算, 排队不烧预算, sub-agent slot acquisition
func TestDispatchSubAgents_TimeoutArmsOnSlotAcquisition(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	parentCfg := aicommon.NewConfig(parentCtx, aicommon.WithDisableAutoSkills(true))
	parentInvoker := &slotArmedLoopInvoker{MockInvoker: mockcfg.NewMockInvoker(parentCtx), cfg: parentCfg}
	parentTask := aicommon.NewStatefulTaskBase("task-slot-timeout", "slot timeout", parentCtx, nil, true)

	// 记录每个 job 的上槽时刻（runtime 构建时刻）与 loop 结束状态。
	var mu sync.Mutex
	armedAt := map[string]time.Time{}
	finished := map[string]bool{}
	loopDone := make(chan string, 4)

	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(childCtx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		childCfg := aicommon.NewConfig(childCtx, opts...)
		child := &slotArmedLoopInvoker{MockInvoker: mockcfg.NewMockInvoker(childCtx), cfg: childCfg}
		return child, nil
	}

	testLoopName := "test_slot_timeout_loop"
	_ = RegisterLoopFactory(testLoopName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			jobID := "unknown"
			if task := r.GetCurrentTask(); task != nil {
				jobID = task.GetId()
			}
			mu.Lock()
			armedAt[jobID] = time.Now()
			mu.Unlock()
			// 持有槽位 1.2s：小于 2s 预算，但大于"排队期若被计入则必死"的量级。
			initOpt := WithInitTask(func(l *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
				select {
				case <-time.After(1200 * time.Millisecond):
					op.Done()
				case <-task.GetContext().Done():
					op.Failed(task.GetContext().Err())
				}
				mu.Lock()
				finished[jobID] = task.GetContext().Err() == nil
				mu.Unlock()
				loopDone <- jobID
			})
			loop := NewMinimalReActLoop(r.GetConfig(), r)
			for _, opt := range opts {
				opt(loop)
			}
			initOpt(loop)
			return loop, nil
		},
	)

	const jobBudget = 2 * time.Second
	jobs := []SubAgentJob{
		{Order: 1, Identifier: "job-1", LoopName: testLoopName, Timeout: jobBudget},
		{Order: 2, Identifier: "job-2", LoopName: testLoopName, Timeout: jobBudget},
		{Order: 3, Identifier: "job-3", LoopName: testLoopName, Timeout: jobBudget},
	}

	start := time.Now()
	results := DispatchSubAgents(parentInvoker, parentTask, jobs, SubAgentOptions{
		TimelineMode:       SubAgentTimelineClean,
		ExecuteConcurrency: 1,
	})
	total := time.Since(start)
	require.Len(t, results, 3, "all 3 jobs must produce results")

	// 每个 job 都必须真正上槽（armed 恰好 3 次），子 loop 正常退出（未被 deadline 杀死）。
	mu.Lock()
	require.Len(t, armedAt, 3, "every job must be armed exactly once")
	armedCount := len(armedAt)
	finishedCount := 0
	for _, ok := range finished {
		if ok {
			finishedCount++
		}
	}
	mu.Unlock()
	require.Equal(t, 3, armedCount, "concurrency=1 must arm all jobs sequentially")
	require.Equal(t, 3, finishedCount, "all loops must finish without context error (budget arms on slot acquisition)")

	// 每个子 loop 都应正常退出（未被 deadline 杀死）。
	for _, res := range results {
		require.NoError(t, res.ExecErr, "job %s must not fail: budget must arm on slot acquisition, not prepare", res.Identifier)
	}

	// 排队时间检查：concurrency=1 串行时，arm 时刻按 job 顺序间隔 ≥1.2s
	//（说明后位 job 确实排队等待，且上槽时拿到完整预算）。
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, armedAt, 3, "every job must be armed exactly once")
	firstArmed, lastArmed := armedAt["unknown"], armedAt["unknown"]
	for _, at := range armedAt {
		if firstArmed.IsZero() || at.Before(firstArmed) {
			firstArmed = at
		}
		if lastArmed.IsZero() || at.After(lastArmed) {
			lastArmed = at
		}
	}
	require.Greater(t, lastArmed.Sub(firstArmed), 2*time.Second,
		"last job must be armed ≥2s after first (two serial 1.2s loops)")
	// 3 × 1.2s ≈ 3.6s 总时长：如果预算在排队期起算，job-2 会在上槽时已耗掉 1.2s，
	// 1.2s sleep > 剩余 0.8s → 必死。能全部走完即为上槽起算的直接证据。
	require.Greater(t, total, 3*time.Second, "3 serial 1.2s loops must take at least 3s")
}

// TestDispatchSubAgents_ParentCancelFailsQueuedJobs 验证父任务取消时，
// 仍在排队等槽的 job 立即失败返回，而不是继续傻等或上槽后僵死。
//
// 关键词: parent cancel, queued sub-agent, AddWithContext
func TestDispatchSubAgents_ParentCancelFailsQueuedJobs(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	parentCfg := aicommon.NewConfig(parentCtx, aicommon.WithDisableAutoSkills(true))
	parentInvoker := &slotArmedLoopInvoker{MockInvoker: mockcfg.NewMockInvoker(parentCtx), cfg: parentCfg}
	parentTask := aicommon.NewStatefulTaskBase("task-queue-cancel", "queue cancel", parentCtx, nil, true)

	// 第一个 job 一直占用槽位；第 2、3 个 job 排队。
	var firstStarted atomic.Bool
	releaseFirst := make(chan struct{})
	origGetter := aicommon.AIRuntimeInvokerGetter
	defer func() { aicommon.AIRuntimeInvokerGetter = origGetter }()
	aicommon.AIRuntimeInvokerGetter = func(childCtx context.Context, opts ...aicommon.ConfigOption) (aicommon.AITaskInvokeRuntime, error) {
		childCfg := aicommon.NewConfig(childCtx, opts...)
		return &slotArmedLoopInvoker{MockInvoker: mockcfg.NewMockInvoker(childCtx), cfg: childCfg}, nil
	}

	testLoopName := "test_queue_cancel_loop"
	_ = RegisterLoopFactory(testLoopName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			initOpt := WithInitTask(func(l *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
				if firstStarted.CompareAndSwap(false, true) {
					// 第一个 job：阻塞直到测试释放或父 ctx 取消。
					select {
					case <-releaseFirst:
					case <-task.GetContext().Done():
					}
				}
				op.Done()
			})
			loop := NewMinimalReActLoop(r.GetConfig(), r)
			for _, opt := range opts {
				opt(loop)
			}
			initOpt(loop)
			return loop, nil
		},
	)

	resultsCh := make(chan []*SubAgentResult, 1)
	go func() {
		resultsCh <- DispatchSubAgents(parentInvoker, parentTask, []SubAgentJob{
			{Order: 1, Identifier: "blocker", LoopName: testLoopName},
			{Order: 2, Identifier: "queued-1", LoopName: testLoopName},
			{Order: 3, Identifier: "queued-2", LoopName: testLoopName},
		}, SubAgentOptions{
			TimelineMode:       SubAgentTimelineClean,
			ExecuteConcurrency: 1,
		})
	}()

	require.Eventually(t, firstStarted.Load, 3*time.Second, 10*time.Millisecond, "first job must occupy the slot")

	// 取消父任务：第一个 job 的 loop ctx 会退出；排队中的 job 必须立即失败返回。
	parentCancel()

	select {
	case results := <-resultsCh:
		require.Len(t, results, 3)
		for _, res := range results {
			// blocker 可能正常退出（其 select 收到 Done），排队的必须失败。
			if res.Identifier != "blocker" {
				require.Error(t, res.ExecErr, "queued job %s must fail fast after parent cancel", res.Identifier)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DispatchSubAgents did not return after parent cancel")
	}
	close(releaseFirst)
}
