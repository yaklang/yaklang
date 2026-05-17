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

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 20*time.Millisecond, 60*time.Millisecond)
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

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 80*time.Millisecond)
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

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 5*time.Millisecond, 30*time.Millisecond)

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

// Ensure NewMockInvoker compiles in this file even when unused locally.
var _ = mockcfg.NewMockInvoker
