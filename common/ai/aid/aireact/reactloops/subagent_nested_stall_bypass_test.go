package reactloops

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// TestStallHeartbeat_NestedJobRegistersProgressAndBypassesStall 验证:
// 当主循环因 fast_context action 下发的 RunNestedJobWithProgress 阻塞时,
// 子 loop 会被注册到 ProgressRegistry 中并保持活跃, stall heartbeat 旁路
// 生效, 不会误报 [LOOP_STALL_DETECTED].
//
// 这是本次重构的重大目标: fast_context.go:17 的 action 下发子循环脱离了
// 之前的心跳检查豁免机制, 改动后需要确保豁免仍能正常触发.
//
// 测试策略:
//  1. 注册一个测试用 loop factory, 其 ExecuteWithExistedTask 会持续 tick
//     子 loop 的 lastIterationTickAt(模拟子 Agent 在工作), 并阻塞直到测试
//     主动取消.
//  2. 主 loop 启动 stall heartbeat(threshold 短), 主循环 tick 设为很旧.
//  3. 调用 RunNestedJobWithProgress(模拟 fast_context 下发), 验证:
//     - registry 中有活跃 handle
//     - stall heartbeat 不触发
//     - 子 loop 结束后 handle 注销
func TestStallHeartbeat_NestedJobRegistersProgressAndBypassesStall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	parentTask := aicommon.NewStatefulTaskBase("parent-fast-context", "scan", ctx, nil, true)
	loop.SetCurrentTask(parentTask)

	// 注册一个测试 loop factory: 子 loop 执行时持续 tick, 并阻塞直到 subRelease
	const testLoopName = "test_nested_stall_bypass_loop"
	subRelease := make(chan struct{})
	var subLoopTickCount atomic.Int64
	_ = RegisterLoopFactory(testLoopName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			// 用 WithInitTask 注入 init handler 模拟子 loop 持续推进: tick + 阻塞
			tickLoop := NewMinimalReActLoop(r.GetConfig(), r)
			initOpt := WithInitTask(func(l *ReActLoop, task aicommon.AIStatefulTask, op *InitTaskOperator) {
				go func() {
					ticker := time.NewTicker(10 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-subRelease:
							return
						case <-ticker.C:
							tickLoop.SetLastIterationTickAtForTest(time.Now().UnixNano())
							subLoopTickCount.Add(1)
						case <-ctx.Done():
							return
						}
					}
				}()
				// 阻塞直到测试释放; 模拟 fast_context 子 loop 在运行
				select {
				case <-subRelease:
				case <-ctx.Done():
				}
				op.Done() // 让子 loop 正常退出
			})
			for _, opt := range opts {
				opt(tickLoop)
			}
			initOpt(tickLoop)
			return tickLoop, nil
		},
	)
	defer func() {
		// 清理: 取消 ctx 会让子 loop 退出
	}()

	// 主循环 tick 设为很旧(模拟主循环阻塞在 fast_context action handler)
	loop.recordIterationTick()
	loop.SetLastIterationTickAtForTest(time.Now().Add(-2 * time.Minute).UnixNano())

	// 启动 stall heartbeat: interval 20ms, threshold 60ms, 无 hard abort
	stopHeartbeat := loop.startStallHeartbeatWithClock(ctx, parentTask, realStallHeartbeatClock{},
		20*time.Millisecond, 60*time.Millisecond, 0)

	// 异步调用 RunNestedJobWithProgress(模拟 fast_context 下发)
	type nestedResult struct {
		res *SubAgentResult
		err error
	}
	doneCh := make(chan nestedResult, 1)
	go func() {
		res, err := RunNestedJobWithProgress(invoker, loop, parentTask, SubAgentJob{
			Order:        1,
			Identifier:  "fast-context-test",
			TaskName:    "fast-context-test",
			LoopName:    testLoopName,
			ForkTimeline: false,
		}, nil, // 无 configure
		)
		doneCh <- nestedResult{res, err}
	}()

	// 等待子 loop 注册到 registry
	require.Eventually(t, func() bool {
		registry := loop.GetSubAgentProgressRegistry()
		return registry != nil && registry.IsAnyActive()
	}, 2*time.Second, 10*time.Millisecond, "子 loop 应注册到 ProgressRegistry")

	// 等待子 loop tick 若干次, 确认它在推进
	require.Eventually(t, func() bool {
		return subLoopTickCount.Load() >= 3
	}, 2*time.Second, 10*time.Millisecond, "子 loop 应持续 tick")

	// 在子 Agent 活跃期间, stall heartbeat 不应触发
	time.Sleep(200 * time.Millisecond)
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"主循环阻塞在 nested 子 loop 时, 子 Agent 仍有进度, stall heartbeat 不应触发")
	}

	// 释放子 loop, 让它正常结束
	close(subRelease)

	// 等待 RunNestedJobWithProgress 返回
	select {
	case r := <-doneCh:
		require.NoError(t, r.err, "RunNestedJobWithProgress 应正常返回")
		require.NotNil(t, r.res, "结果不应为 nil")
	case <-time.After(5 * time.Second):
		t.Fatal("RunNestedJobWithProgress 超时未返回")
	}

	// 子 loop 结束后, handle 应注销, registry 不再有活跃 handle
	require.Eventually(t, func() bool {
		registry := loop.GetSubAgentProgressRegistry()
		return registry == nil || !registry.IsAnyActive()
	}, 2*time.Second, 10*time.Millisecond, "子 loop 结束后 registry 应无活跃 handle")

	// 子 loop 结束后, 主循环仍很旧, stall heartbeat 现在应恢复触发
	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond, "子 Agent 注销后, stall heartbeat 应恢复触发")

	stopHeartbeat()

	_ = fmt.Sprintf("subLoopTickCount=%d", subLoopTickCount.Load())
	_ = schema.AI_REACT_LOOP_NAME_DEFAULT // 确保 schema 包被引用
}
