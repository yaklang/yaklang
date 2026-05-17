package reactloops

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// 主循环 stall 心跳监控参数. 都是 var 而非 const, 方便测试用更短间隔覆盖.
//
// 关键词: stall heartbeat 间隔, 卡死阈值, 可测试性
var (
	// loopStallHeartbeatInterval 是心跳检查间隔. 每隔这么久, heartbeat
	// goroutine 比对一次 r.lastIterationTickAt 是否前进.
	loopStallHeartbeatInterval = 30 * time.Second

	// loopStallStuckThreshold 是 "无推进就视为卡死" 的阈值. 一旦距离上一次
	// iteration 推进超过这个时长, 写一条 [LOOP_STALL_DETECTED] timeline +
	// dump goroutine stack 到日志. 默认 90s 保守一些, 比 watchdog (2min)
	// 早一步给出信号.
	loopStallStuckThreshold = 90 * time.Second

	// loopStallStackBudget 控制 dump 的 goroutine stack 字节上限, 防止
	// 单条 timeline 撑爆前端面板; 一般来说 64KB 已经够看清主要 goroutine.
	loopStallStackBudget = 64 * 1024
)

// stallHeartbeatTimeProvider 是 startStallHeartbeat 的时间源, 默认使用
// time.Now / time.NewTicker, 测试可通过 newStallHeartbeatWithClock 注入
// 一个加速的时钟以便在毫秒级单测里看到 stall 事件.
//
// 关键词: stallHeartbeatTimeProvider, 测试加速时钟
type stallHeartbeatTimeProvider interface {
	Now() time.Time
	NewTicker(d time.Duration) *time.Ticker
}

type realStallHeartbeatClock struct{}

func (realStallHeartbeatClock) Now() time.Time                       { return time.Now() }
func (realStallHeartbeatClock) NewTicker(d time.Duration) *time.Ticker { return time.NewTicker(d) }

// recordIterationTick 在主循环每轮 iteration 开始时调用, 让 stall heartbeat
// 知道我们还在动. 写入是无锁的, 读侧也是, 故 atomic.Int64 足以胜任.
//
// 关键词: recordIterationTick, lastIterationTickAt 写入
func (r *ReActLoop) recordIterationTick() {
	if r == nil {
		return
	}
	r.lastIterationTickAt.Store(time.Now().UnixNano())
}

// startStallHeartbeat 启动主循环 stall 监控 goroutine. 返回一个 stop func,
// 调用方必须在 Execute 退出前调用 (defer stop()), 否则 goroutine 会一直
// 等待 ctx.Done 才退出.
//
// 实现要点:
//   - 监控仅写 log + timeline, 永远不主动 abort 任何 task. 它是观察者, 不是
//     抢断者; 真正的兜底是 P1 流空闲超时.
//   - lastIterationTickAt 初始值若为 0 (主循环还没真正 tick 过), 暂不告警,
//     避免启动瞬间误报.
//   - 复用 task 的 context, 任务取消立刻退出.
//
// 关键词: startStallHeartbeat, 主循环卡死兜底观察, [LOOP_STALL_DETECTED]
func (r *ReActLoop) startStallHeartbeat(ctx context.Context, task aicommon.AIStatefulTask) func() {
	return r.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, loopStallHeartbeatInterval, loopStallStuckThreshold)
}

// startStallHeartbeatWithClock 是可注入时钟的内部实现, 仅供本包测试调用.
func (r *ReActLoop) startStallHeartbeatWithClock(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	clock stallHeartbeatTimeProvider,
	interval, threshold time.Duration,
) func() {
	if r == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = loopStallHeartbeatInterval
	}
	if threshold <= 0 {
		threshold = loopStallStuckThreshold
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		ticker := clock.NewTicker(interval)
		defer ticker.Stop()

		var lastReported int64
		for {
			select {
			case <-stopCh:
				return
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				lastTick := r.lastIterationTickAt.Load()
				if lastTick == 0 {
					continue
				}
				gap := now.Sub(time.Unix(0, lastTick))
				if gap < threshold {
					continue
				}
				if lastTick == lastReported {
					// 同一次卡死, 只报一次, 避免 timeline 灌水
					continue
				}
				lastReported = lastTick
				r.reportLoopStall(task, gap)
			}
		}
	}()

	return func() {
		close(stopCh)
		<-doneCh
	}
}

// reportLoopStall 将一次"卡死"事件写到 log + timeline.
// 关键词: reportLoopStall, [LOOP_STALL_DETECTED]
func (r *ReActLoop) reportLoopStall(task aicommon.AIStatefulTask, gap time.Duration) {
	iteration := r.GetCurrentIterationIndex()
	taskID := "<unknown>"
	if task != nil {
		taskID = task.GetId()
	}
	log.Warnf("[LOOP_STALL_DETECTED] task=%s iteration=%d no_progress_for=%v", taskID, iteration, gap)

	stackBuf := make([]byte, loopStallStackBudget)
	n := runtime.Stack(stackBuf, true)
	if n > 0 {
		log.Warnf("[LOOP_STALL_DETECTED][goroutines]\n%s", string(stackBuf[:n]))
	}

	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline("[LOOP_STALL_DETECTED]", fmt.Sprintf(
			"main loop has not advanced for %v at iteration %d; goroutine stacks dumped to log",
			gap, iteration,
		))
	}
}
