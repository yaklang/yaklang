package tools

import (
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/fp"
)

// fingerprint_scan_guard_test 验证 hostPortGuard 单主机端口数熔断器的行为.
//
// 关键词: hostPortGuard 熔断单测, scan port 单主机端口阈值, tarpit 防护

// TestMUSTPASS_HostPortGuard_TripsOnLimit 验证同一主机达到 limit 时触发熔断,
// 且触发后对任意主机的 observe 都返回 true (上层能稳定走到强制停止分支).
func TestMUSTPASS_HostPortGuard_TripsOnLimit(t *testing.T) {
	g := newHostPortGuard(3)

	if g.observe("1.1.1.1") {
		t.Fatal("first observe should not trip")
	}
	if g.observe("1.1.1.1") {
		t.Fatal("second observe should not trip")
	}
	if !g.observe("1.1.1.1") {
		t.Fatal("third observe should trip at limit=3")
	}
	// 触发后, 对其他主机也应当返回 true, 让上层一致地停止.
	if !g.observe("2.2.2.2") {
		t.Fatal("after tripped, observe on any host should return true")
	}
}

// TestMUSTPASS_HostPortGuard_PerHostIndependent 验证不同主机的计数相互独立,
// 单个主机端口数低于阈值时不会误触发熔断.
func TestMUSTPASS_HostPortGuard_PerHostIndependent(t *testing.T) {
	g := newHostPortGuard(3)
	hosts := []string{"a", "b", "c", "d", "e"}
	// 每个主机各 observe 2 次 (低于阈值 3), 不应触发.
	for i := 0; i < 2; i++ {
		for _, h := range hosts {
			if g.observe(h) {
				t.Fatalf("host %s tripped below limit unexpectedly", h)
			}
		}
	}
}

// TestMUSTPASS_HostPortGuard_DisabledWhenNonPositiveLimit 验证 limit <= 0 时
// 熔断器被禁用, observe 恒返回 false.
func TestMUSTPASS_HostPortGuard_DisabledWhenNonPositiveLimit(t *testing.T) {
	for _, limit := range []int{0, -1} {
		g := newHostPortGuard(limit)
		for i := 0; i < 1000; i++ {
			if g.observe("1.1.1.1") {
				t.Fatalf("guard with limit=%d should never trip", limit)
			}
		}
	}
	// nil guard 也应安全且恒返回 false.
	var nilGuard *hostPortGuard
	if nilGuard.observe("1.1.1.1") {
		t.Fatal("nil guard should never trip")
	}
}

// TestMUSTPASS_HostPortGuard_ConcurrentSafe 验证并发 observe 不会 data race,
// 并且最终一定进入 tripped 状态 (用 -race 运行时可检测竞态).
func TestMUSTPASS_HostPortGuard_ConcurrentSafe(t *testing.T) {
	g := newHostPortGuard(50)
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.observe("same-host")
		}()
	}
	wg.Wait()
	if !g.observe("same-host") {
		t.Fatal("guard should be tripped after 200 concurrent observes on one host with limit=50")
	}
}

func TestMUSTPASS_OpenPortGuardLimit_DisabledByConfig(t *testing.T) {
	if got := openPortGuardLimit(fp.NewConfig()); got != maxOpenPortsPerHost {
		t.Fatalf("default guard limit = %d, want %d", got, maxOpenPortsPerHost)
	}
	if got := openPortGuardLimit(fp.NewConfig(fp.WithOpenPortGuardLimit(42))); got != 42 {
		t.Fatalf("custom guard limit = %d, want 42", got)
	}
	if got := openPortGuardLimit(fp.NewConfig(fp.WithDisableOpenPortGuard())); got != 0 {
		t.Fatalf("disabled guard limit = %d, want 0", got)
	}
	if got := openPortGuardLimit(fp.NewConfig(fp.WithOpenPortGuardLimit(42), fp.WithDisableOpenPortGuard())); got != 0 {
		t.Fatalf("disabled guard should override custom limit, got %d", got)
	}
}
