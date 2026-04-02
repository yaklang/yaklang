package aibalance

import (
	"bytes"
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

func TestBalancerCloseReleasesRateLimiterGoroutines(t *testing.T) {
	const balancerCount = 4
	const webLimiterSig = "aibalance.(*WebSearchRateLimiter).cleanupLoop"
	const amapLimiterSig = "aibalance.(*AmapRateLimiter).cleanupLoop"

	baseWeb := countGoroutinesBySignature(webLimiterSig)
	baseAmap := countGoroutinesBySignature(amapLimiterSig)

	balancers := make([]*Balancer, 0, balancerCount)
	for i := 0; i < balancerCount; i++ {
		balancers = append(balancers, &Balancer{config: NewServerConfig()})
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
