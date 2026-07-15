package reactloops

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// TestProgressRegistry_BasicRegisterUnregister 验证 Register/Unregister
// 的基本生命周期: 注册后 IsAnyActive() 为 true, 注销后为 false.
//
// 关键词: ProgressRegistry 基本生命周期
func TestProgressRegistry_BasicRegisterUnregister(t *testing.T) {
	reg := NewProgressRegistry()
	require.False(t, reg.IsAnyActive(), "empty registry should not be active")

	handle := NewSubAgentHandle("sub-task-1", "scan_a", nil, time.Now())
	reg.Register(handle)
	require.True(t, reg.IsAnyActive(), "registry should be active after register")

	got := reg.GetHandle("sub-task-1")
	require.NotNil(t, got, "GetHandle should return the registered handle")
	require.Equal(t, "scan_a", got.Identifier)
	require.False(t, got.IsFinished(), "handle should not be finished before unregister")

	reg.Unregister("sub-task-1", nil)
	require.False(t, reg.IsAnyActive(), "registry should not be active after unregister")
	require.Nil(t, reg.GetHandle("sub-task-1"), "GetHandle should return nil after unregister")
	require.True(t, got.IsFinished(), "handle should be finished after unregister")
}

// TestProgressRegistry_LastActivityAtFallback 验证 SubLoop 为 nil 时
// LastActivityAt() fallback 到 StartedAt.
//
// 关键词: LastActivityAt fallback, SubLoop nil
func TestProgressRegistry_LastActivityAtFallback(t *testing.T) {
	startedAt := time.Now().Add(-5 * time.Second)
	handle := NewSubAgentHandle("sub-task-2", "scan_b", nil, startedAt)

	activity := handle.LastActivityAt()
	require.False(t, activity.IsZero(), "LastActivityAt should not be zero")
	require.True(t, activity.Equal(startedAt), "LastActivityAt should fallback to StartedAt when SubLoop is nil")
}

// TestProgressRegistry_LastActivityAtWithSubLoop 验证 SubLoop 不为 nil 时
// LastActivityAt() 读取 SubLoop 的 lastIterationTickAt.
//
// 关键词: LastActivityAt SubLoop, lastIterationTickAt 原子读
func TestProgressRegistry_LastActivityAtWithSubLoop(t *testing.T) {
	startedAt := time.Now().Add(-10 * time.Second)
	handle := NewSubAgentHandle("sub-task-3", "scan_c", nil, startedAt)

	// Create a minimal ReActLoop and tick it
	loop := &ReActLoop{}
	tickTime := time.Now().Add(-2 * time.Second)
	loop.SetLastIterationTickAtForTest(tickTime.UnixNano())
	handle.SubLoop = loop

	activity := handle.LastActivityAt()
	require.False(t, activity.IsZero(), "LastActivityAt should not be zero")
	require.True(t, activity.Equal(tickTime), "LastActivityAt should read from SubLoop.lastIterationTickAt")
}

// TestProgressRegistry_AggregateLastActivityAt 验证 AggregateLastActivityAt
// 返回所有活跃子 Agent 中最近的活动时间.
//
// 关键词: AggregateLastActivityAt, 多子 Agent 聚合
func TestProgressRegistry_AggregateLastActivityAt(t *testing.T) {
	reg := NewProgressRegistry()

	// No active handles -> zero
	require.True(t, reg.AggregateLastActivityAt().IsZero(), "empty registry should return zero")

	// Register two handles with different StartedAt
	older := NewSubAgentHandle("sub-a", "ident_a", nil, time.Now().Add(-30*time.Second))
	newer := NewSubAgentHandle("sub-b", "ident_b", nil, time.Now().Add(-5*time.Second))
	reg.Register(older)
	reg.Register(newer)

	latest := reg.AggregateLastActivityAt()
	require.False(t, latest.IsZero(), "aggregate should not be zero with active handles")

	// The newer handle's StartedAt should be the latest
	// (both have nil SubLoop, so fallback to StartedAt)
	require.True(t, latest.Equal(newer.StartedAt), "aggregate should return the most recent activity")
}

// TestProgressRegistry_DoneChannel 验证 done channel 在 Unregister 后被 close.
//
// 关键词: Done channel, Unregister close
func TestProgressRegistry_DoneChannel(t *testing.T) {
	reg := NewProgressRegistry()
	handle := NewSubAgentHandle("sub-task-done", "scan_d", nil, time.Now())
	reg.Register(handle)

	done := handle.Done()
	select {
	case <-done:
		t.Fatal("done channel should not be closed before unregister")
	default:
	}

	reg.Unregister("sub-task-done", nil)

	select {
	case <-done:
		// expected: channel is closed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("done channel should be closed after unregister")
	}
}

// TestProgressRegistry_WaitForAll 验证 WaitForAll 在所有子 Agent
// 注销后返回 nil.
//
// 关键词: WaitForAll, 阻塞等待
func TestProgressRegistry_WaitForAll(t *testing.T) {
	reg := NewProgressRegistry()
	h1 := NewSubAgentHandle("sub-wait-1", "wait_a", nil, time.Now())
	h2 := NewSubAgentHandle("sub-wait-2", "wait_b", nil, time.Now())
	reg.Register(h1)
	reg.Register(h2)

	// Unregister in a goroutine after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		reg.Unregister("sub-wait-1", nil)
		reg.Unregister("sub-wait-2", nil)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := reg.WaitForAll(ctx)
	require.NoError(t, err, "WaitForAll should return nil after all handles are unregistered")
}

// TestProgressRegistry_WaitForIdentifiers 验证 WaitForIdentifiers
// 只等待指定 identifiers 对应的子 Agent.
//
// 关键词: WaitForIdentifiers, 选择性等待
func TestProgressRegistry_WaitForIdentifiers(t *testing.T) {
	reg := NewProgressRegistry()
	h1 := NewSubAgentHandle("sub-id-1", "target_a", nil, time.Now())
	h2 := NewSubAgentHandle("sub-id-2", "target_b", nil, time.Now())
	h3 := NewSubAgentHandle("sub-id-3", "other_c", nil, time.Now())
	reg.Register(h1)
	reg.Register(h2)
	reg.Register(h3)

	// Only unregister target_a and target_b; leave other_c active
	go func() {
		time.Sleep(50 * time.Millisecond)
		reg.Unregister("sub-id-1", nil)
		reg.Unregister("sub-id-2", nil)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := reg.WaitForIdentifiers(ctx, []string{"target_a", "target_b"})
	require.NoError(t, err, "WaitForIdentifiers should return nil after targets finish")

	// other_c should still be active
	require.True(t, reg.IsAnyActive(), "other_c should still be active")

	// Cleanup
	reg.Unregister("sub-id-3", nil)
}

// TestProgressRegistry_NilSafe 验证所有方法在 nil receiver 时的安全行为.
//
// 关键词: nil safe, 防空指针
func TestProgressRegistry_NilSafe(t *testing.T) {
	var reg *ProgressRegistry
	require.False(t, reg.IsAnyActive())
	require.Nil(t, reg.GetHandle("x"))
	require.Empty(t, reg.AllHandles())
	require.True(t, reg.AggregateLastActivityAt().IsZero())

	// Register/Unregister on nil should not panic
	reg.Register(nil)
	reg.Unregister("x", nil)

	// WaitForAll on nil
	err := reg.WaitForAll(context.Background())
	require.NoError(t, err)
}

// TestProgressRegistry_UnregisterIdempotent 验证重复 Unregister 是安全的.
//
// 关键词: Unregister 幂等, 重复调用安全
func TestProgressRegistry_UnregisterIdempotent(t *testing.T) {
	reg := NewProgressRegistry()
	handle := NewSubAgentHandle("sub-idemp", "idemp", nil, time.Now())
	reg.Register(handle)

	reg.Unregister("sub-idemp", nil)
	reg.Unregister("sub-idemp", nil) // should not panic

	require.True(t, handle.IsFinished())
}

// TestProgressRegistry_AllHandlesSnapshot 验证 AllHandles 返回的是快照副本,
// 修改返回的 slice 不影响内部状态.
//
// 关键词: AllHandles 快照, 修改不影响内部
func TestProgressRegistry_AllHandlesSnapshot(t *testing.T) {
	reg := NewProgressRegistry()
	reg.Register(NewSubAgentHandle("sub-snap-1", "snap_a", nil, time.Now()))
	reg.Register(NewSubAgentHandle("sub-snap-2", "snap_b", nil, time.Now()))

	snapshot := reg.AllHandles()
	require.Len(t, snapshot, 2)

	// Modify the snapshot
	snapshot[0] = nil
	snapshot = append(snapshot, nil)

	// Internal state should be unaffected
	require.Len(t, reg.AllHandles(), 2, "internal state should not be affected by modifying snapshot")
}

// Ensure unused import is consumed
var _ aicommon.AIStatefulTask
