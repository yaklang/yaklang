package core_test

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/brute/core"
)

// TestNoDriverImports 强制保证 brute/core 的依赖图中不存在任何数据库驱动
// 或具体协议实现 —— 这是精简构建的前提（任务验收标准第一条）。
func TestNoDriverImports(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "github.com/yaklang/yaklang/common/brute/core")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("go list failed: %v", err)
	}
	banned := []string{
		"github.com/go-sql-driver/mysql",
		"github.com/denisenkom/go-mssqldb",
		"go.mongodb.org/mongo-driver",
		"github.com/go-pg/pg",
		"github.com/lib/pq",
		"github.com/sijms/go-ora",
		"golang.org/x/crypto/ssh",
		"github.com/go-ldap/ldap",
	}
	for _, pkg := range strings.Split(string(out), "\n") {
		pkg = strings.TrimSpace(pkg)
		for _, b := range banned {
			if pkg == b || strings.HasPrefix(pkg, b+"/") {
				t.Fatalf("brute/core must not depend on driver %s (found in dep graph)", pkg)
			}
		}
	}
}

// TestMemoryBoundedOnLargeCombinationSpace 证明：组合空间 T×U×P 巨大时，
// 内存占用不随完整组合数线性增长（有界队列 + 惰性生成）。
func TestMemoryBoundedOnLargeCombinationSpace(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	const targets = 5
	const users = 100
	const passwords = 200 // 组合空间 10 万

	prober := core.ProberFunc(func(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
		time.Sleep(200 * time.Microsecond)
		return core.Result{Outcome: core.OutcomeAuthFailed}
	})

	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	s := &core.Scheduler{
		Prober:            prober,
		Protocol:          "memtest",
		GlobalConcurrency: 8,
		TargetConcurrency: 2,
		QueueSize:         128, // 故意小队列，验证有界性
		Sink:              func(core.Result) {},
	}
	start := time.Now()
	stats, err := s.Run(context.Background(), core.NewCartesianSource(
		genN(targets, "t%d.example:1"), genN(users, "u%d"), genN(passwords, "p%d")))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	runtime.ReadMemStats(&m1)
	heapGrew := int64(m1.HeapAlloc) - int64(m0.HeapAlloc)
	if heapGrew < 0 {
		heapGrew = 0
	}

	// 完整物化 10 万组合（每条含字符串约百字节）需要 ~10MB 量级；
	// 有界调度器的堆增长必须远小于该量级（阈值 8MB，留足冗余）。
	t.Logf("generated=%d executed=%d elapsed=%s heap grew=%d bytes",
		stats.Generated, stats.Executed, elapsed, heapGrew)
	if heapGrew > 8<<20 {
		t.Fatalf("heap grew %d bytes; scheduler appears to materialize the full product", heapGrew)
	}
	if stats.Executed != int64(targets*users*passwords) {
		t.Fatalf("expected all %d combos executed, got %d", targets*users*passwords, stats.Executed)
	}
}

func genN(n int, pattern string) []string {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = fmt.Sprintf(pattern, i)
	}
	return out
}

// blackholeListener 接受连接后不读不写，用于验证取消能中断阻塞的 I/O。
type blackholeListener struct {
	net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func (b *blackholeListener) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.conns {
		_ = c.Close()
	}
	return b.Listener.Close()
}

// TestCancellationLatency 验证：取消后，被网络 I/O 阻塞的探测在期限内退出
// （探针必须用 ctx 控制连接 deadline）。
func TestCancellationLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bh := &blackholeListener{Listener: l}
	defer bh.Close()
	go func() {
		for {
			conn, err := bh.Accept()
			if err != nil {
				return
			}
			bh.mu.Lock()
			bh.conns = append(bh.conns, conn)
			bh.mu.Unlock()
		}
	}()

	prober := core.ProberFunc(func(ctx context.Context, target core.Target, cred core.Credential, opts core.Options) core.Result {
		conn, transport, err := core.Dialer(ctx, target.String(), opts.TLSPolicy, opts.Timeout)
		if err != nil {
			return classifyDial(err)
		}
		defer core.SafeClose(conn)
		unwatch := core.WatchConn(ctx, conn)
		defer unwatch()
		core.SetDeadline(conn, ctx, opts.Timeout)
		buf := make([]byte, 16)
		if _, err := conn.Read(buf); err != nil {
			if ctx.Err() != nil {
				return core.Result{Outcome: core.OutcomeCancelled, Err: core.ErrDeadline}
			}
			return core.Result{Outcome: core.OutcomeTargetUnavailable, Err: core.ErrIO}
		}
		return core.Result{Outcome: core.OutcomeAuthFailed, Transport: transport}
	})

	ctx, cancel := context.WithCancel(context.Background())
	s := &core.Scheduler{
		Prober:            prober,
		Protocol:          "cancel",
		GlobalConcurrency: 4,
		TargetConcurrency: 1,
		QueueSize:         16,
		DefaultTimeout:    30 * time.Second, // 远大于取消验证窗口
		Sink:              func(core.Result) {},
	}
	done := make(chan struct{})
	go func() {
		_, _ = s.Run(ctx, core.NewCartesianSource(
			[]string{bh.Addr().String()}, []string{"u"}, []string{"p1", "p2", "p3", "p4"}))
		close(done)
	}()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case <-done:
		latency := time.Since(start)
		if latency > 2*time.Second {
			t.Fatalf("cancellation latency too high: %v", latency)
		}
		t.Logf("cancellation latency: %v", latency)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after cancellation")
	}
}

func classifyDial(err error) core.Result {
	if core.IsDialError(err) {
		return core.Result{Outcome: core.OutcomeTargetUnavailable, Err: core.ErrDial}
	}
	return core.Result{Outcome: core.OutcomeUnknown, Err: core.ErrIO}
}
