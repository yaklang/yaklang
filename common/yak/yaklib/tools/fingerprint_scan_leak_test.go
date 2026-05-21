package tools

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
)

// fingerprint_scan_leak_test 验证 fingerprint scan goroutine 在 ctx cancel /
// 下游停止消费时不再泄漏. 历史问题: outC 是 unbuffered 且 send 没有 ctx 短路,
// 调用方 (如 yak VM) 提前退出 range outC 后, inner goroutine 永久阻塞在
// `outC <- result`, 拖死 swg.Wait/close(outC), 进一步拖死整个 ReAct 主循环.
//
// 关键词: fingerprint goroutine 泄漏回归测试, outC ctx 短路验证,
//	sendMatchResultOrDrop, scan_port cancel 行为

// waitGoroutineConverge 反复读 runtime.NumGoroutine, 直到取到与 baseline 的
// 差值落入容忍区间 (tolerance) 或超时. 用于断言 goroutine 收敛, 而不是依赖
// 一次性快照 (race window 可能漏掉刚回收的 goroutine).
//
// 关键词: NumGoroutine 收敛轮询, goroutine 泄漏断言基线对比
func waitGoroutineConverge(t *testing.T, baseline int, tolerance int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	last := runtime.NumGoroutine()
	for time.Now().Before(deadline) {
		runtime.GC()
		runtime.Gosched()
		last = runtime.NumGoroutine()
		if last-baseline <= tolerance {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last
}

// TestMUSTPASS_Fingerprint_SendMatchResultOrDrop_CancelExits 验证
// sendMatchResultOrDrop 在 outC 无消费者 + ctx cancel 时立即退出, 不会永久挂起.
//
// 关键词: sendMatchResultOrDrop ctx 短路, drop on cancel, 单元测试
func TestMUSTPASS_Fingerprint_SendMatchResultOrDrop_CancelExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outC := make(chan *fp.MatchResult)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sendMatchResultOrDrop(ctx, outC, &fp.MatchResult{})
	}()

	select {
	case <-done:
		t.Fatal("sendMatchResultOrDrop returned before cancel or consumption")
	case <-time.After(80 * time.Millisecond):
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sendMatchResultOrDrop did not return after ctx cancel")
	}
}

// TestMUSTPASS_Fingerprint_SendMatchResultOrDrop_BufferedSucceeds 验证正常路径:
// outC 有 buffer 时, send 直接成功, 不依赖 ctx.
//
// 关键词: sendMatchResultOrDrop normal path, buffered outC
func TestMUSTPASS_Fingerprint_SendMatchResultOrDrop_BufferedSucceeds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outC := make(chan *fp.MatchResult, 1)
	res := &fp.MatchResult{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sendMatchResultOrDrop(ctx, outC, res)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("sendMatchResultOrDrop did not return on buffered outC")
	}

	select {
	case got := <-outC:
		if got != res {
			t.Fatalf("unexpected result; want %p, got %p", res, got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected result in outC after send")
	}
}
