package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// TestStallHeartbeat_FiresAfterThreshold 验证: 主循环 recordIterationTick
// 推进后, 心跳协程在超过 threshold 没有新 tick 时, 会写一条
// [LOOP_STALL_DETECTED] timeline.
//
// 关键词: stall heartbeat 触发, [LOOP_STALL_DETECTED]
func TestStallHeartbeat_FiresAfterThreshold(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stall-fire", "stall heartbeat fire", ctx, nil, true)
	loop.SetCurrentTask(task)

	// 手动 tick 一次, 模拟主循环刚开始;
	// 之后不再 tick, 心跳应在 threshold 内触发.
	loop.recordIterationTick()

	// hardAbort=0 表示禁用硬抢断, 保留原 "纯观察者" 语义.
	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 20*time.Millisecond, 60*time.Millisecond, 0)
	defer stop()

	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond, "expected [LOOP_STALL_DETECTED] timeline entry")
}

// TestStallHeartbeat_DoesNotFireOnHealthyProgress 验证: 主循环持续推进时,
// 心跳协程不会误报卡死.
// 关键词: stall heartbeat 健康路径不误报
func TestStallHeartbeat_DoesNotFireOnHealthyProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stall-healthy", "healthy progress", ctx, nil, true)
	loop.SetCurrentTask(task)

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 80*time.Millisecond, 0)
	defer stop()

	// 在 200ms 期间每 15ms 推进一次, 远低于 80ms 阈值.
	stopTick := time.After(200 * time.Millisecond)
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stopTick:
			for _, e := range invoker.Entries() {
				if e.Tag == "[LOOP_STALL_DETECTED]" {
					t.Fatalf("stall heartbeat fired during healthy progression")
				}
			}
			return
		case <-ticker.C:
			loop.recordIterationTick()
		}
	}
}

// TestStallHeartbeat_StopReleasesGoroutine 验证: stop 返回后心跳协程立刻
// 退出, 不再访问 timeline. 这条主要为了避免泄漏 / 测试结束后误写.
// 关键词: stall heartbeat stop 释放
func TestStallHeartbeat_StopReleasesGoroutine(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stall-stop", "stop releases", ctx, nil, true)
	loop.SetCurrentTask(task)
	loop.recordIterationTick()

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 5*time.Millisecond, 30*time.Millisecond, 0)

	// 先等到至少一次火警写入, 然后调用 stop, 之后等长一会儿确认 timeline 不再增长.
	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	stop()
	beforeStop := len(invoker.Entries())
	time.Sleep(100 * time.Millisecond)
	afterStop := len(invoker.Entries())
	require.Equal(t, beforeStop, afterStop, "no timeline entries should be added after stop()")
}

// TestStallHeartbeat_HardAbortsAfterLongStall 验证: 当主循环卡死超过
// hardAbortThreshold 时, heartbeat 协程主动调用 task.Cancel() 把主循环踢出来,
// 并写一条 [LOOP_STALL_HARD_ABORT] timeline.
//
// 历史背景: 当 fingerprint goroutine 因 outC 无 ctx 短路而永久阻塞时, ReAct
// 主循环会卡死, 但 verification watchdog 仍然在异步触发, 给人 "假活" 的错觉.
// 本测试是 hard abort 兜底机制的核心回归用例.
//
// 关键词: hard abort 兜底, [LOOP_STALL_HARD_ABORT], task.Cancel,
//   主循环硬卡死自救
func TestStallHeartbeat_HardAbortsAfterLongStall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stall-hardabort", "stall hard abort", ctx, nil, true)
	loop.SetCurrentTask(task)

	loop.recordIterationTick()

	// interval=10ms, stuck=30ms, hardAbort=80ms.
	// 80ms 内会先经过几次 [LOOP_STALL_DETECTED] (>= 30ms), 然后命中 hardAbort.
	stop := loop.startStallHeartbeatWithClock(
		ctx, task, realStallHeartbeatClock{},
		10*time.Millisecond, 30*time.Millisecond, 80*time.Millisecond,
	)
	defer stop()

	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_HARD_ABORT]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "expected [LOOP_STALL_HARD_ABORT] timeline entry")

	// hard abort 必须导致 task.GetContext() 进入 done 状态, 让主循环能退出.
	require.Eventually(t, func() bool {
		select {
		case <-task.GetContext().Done():
			return true
		default:
			return false
		}
	}, 1*time.Second, 10*time.Millisecond, "task ctx should be done after hard abort")
}

// TestStallHeartbeat_HardAbortDisabledWhenZero 验证 hardAbort=0 时关闭硬抢断:
// 即使 gap 超过 hardAbort 的等价时间, 也只会持续触发 [LOOP_STALL_DETECTED],
// 不会出现 [LOOP_STALL_HARD_ABORT], 也不会 cancel task.
//
// 关键词: hard abort feature flag, hardAbort 0 禁用语义
func TestStallHeartbeat_HardAbortDisabledWhenZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stall-hardabort-off", "stall hard abort off", ctx, nil, true)
	loop.SetCurrentTask(task)
	loop.recordIterationTick()

	stop := loop.startStallHeartbeatWithClock(
		ctx, task, realStallHeartbeatClock{},
		10*time.Millisecond, 30*time.Millisecond, 0,
	)
	defer stop()

	// 等到第一次 [LOOP_STALL_DETECTED] 出现, 确保 heartbeat 真的在跑.
	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond)

	// 再多等一段时间让 gap 远超过 "测试可能挑选的" hardAbort 模拟值;
	// 但因为 hardAbort=0, 不应出现 hard abort timeline, 也不应 cancel task.
	time.Sleep(300 * time.Millisecond)
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_HARD_ABORT]", e.Tag, "hard abort must not fire when hardAbort=0")
	}
	select {
	case <-task.GetContext().Done():
		t.Fatal("task ctx must not be cancelled when hardAbort is disabled")
	default:
	}
}

// Ensure NewMockInvoker compiles in this file even when unused locally.
var _ = mockcfg.NewMockInvoker
