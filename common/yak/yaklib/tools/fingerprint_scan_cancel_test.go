//go:build !race

// 见 fingerprint_scan_leak_integration_test.go 顶部说明: fp 包内部 StableReader
// 存在一个先于本次改动的 data race, 故网络相关集成测试统一在非 race 模式运行.
//
// 关键词: scan port cancel 集成测试, race 模式隔离

package tools

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
)

// startTarpitListener 启动一个"会 accept 但永不回包也不关闭连接"的 TCP listener,
// 用来模拟 tarpit / 全端口响应的异常主机: fp.MatchWithContext 对它发起探测后会一直
// 读不到响应, 只能等到 probe timeout 才结束. 这样可以放大"派发循环不短路"导致的
// 卡顿: 一旦派发了一批任务, 没有 ctx 短路就得卡满整个 probe timeout.
//
// 关键词: 本地 tarpit listener, 永不回包, 测试不外连
func startTarpitListener(t *testing.T) (host string, port int, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	var conns []net.Conn
	var mu atomic.Bool // 仅用作 stop 标记
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, conn) // 持有连接, 永不回包也不关闭
			if mu.Load() {
				return
			}
		}
	}()

	return addr.IP.String(), addr.Port, func() {
		mu.Store(true)
		_ = ln.Close()
		for _, c := range conns {
			_ = c.Close()
		}
	}
}

// TestMUSTPASS_Fingerprint_ScanFromTargetStream_CancelStopsDispatch 验证派发循环
// 在 ctx 已取消时立即短路, 不会把上游剩余的大量目标继续"派发 + 打印 + 探测"一遍.
//
// 构造: 5000 个指向 tarpit 的目标 + 30s probe timeout + 启动前就 cancel 的 ctx.
// 期望: 派发循环第一轮就因 scanCtx.Err() != nil 而 break, inner goroutine 也在
// 真正探测前短路, outC 迅速关闭 (远小于 probe timeout). 如果派发循环忽略 ctx
// (历史 bug), 则至少要卡满一整个 probe timeout 才可能结束.
//
// 关键词: 派发循环 ctx 短路回归, cancel 后不再刷屏, scan port 资源泄漏
func TestMUSTPASS_Fingerprint_ScanFromTargetStream_CancelStopsDispatch(t *testing.T) {
	host, port, closeFn := startTarpitListener(t)
	defer closeFn()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 启动前就取消, 模拟"上层已取消"

	syns := make(chan *synscan.SynScanResult, 5000)
	for i := 0; i < 5000; i++ {
		syns <- &synscan.SynScanResult{Host: host, Port: port}
	}
	close(syns)

	start := time.Now()
	outC, err := _scanFromTargetStream(
		syns,
		fp.WithCtx(ctx),
		fp.WithPoolSize(50),
		fp.WithProbeTimeoutHumanRead(30.0), // 故意很长, 放大"不短路"的卡顿
	)
	if err != nil {
		t.Fatalf("_scanFromTargetStream returned error: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range outC {
		}
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		t.Logf("outC closed in %v after pre-canceled ctx", elapsed)
		if elapsed > 10*time.Second {
			t.Fatalf("outC closed too slowly (%v); dispatch loop likely ignores ctx", elapsed)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("outC did not close after pre-canceled ctx; dispatch loop ignores ctx")
	}
}
