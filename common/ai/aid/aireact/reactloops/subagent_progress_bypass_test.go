package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// TestStallHeartbeat_SubAgentActiveDoesNotFire 验证: 当主循环不推进
// 但 ProgressRegistry 中有活跃子 Agent 仍有进度时, stall heartbeat
// 不会触发 [LOOP_STALL_DETECTED].
//
// 关键词: stall heartbeat sub-agent 旁路, 不误报卡死
func TestStallHeartbeat_SubAgentActiveDoesNotFire(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-sub-agent-active", "sub-agent active", ctx, nil, true)
	loop.SetCurrentTask(task)

	// Register a sub-agent with recent activity
	registry := NewProgressRegistry()
	loop.SetSubAgentProgressRegistry(registry)
	handle := NewSubAgentHandle("sub-1", "scan_a", nil, time.Now())
	registry.Register(handle)
	// Wire a sub-loop that ticks continuously so LastActivityAt stays fresh
	subLoop := &ReActLoop{}
	handle.SubLoop = subLoop

	// Simulate sub-agent iterating (ticks every 10ms, well within 60ms threshold)
	subStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-subStop:
				return
			case <-ticker.C:
				subLoop.SetLastIterationTickAtForTest(time.Now().UnixNano())
			}
		}
	}()
	defer close(subStop)

	// Tick once then stop ticking (simulate main loop blocked on dispatch)
	loop.recordIterationTick()
	// Make the main loop tick old enough to trigger stall (but sub-agent is fresh)
	loop.SetLastIterationTickAtForTest(time.Now().Add(-2 * time.Minute).UnixNano())

	// Start heartbeat with short thresholds
	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{},
		20*time.Millisecond, 60*time.Millisecond, 0)
	defer stop()

	// Wait long enough for a stall to fire if the bypass wasn't working
	time.Sleep(200 * time.Millisecond)

	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"stall heartbeat must not fire when sub-agents are active")
	}
}

// TestStallHeartbeat_SubAgentStaleFires 验证: 当子 Agent 也没有进度
// (LastActivityAt 距今超过 threshold) 时, stall heartbeat 正常触发.
//
// 关键词: stall heartbeat sub-agent 过期, 正常报卡死
func TestStallHeartbeat_SubAgentStaleFires(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-sub-agent-stale", "sub-agent stale", ctx, nil, true)
	loop.SetCurrentTask(task)

	// Register a sub-agent with STALE activity
	registry := NewProgressRegistry()
	loop.SetSubAgentProgressRegistry(registry)
	handle := NewSubAgentHandle("sub-2", "scan_b", nil, time.Now().Add(-5*time.Minute))
	registry.Register(handle)
	// Sub-loop tick is also stale
	subLoop := &ReActLoop{}
	subLoop.SetLastIterationTickAtForTest(time.Now().Add(-5 * time.Minute).UnixNano())
	handle.SubLoop = subLoop

	// Main loop tick is stale too
	loop.SetLastIterationTickAtForTest(time.Now().Add(-2 * time.Minute).UnixNano())

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{},
		20*time.Millisecond, 60*time.Millisecond, 0)
	defer stop()

	// Stall should fire because both main loop and sub-agents are stale
	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond, "expected [LOOP_STALL_DETECTED] when sub-agents are also stale")
}

// TestStallHeartbeat_NoRegistryFiresNormally 验证: 没有 registry 时
// stall heartbeat 行为不受影响 (回归测试).
//
// 关键词: stall heartbeat 无 registry 回归
func TestStallHeartbeat_NoRegistryFiresNormally(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-no-registry", "no registry", ctx, nil, true)
	loop.SetCurrentTask(task)
	// No registry set

	loop.recordIterationTick()
	loop.SetLastIterationTickAtForTest(time.Now().Add(-2 * time.Minute).UnixNano())

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{},
		20*time.Millisecond, 60*time.Millisecond, 0)
	defer stop()

	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond, "expected [LOOP_STALL_DETECTED] when no registry is set")
}

// TestStallHeartbeat_SubAgentUnregisteredFiresNormally 验证: 子 Agent
// 注销后 (IsAnyActive == false), stall heartbeat 恢复正常行为.
//
// 关键词: stall heartbeat 子 Agent 注销后恢复
func TestStallHeartbeat_SubAgentUnregisteredFiresNormally(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-sub-unregistered", "unregistered", ctx, nil, true)
	loop.SetCurrentTask(task)

	registry := NewProgressRegistry()
	loop.SetSubAgentProgressRegistry(registry)
	handle := NewSubAgentHandle("sub-3", "scan_c", nil, time.Now())
	registry.Register(handle)
	subLoop := &ReActLoop{}
	handle.SubLoop = subLoop

	// Simulate sub-agent iterating
	subStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-subStop:
				return
			case <-ticker.C:
				subLoop.SetLastIterationTickAtForTest(time.Now().UnixNano())
			}
		}
	}()
	defer close(subStop)

	loop.recordIterationTick()
	loop.SetLastIterationTickAtForTest(time.Now().Add(-2 * time.Minute).UnixNano())

	stop := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{},
		20*time.Millisecond, 60*time.Millisecond, 0)
	defer stop()

	// Initially sub-agent is active, no stall
	time.Sleep(100 * time.Millisecond)
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag, "must not fire while sub-agent active")
	}

	// Unregister the sub-agent - now stall should fire
	registry.Unregister("sub-3", nil)

	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 20*time.Millisecond, "expected [LOOP_STALL_DETECTED] after sub-agent unregistered")
}
