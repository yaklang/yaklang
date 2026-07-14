package reactloops

import (
	"bytes"
	"context"
	"io"
	"sync"
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
//
//	主循环硬卡死自救
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

// ---------------------------------------------------------------------------
// KeepAlive integration tests: verify that the keep-alive ticker prevents
// stall-heartbeat false positives while the parent loop is blocked waiting
// for sub-agents, and that without keep-alive the stall detector still fires.
// ---------------------------------------------------------------------------

// TestKeepAlive_PreventsStallDuringSubAgentWait simulates the real-world
// scenario where the parent loop is blocked inside RunForkJobsConcurrently
// waiting for slow sub-agents. The keep-alive ticker (started via
// subagent.RunKeepAlive) periodically calls loop.KeepAlive to refresh
// lastIterationTickAt, so the stall heartbeat goroutine never sees a gap
// exceeding the stuck threshold — no [LOOP_STALL_DETECTED] is emitted.
//
// 关键词: KeepAlive 保活, sub-agent 等待期间不误报 stall
func TestKeepAlive_PreventsStallDuringSubAgentWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-keepalive", "keep alive during sub agent wait", ctx, nil, true)
	loop.SetCurrentTask(task)

	// Simulate the parent loop having ticked once before entering the
	// sub-agent wait (just like recordIterationTick at the top of each
	// iteration in ExecuteWithExistedTask).
	loop.recordIterationTick()

	// Start a very aggressive stall heartbeat: interval=10ms, threshold=50ms.
	// Without keep-alive, a stall would be reported within ~50ms.
	stopHeartbeat := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 50*time.Millisecond, 0)
	defer stopHeartbeat()

	// Start the keep-alive ticker — this is exactly what
	// subagent.RunForkJobsConcurrently does internally while blocked on
	// workers.Wait(). The default keepAliveInterval (15s) is too slow for a
	// unit test, so we call loop.KeepAlive manually on a fast ticker to
	// simulate the same mechanism at test scale.
	stopKeepAlive := make(chan struct{})
	keepAliveDone := make(chan struct{})
	go func() {
		defer close(keepAliveDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		loop.KeepAlive() // fire immediately
		for {
			select {
			case <-stopKeepAlive:
				return
			case <-ticker.C:
				loop.KeepAlive()
			}
		}
	}()

	// Wait long enough that the stall detector would have fired multiple times
	// (300ms >> 50ms threshold) if the tick were not being refreshed.
	time.Sleep(300 * time.Millisecond)

	// Stop the keep-alive ticker.
	close(stopKeepAlive)
	<-keepAliveDone

	// Assert: no [LOOP_STALL_DETECTED] should have been emitted because
	// keepAlive kept lastIterationTickAt fresh.
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"stall heartbeat must not fire while keep-alive is refreshing the tick")
	}
}

// TestKeepAlive_StillFiresAfterKeepAliveStops verifies that once the
// keep-alive ticker stops (i.e. sub-agents have finished but the parent
// loop has not yet advanced to the next iteration), the stall detector
// resumes normal operation and fires [LOOP_STALL_DETECTED]. This ensures
// keep-alive suppresses false positives only during the actual wait, not
// permanently.
//
// 关键词: KeepAlive 停止后 stall 恢复正常检测
func TestKeepAlive_StillFiresAfterKeepAliveStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-keepalive-then-stall", "keep alive then stall", ctx, nil, true)
	loop.SetCurrentTask(task)

	loop.recordIterationTick()

	// Start stall heartbeat: interval=10ms, threshold=50ms.
	stopHeartbeat := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 50*time.Millisecond, 0)
	defer stopHeartbeat()

	// Phase 1: run keep-alive for 200ms — should NOT trigger stall.
	stopKeepAlive := make(chan struct{})
	keepAliveDone := make(chan struct{})
	go func() {
		defer close(keepAliveDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		loop.KeepAlive()
		for {
			select {
			case <-stopKeepAlive:
				return
			case <-ticker.C:
				loop.KeepAlive()
			}
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(stopKeepAlive)
	<-keepAliveDone

	// Confirm no stall during the keep-alive window.
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"stall must not fire during keep-alive window")
	}

	// Phase 2: stop keep-alive and wait — stall SHOULD now fire because
	// lastIterationTickAt is no longer being refreshed.
	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "stall should fire after keep-alive stops and tick goes stale")
}

// TestKeepAlive_NilIsSafe verifies that calling KeepAlive on a nil ReActLoop
// does not panic. This mirrors the nil-safety contract that RunKeepAlive
// relies on (a nil KeepAliveFunc is a no-op, and a nil loop's KeepAlive is
// also a no-op).
//
// 关键词: KeepAlive nil 安全
func TestKeepAlive_NilIsSafe(t *testing.T) {
	var loop *ReActLoop
	require.NotPanics(t, func() {
		loop.KeepAlive()
	})
}

// TestKeepAlive_RefreshesTick verifies that calling KeepAlive updates
// lastIterationTickAt so that a subsequent stall-heartbeat check sees a
// fresh tick. This is the atomic-level contract that the keep-alive ticker
// relies on: each call to KeepAlive must make lastIterationTickAt current.
//
// 关键词: KeepAlive 刷新 tick
func TestKeepAlive_RefreshesTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)

	// Record an old tick, then sleep briefly so "now" advances.
	loop.recordIterationTick()
	oldTick := loop.lastIterationTickAt.Load()
	time.Sleep(5 * time.Millisecond)

	// KeepAlive should write a newer timestamp.
	loop.KeepAlive()
	newTick := loop.lastIterationTickAt.Load()
	require.Greater(t, newTick, oldTick, "KeepAlive must refresh lastIterationTickAt to a newer value")
}

// ---------------------------------------------------------------------------
// AI streaming reason-chunk tick refresh tests: verify that when a slow AI
// model streams reason/thinking chunks over time, the SetOnReasonChunk callback
// (wired in callAITransaction) refreshes lastIterationTickAt on every chunk,
// preventing the stall heartbeat from misfiring during long AI calls.
//
// These tests replicate the key code path from exec.go's callAITransaction:
//   1. Build an AIResponse, feed a reason stream from an io.Pipe with delayed
//      writes (simulating a slow model producing thinking chunks at intervals).
//   2. Register SetOnReasonChunk with loop.recordIterationTick (exactly as
//      exec.go does).
//   3. Consume the stream via GetOutputStreamReader (which drains the reason
//      channel and fires invokeReasonChunk -> onReasonChunk -> recordIterationTick).
//   4. Assert no [LOOP_STALL_DETECTED] during the slow stream, and that stall
//      fires after the stream ends and the tick goes stale.
//
// 关键词: AI reason chunk 刷新 tick, 慢模型 stall 误报规避, io.Pipe 流式模拟
// ---------------------------------------------------------------------------

// simulateSlowReasonStream builds an AIResponse that emits reason chunks at
// intervals, then a final output chunk with the action JSON. It returns the
// response and a done channel that is closed when the feeder goroutine
// finishes writing all chunks.
//
// chunkInterval controls how long the feeder sleeps between reason chunks.
// This mirrors the real-world scenario where a thinking model (e.g.
// minimax-m3-thinking) produces reasoning tokens at intervals, with the
// total stream duration exceeding the stall threshold.
//
// Each reason chunk is emitted as a separate EmitReasonStream call so the
// GetOutputStreamReader goroutine processes them independently, firing
// invokeReasonChunk after each one.
func simulateSlowReasonStream(
	cfg aicommon.AICallerConfigIf,
	reasonChunks [][]byte,
	chunkInterval time.Duration,
	finalOutput []byte,
) (*aicommon.AIResponse, chan struct{}) {
	rsp := cfg.NewAIResponse()
	done := make(chan struct{})

	go func() {
		defer close(done)
		// Feed each reason chunk as a separate stream item so each one
		// triggers a separate invokeReasonChunk callback.
		for _, chunk := range reasonChunks {
			rsp.EmitReasonStream(bytes.NewReader(chunk))
			time.Sleep(chunkInterval)
		}
		// Feed the final output (action JSON).
		if len(finalOutput) > 0 {
			rsp.EmitOutputStream(bytes.NewReader(finalOutput))
		}
		rsp.Close()
	}()

	return rsp, done
}

// TestAIReasonChunk_RefreshesTickPreventsStall verifies that reason chunks
// arriving during a slow AI call keep lastIterationTickAt fresh, so the
// stall heartbeat does not fire even when the total AI call duration exceeds
// the stuck threshold. This is the direct test for the exec.go fix that adds
// r.recordIterationTick() inside SetOnReasonChunk.
//
// 关键词: reason chunk 刷新 tick, 慢 AI 调用不误报 stall
func TestAIReasonChunk_RefreshesTickPreventsStall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-reason-stream", "slow AI reason stream", ctx, nil, true)
	loop.SetCurrentTask(task)

	// Initial tick (simulating iteration start).
	loop.recordIterationTick()

	// Start stall heartbeat: interval=10ms, threshold=50ms.
	// Without reason-chunk tick refresh, stall would fire within ~50ms.
	stopHeartbeat := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 50*time.Millisecond, 0)
	defer stopHeartbeat()

	// Build a slow reason stream: 6 chunks, 15ms apart = ~90ms total duration.
	// This exceeds the 50ms threshold, but reason chunks refresh the tick
	// every 15ms, so stall should NOT fire.
	reasonChunks := [][]byte{
		[]byte("Analyzing the target codebase..."),
		[]byte("Looking for SQL injection patterns..."),
		[]byte("Found potential entry points..."),
		[]byte("Examining database query layer..."),
		[]byte("Tracing user input flow..."),
		[]byte("Concluding analysis..."),
	}
	finalOutput := []byte(`{"@action": "finish"}`)

	rsp, feedDone := simulateSlowReasonStream(
		invoker.GetConfig(), reasonChunks, 15*time.Millisecond, finalOutput,
	)

	// Wire SetOnReasonChunk exactly like exec.go does.
	rsp.SetOnReasonChunk(func(b []byte) {
		loop.recordIterationTick()
	})

	// Consume the output stream — this drains the reason channel and
	// fires invokeReasonChunk -> onReasonChunk -> recordIterationTick.
	stream := rsp.GetOutputStreamReader("test-node", false, invoker.GetConfig().GetEmitter())
	io.Copy(io.Discard, stream)

	// Wait for the feeder goroutine to finish.
	select {
	case <-feedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reason stream feeder did not complete in time")
	}

	// Assert: no [LOOP_STALL_DETECTED] should have been emitted because
	// reason chunks kept lastIterationTickAt fresh throughout the slow call.
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"stall must not fire while AI reason chunks are refreshing the tick")
	}
}

// TestAIReasonChunk_StallFiresWhenStreamGoesSilent verifies that if the AI
// stops producing reason chunks (simulating a truly stuck AI connection),
// the stall heartbeat correctly fires. This ensures the reason-chunk tick
// refresh does not mask genuine stalls.
//
// 关键词: reason chunk 停止后 stall 恢复正常
func TestAIReasonChunk_StallFiresWhenStreamGoesSilent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-reason-silent", "AI goes silent", ctx, nil, true)
	loop.SetCurrentTask(task)

	// Initial tick.
	loop.recordIterationTick()

	// Start stall heartbeat: interval=10ms, threshold=50ms.
	stopHeartbeat := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 50*time.Millisecond, 0)
	defer stopHeartbeat()

	// Phase 1: produce 3 reason chunks at 15ms intervals (~45ms total).
	// This keeps the tick fresh, no stall.
	reasonChunks := [][]byte{
		[]byte("Starting analysis..."),
		[]byte("Examining patterns..."),
		[]byte("Almost done..."),
	}

	rsp, feedDone := simulateSlowReasonStream(
		invoker.GetConfig(), reasonChunks, 15*time.Millisecond, nil,
	)

	rsp.SetOnReasonChunk(func(b []byte) {
		loop.recordIterationTick()
	})

	// Consume the output stream.
	stream := rsp.GetOutputStreamReader("test-node", false, invoker.GetConfig().GetEmitter())
	io.Copy(io.Discard, stream)

	select {
	case <-feedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("reason stream feeder did not complete in time")
	}

	// No stall should have fired during the active streaming phase.
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"stall must not fire while reason chunks are streaming")
	}

	// Phase 2: the stream has ended, no more reason chunks arrive.
	// The tick will go stale, and stall SHOULD fire.
	require.Eventually(t, func() bool {
		for _, e := range invoker.Entries() {
			if e.Tag == "[LOOP_STALL_DETECTED]" {
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond,
		"stall must fire after AI stream goes silent and tick goes stale")
}

// TestAIStreamFieldHandler_RefreshesTick verifies that the stream field
// handler entry point (added in exec.go) also refreshes the tick when stream
// fields start arriving, providing complementary coverage to SetOnReasonChunk
// for models that produce output chunks without reason/thinking.
//
// 关键词: stream field handler 刷新 tick, 输出流 stall 误报规避
func TestAIStreamFieldHandler_RefreshesTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	invoker := newTimelineCapturingInvoker(ctx)
	loop := NewMinimalReActLoop(invoker.GetConfig(), invoker)
	task := aicommon.NewStatefulTaskBase("task-stream-field", "AI output stream", ctx, nil, true)
	loop.SetCurrentTask(task)

	// Initial tick.
	loop.recordIterationTick()

	// Start stall heartbeat: interval=10ms, threshold=50ms.
	stopHeartbeat := loop.startStallHeartbeatWithClock(ctx, task, realStallHeartbeatClock{}, 10*time.Millisecond, 50*time.Millisecond, 0)
	defer stopHeartbeat()

	// Simulate a slow output stream: feed output chunks at 15ms intervals.
	// This exercises the stream field handler entry tick refresh.
	cfg := invoker.GetConfig()
	rsp := cfg.NewAIResponse()

	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		// Feed an output stream with delayed writes through an io.Pipe.
		pr, pw := io.Pipe()
		rsp.EmitOutputStream(pr)

		go func() {
			defer pw.Close()
			// Write output in chunks with delays, total ~90ms > 50ms threshold.
			chunks := [][]byte{
				[]byte(`{"@action": `),
				[]byte(`"finish",`),
				[]byte(`"answer": `),
				[]byte(`"done"}`),
			}
			for _, c := range chunks {
				pw.Write(c)
				time.Sleep(15 * time.Millisecond)
			}
		}()

		// Drain the pipe reader side to unblock the writer.
		io.Copy(io.Discard, pr)
		rsp.Close()
	}()

	// Simulate the stream field handler tick refresh: when the stream
	// reader starts producing data, call recordIterationTick (this is what
	// the stream field handler entry point does in exec.go).
	stream := rsp.GetOutputStreamReader("test-node", false, cfg.GetEmitter())

	// Read in a goroutine, refreshing tick on each successful read.
	var readWg sync.WaitGroup
	readWg.Add(1)
	go func() {
		defer readWg.Done()
		buf := make([]byte, 64)
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				// Simulate the stream field handler entry tick refresh.
				loop.recordIterationTick()
			}
			if err != nil {
				break
			}
		}
	}()

	readWg.Wait()

	select {
	case <-feedDone:
	case <-time.After(2 * time.Second):
		t.Fatal("output stream feeder did not complete in time")
	}

	// Assert: no stall during the slow output stream.
	for _, e := range invoker.Entries() {
		require.NotEqual(t, "[LOOP_STALL_DETECTED]", e.Tag,
			"stall must not fire while output stream chunks are refreshing the tick")
	}
}
