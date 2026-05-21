//go:build !race

// 网络集成测试: 不在 race 模式下运行.
//
// 原因: fp 包内部的 utils.StableReader 与 io.Copy goroutine 之间存在一个先存在
// 的 data race (common/utils/bytes_reader.go:353 vs :372, buffer.Bytes() 与
// io.Copy 写入 buffer 之间缺 mutex 保护). 这个 race 不在 PR-1 范围内, 应当
// 单独提 issue 修复. 本文件的两个集成测试在普通 (非 race) 模式下完整覆盖
// _scanFromTargetStream 与 _scanFingerprint 的 goroutine 收敛行为.
//
// 关键词: fingerprint 泄漏集成测试, race 模式隔离, StableReader 先存在 race

package tools

import (
	"context"
	"net"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
)

// startBlackHoleListener 启动一个 accept 后立即 close 连接的 TCP listener.
// 用于让 fp.MatchWithContext 走一个完整的 dial+close 路径, 但不返回有意义的
// banner, 这样 inner goroutine 能尽量快地结束 (要么 fp 内部跑完, 要么命中
// ctx cancel). 测试期间所有目标端口都指向同一个 listener.
//
// 关键词: 本地 TCP listener, 黑洞接受, 测试不外连
func startBlackHoleListener(t *testing.T) (host string, port int, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	var stopped atomic.Bool
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
			if stopped.Load() {
				return
			}
		}
	}()

	return addr.IP.String(), addr.Port, func() {
		stopped.Store(true)
		_ = ln.Close()
	}
}

// TestMUSTPASS_Fingerprint_ScanFromTargetStream_CtxCancel_NoLeak 集成测
// _scanFromTargetStream: 注入 N 条 SynScanResult, 完全不消费 outC, 然后 cancel
// ctx, 断言 inner goroutine 收敛回 baseline 附近.
//
// 该测试是 fingerprint goroutine 泄漏修复的核心回归用例. 修复前: outC 无 buffer
// + send 无 ctx 短路, inner goroutine 永久阻塞. 修复后: 通过 sendMatchResultOrDrop
// 走 ctx.Done() 分支退出, swg.Done -> swg.Wait -> close(outC) 链路解开.
//
// 关键词: _scanFromTargetStream 泄漏回归, ctx cancel inner goroutine 退出
func TestMUSTPASS_Fingerprint_ScanFromTargetStream_CtxCancel_NoLeak(t *testing.T) {
	host, port, closeFn := startBlackHoleListener(t)
	defer closeFn()

	runtime.GC()
	runtime.Gosched()
	baseline := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())

	// 注入 50 个 syn 结果 (concurrent 池默认 50, 让池满+排队两态都覆盖).
	const synCount = 50
	syns := make(chan *synscan.SynScanResult, synCount)
	for i := 0; i < synCount; i++ {
		syns <- &synscan.SynScanResult{Host: host, Port: port}
	}
	close(syns)

	outC, err := _scanFromTargetStream(
		syns,
		fp.WithCtx(ctx),
		fp.WithPoolSize(50),
		fp.WithProbeTimeoutHumanRead(2.0),
	)
	if err != nil {
		t.Fatalf("_scanFromTargetStream returned error: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	cancel()

	// 故意不读 outC: 模拟 yak VM 提前退出 range 的真实场景.
	_ = outC

	got := waitGoroutineConverge(t, baseline, 8, 5*time.Second)
	if got-baseline > 8 {
		t.Fatalf(
			"goroutine leak detected: baseline=%d after=%d diff=%d",
			baseline, got, got-baseline,
		)
	}
	t.Logf("goroutine converged: baseline=%d after=%d diff=%d", baseline, got, got-baseline)
}

// TestMUSTPASS_Fingerprint_InFlightCounter_BalancedAfterScan 验证
// fingerprintInFlight 计数器在扫描完成 (含 ctx cancel) 后回到 0, 不会出现
// "计数一直高位漂浮" 的现象 (那种现象通常意味着 goroutine 泄漏).
//
// 关键词: fingerprintInFlight inc/dec 平衡, 可观测性回归
func TestMUSTPASS_Fingerprint_InFlightCounter_BalancedAfterScan(t *testing.T) {
	host, port, closeFn := startBlackHoleListener(t)
	defer closeFn()

	startInFlight := GetInFlightFingerprintScans()

	ctx, cancel := context.WithCancel(context.Background())

	const synCount = 30
	syns := make(chan *synscan.SynScanResult, synCount)
	for i := 0; i < synCount; i++ {
		syns <- &synscan.SynScanResult{Host: host, Port: port}
	}
	close(syns)

	outC, err := _scanFromTargetStream(
		syns,
		fp.WithCtx(ctx),
		fp.WithPoolSize(30),
		fp.WithProbeTimeoutHumanRead(2.0),
	)
	if err != nil {
		t.Fatalf("_scanFromTargetStream returned error: %v", err)
	}

	time.Sleep(120 * time.Millisecond)
	cancel()
	_ = outC

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if GetInFlightFingerprintScans()-startInFlight <= 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if got := GetInFlightFingerprintScans() - startInFlight; got > 0 {
		t.Fatalf("fingerprintInFlight did not return to baseline: start=%d delta=%d", startInFlight, got)
	}
}

// TestMUSTPASS_Fingerprint_ScanFingerprint_CtxCancel_NoLeak 集成测
// _scanFingerprint (与 _scanFromTargetStream 共享同一份 send helper).
//
// 关键词: _scanFingerprint 泄漏回归, ctx cancel
func TestMUSTPASS_Fingerprint_ScanFingerprint_CtxCancel_NoLeak(t *testing.T) {
	host, port, closeFn := startBlackHoleListener(t)
	defer closeFn()

	runtime.GC()
	runtime.Gosched()
	baseline := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())

	config := fp.NewConfig(
		fp.WithCtx(ctx),
		fp.WithPoolSize(20),
		fp.WithProbeTimeoutHumanRead(2.0),
	)

	// 用 20 个端口形成池满后还有排队的状态.
	// port 指向 listener, port+1..+19 没有 listener, 这样 dial 会立即拒绝,
	// fp 走快速失败路径, 让 inner goroutine 在 ctx cancel 之前就开始堆积.
	ports := []int{port}
	for i := 1; i < 20; i++ {
		ports = append(ports, port+i)
	}
	portStr := ""
	for i, p := range ports {
		if i == 0 {
			portStr = strconv.Itoa(p)
			continue
		}
		portStr += "," + strconv.Itoa(p)
	}

	outC, err := _scanFingerprint(ctx, config, 20, host, portStr)
	if err != nil {
		t.Fatalf("_scanFingerprint returned error: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	cancel()

	_ = outC

	got := waitGoroutineConverge(t, baseline, 12, 6*time.Second)
	if got-baseline > 12 {
		t.Fatalf(
			"goroutine leak detected: baseline=%d after=%d diff=%d",
			baseline, got, got-baseline,
		)
	}
	t.Logf("goroutine converged: baseline=%d after=%d diff=%d", baseline, got, got-baseline)
}
