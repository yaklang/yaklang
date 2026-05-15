package aibalance

import (
	"bytes"
	"context"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

func dumpLifecycleTestGoroutines() string {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
	return buf.String()
}

func countGoroutinesBySignature(signature string) int {
	count := 0
	for _, block := range strings.Split(dumpLifecycleTestGoroutines(), "\n\n") {
		if strings.Contains(block, signature) {
			count++
		}
	}
	return count
}

func waitForGoroutineCount(t *testing.T, signature string, expected int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		runtime.GC()
		current := countGoroutinesBySignature(signature)
		if current == expected {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine count for %s did not converge: want=%d got=%d", signature, expected, current)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForGoroutineCountAtLeast(t *testing.T, signature string, minimum int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		current := countGoroutinesBySignature(signature)
		if current >= minimum {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine count for %s did not reach minimum: want>=%d got=%d", signature, minimum, current)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestBalancerCloseReleasesRateLimiterGoroutines 验证 rate limiter
// 的后台 cleanup goroutine 在 Balancer.Close 后被正确释放。
//
// 注意：WebSearchRateLimiter / AmapRateLimiter 现在采用 lazy 启动
// (sync.Once + ensureCleanupStarted)，构造时不会创建后台 goroutine，
// 只在第一次 CheckRateLimit / WaitForRateLimit 时才 fire。
// 因此本测试在 NewServerConfig() 之后必须显式触发一次使用，让 cleanup
// 进入运行状态，再验证 Close 能把它收回。
//
// 关键词: TestBalancerCloseReleasesRateLimiterGoroutines, lazy cleanup goroutine 释放验证
func TestBalancerCloseReleasesRateLimiterGoroutines(t *testing.T) {
	const balancerCount = 4
	const webLimiterSig = "aibalance.(*WebSearchRateLimiter).cleanupLoop"
	const amapLimiterSig = "aibalance.(*AmapRateLimiter).cleanupLoop"

	baseWeb := countGoroutinesBySignature(webLimiterSig)
	baseAmap := countGoroutinesBySignature(amapLimiterSig)

	balancers := make([]*Balancer, 0, balancerCount)
	for i := 0; i < balancerCount; i++ {
		cfg := NewServerConfig()
		// 触发 lazy 启动：构造后 limiter 默认不会启动 cleanup goroutine，
		// 用一次 dummy 检查把 startOnce 走完，与生产路径首次接到请求时的
		// 行为一致。
		// 关键词: 触发 lazy cleanup, dummy CheckRateLimit
		cfg.webSearchRateLimiter.CheckRateLimit("lifecycle-test-trigger")
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_ = cfg.amapRateLimiter.WaitForRateLimit("lifecycle-test-trigger", ctx)
		cancel()
		balancers = append(balancers, &Balancer{config: cfg})
	}

	waitForGoroutineCountAtLeast(t, webLimiterSig, baseWeb+balancerCount, 2*time.Second)
	waitForGoroutineCountAtLeast(t, amapLimiterSig, baseAmap+balancerCount, 2*time.Second)

	for _, balancer := range balancers {
		if err := balancer.Close(); err != nil {
			t.Fatalf("close balancer failed: %v", err)
		}
	}

	waitForGoroutineCount(t, webLimiterSig, baseWeb, 3*time.Second)
	waitForGoroutineCount(t, amapLimiterSig, baseAmap, 3*time.Second)
}

func TestServerConfigCloseIsIdempotent(t *testing.T) {
	cfg := NewServerConfig()
	cfg.Close()
	cfg.Close()

	select {
	case <-cfg.amapHealthCheckStopCh:
	default:
		t.Fatal("expected amap health check stop channel to be closed")
	}
}
