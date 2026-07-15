package reactloops

import (
	"context"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// SubAgentHandle 持有单个子 Agent 的运行期引用, 供 stall heartbeat /
// verification watchdog 等监控机制读取 "子 Agent 是否有进展".
//
// 字段在读侧 (heartbeat goroutine / watchdog) 可能被并发访问, 写侧仅
// 在 dispatch action handler 线程中操作, 因此用一个 mutex 保护即可.
//
// 关键词: SubAgentHandle, 子 Agent 进度追踪, stall heartbeat 旁路
type SubAgentHandle struct {
	// SubTaskID 是子 Agent 的唯一标识 (aicommon.AIStatefulTask.GetId()).
	SubTaskID string

	// Identifier 是 dispatch 时指定的稳定标签 (如 "scan_host_a").
	Identifier string

	// SubTask 是子 Agent 的 stateful task; 可能为 nil (创建失败时).
	SubTask aicommon.AIStatefulTask

	// SubLoop 是子 Agent 的 ReActLoop; 子 Agent 的 lastIterationTickAt
	// 是判断 "子 Agent 是否还在动" 的主要信号源. 可能为 nil (loop 创建
	// 失败或尚未创建).
	SubLoop *ReActLoop

	// StartedAt 是子 Agent 开始运行的时刻.
	StartedAt time.Time

	// done 在子 Agent 运行结束后被 close, 用于 wait action 阻塞等待.
	done chan struct{}

	// mu 保护 finished / execErr 字段的并发读写.
	mu       sync.Mutex
	finished bool
	execErr  error
}

// NewSubAgentHandle 创建一个新的 SubAgentHandle, 内部初始化 done channel.
// SubLoop 字段在 sub-loop 创建后由调用方设置 (通过 handle.SubLoop = subLoop).
//
// 关键词: NewSubAgentHandle, 构造函数, done channel 初始化
func NewSubAgentHandle(subTaskID, identifier string, subTask aicommon.AIStatefulTask, startedAt time.Time) *SubAgentHandle {
	return &SubAgentHandle{
		SubTaskID:  subTaskID,
		Identifier: identifier,
		SubTask:    subTask,
		StartedAt:  startedAt,
		done:       make(chan struct{}),
	}
}

// LastActivityAt 返回该子 Agent 最近一次有进展的时间戳.
//
// 优先读 SubLoop.GetLastIterationTickAt() (原子读, 无锁), 它由子 Agent 的
// 主循环每轮 iteration 推进; fallback 到 StartedAt (子 Agent 刚启动,
// 子 loop 还没 tick 过).
//
// 关键词: LastActivityAt, 子 Agent 进度时间戳, lastIterationTickAt 原子读
func (h *SubAgentHandle) LastActivityAt() time.Time {
	if h == nil {
		return time.Time{}
	}
	if h.SubLoop != nil {
		if tick := h.SubLoop.GetLastIterationTickAt(); tick > 0 {
			return time.Unix(0, tick)
		}
	}
	return h.StartedAt
}

// IsFinished 返回该子 Agent 是否已经结束 (成功或失败).
func (h *SubAgentHandle) IsFinished() bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.finished
}

// Done 返回一个 channel, 在子 Agent 运行结束后被 close.
// 调用方可以阻塞 <-handle.Done() 等待子 Agent 完成.
func (h *SubAgentHandle) Done() <-chan struct{} {
	if h == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return h.done
}

// ExecErr 返回子 Agent 的执行错误 (如果有), 仅在 IsFinished() 为 true 后保证有意义.
func (h *SubAgentHandle) ExecErr() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.execErr
}

// markFinished 标记子 Agent 已结束, 关闭 done channel 并记录错误.
// 只能由 registry 的 Register/Unregister 生命周期管理方调用一次.
func (h *SubAgentHandle) markFinished(err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.finished {
		return
	}
	h.finished = true
	h.execErr = err
	close(h.done)
}

// ProgressRegistry 是一个线程安全的子 Agent 进度注册表,
// 由 dispatch_sub_react_agents action 在下发子 Agent 时注册,
// 在子 Agent 结束时注销.
//
// 主要消费者:
//   - startStallHeartbeat: 当主循环因等待子 Agent 而不推进时,
//     检查 registry 中是否有活跃子 Agent 仍有进度, 避免误报卡死.
//   - triggerVerificationWatchdog: 当有活跃子 Agent 时, 跳过自动验证,
//     避免在子 Agent 运行期间误判任务已完成.
//   - wait_sub_react_agents action: 阻塞等待指定子 Agent 的 done channel.
//
// 关键词: ProgressRegistry, 子 Agent 进度注册表, stall heartbeat 旁路
type ProgressRegistry struct {
	mu      sync.Mutex
	handles map[string]*SubAgentHandle
}

// NewProgressRegistry 创建一个空的注册表.
func NewProgressRegistry() *ProgressRegistry {
	return &ProgressRegistry{
		handles: make(map[string]*SubAgentHandle),
	}
}

// Register 注册一个子 Agent handle, 以 SubTaskID 为 key.
// 如果同 key 已存在, 覆盖旧 handle (旧 handle 不受影响, 但从 registry 视角不再可见).
// 返回新创建的 handle, 调用方应在子 Agent 运行结束后调用 Unregister(subTaskID).
func (r *ProgressRegistry) Register(handle *SubAgentHandle) *SubAgentHandle {
	if r == nil || handle == nil {
		return handle
	}
	if handle.done == nil {
		handle.done = make(chan struct{})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handles[handle.SubTaskID] = handle
	return handle
}

// Unregister 注销一个子 Agent 并标记其为 finished.
// 重复调用是安全的 (幂等).
func (r *ProgressRegistry) Unregister(subTaskID string, execErr error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	handle, ok := r.handles[subTaskID]
	if ok {
		delete(r.handles, subTaskID)
	}
	r.mu.Unlock()
	if ok {
		handle.markFinished(execErr)
	}
}

// GetHandle 按 subTaskID 获取一个活跃的 handle, 不存在返回 nil.
func (r *ProgressRegistry) GetHandle(subTaskID string) *SubAgentHandle {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.handles[subTaskID]
}

// AllHandles 返回当前所有活跃 handle 的快照副本 (调用方可安全遍历).
func (r *ProgressRegistry) AllHandles() []*SubAgentHandle {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]*SubAgentHandle, 0, len(r.handles))
	for _, h := range r.handles {
		result = append(result, h)
	}
	return result
}

// IsAnyActive 返回 registry 中是否还有任何活跃 (未注销) 的子 Agent.
func (r *ProgressRegistry) IsAnyActive() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.handles) > 0
}

// AggregateLastActivityAt 返回所有活跃子 Agent 中最近一次的活动时间.
// 如果没有活跃子 Agent, 返回零值 (time.Time{}).
//
// 用于 stall heartbeat 判断 "子 Agent 还在动吗":
// 如果最近活动时间距今 < threshold, 说明子 Agent 有进展, 主循环的 "卡死"
// 实际是正常的等待, 不应触发 stall 报警或 hard abort.
//
// 关键词: AggregateLastActivityAt, 子 Agent 聚合进度, stall heartbeat 旁路判断
func (r *ProgressRegistry) AggregateLastActivityAt() time.Time {
	if r == nil {
		return time.Time{}
	}
	handles := r.AllHandles()
	if len(handles) == 0 {
		return time.Time{}
	}
	var latest time.Time
	for _, h := range handles {
		if h == nil {
			continue
		}
		at := h.LastActivityAt()
		if at.IsZero() {
			continue
		}
		if at.After(latest) {
			latest = at
		}
	}
	return latest
}

// WaitForAll 阻塞等待所有当前活跃的子 Agent 完成, 或 ctx 被取消.
// 返回 nil 表示全部完成, ctx.Err() 表示超时/取消.
func (r *ProgressRegistry) WaitForAll(ctx context.Context) error {
	if r == nil {
		return nil
	}
	handles := r.AllHandles()
	for _, h := range handles {
		if h == nil || h.IsFinished() {
			continue
		}
		select {
		case <-h.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// WaitForIdentifiers 阻塞等待指定 identifiers 对应的子 Agent 完成, 或 ctx 被取消.
// identifier 匹配 SubAgentHandle.Identifier 字段. 如果某个 identifier 没有对应
// 的活跃 handle, 跳过它 (视为已完成或不存在).
func (r *ProgressRegistry) WaitForIdentifiers(ctx context.Context, identifiers []string) error {
	if r == nil {
		return nil
	}
	// 构建 identifier → handle 映射快照
	r.mu.Lock()
	idSet := make(map[string]bool, len(identifiers))
	for _, id := range identifiers {
		idSet[id] = true
	}
	var toWait []*SubAgentHandle
	for _, h := range r.handles {
		if h == nil {
			continue
		}
		if len(identifiers) == 0 || idSet[h.Identifier] {
			toWait = append(toWait, h)
		}
	}
	r.mu.Unlock()

	for _, h := range toWait {
		if h == nil || h.IsFinished() {
			continue
		}
		select {
		case <-h.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
