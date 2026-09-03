package core

import (
	"context"
	"sync"
	"testing"
	"time"
)

// 复现：两并发 Take，5s 间隔令牌 + 4s jitter —— 第二个应在 ~10s 内拿到
func TestRateLimiterTwoConcurrentTakes(t *testing.T) {
	rl := newRateLimiter(0.2, 4*time.Second)
	var wg sync.WaitGroup
	times := make([]time.Duration, 2)
	start := time.Now()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := rl.Take(ctx); err != nil {
				t.Errorf("take %d failed: %v", i, err)
				return
			}
			times[i] = time.Since(start)
		}(i)
	}
	wg.Wait()
	t.Logf("take0=%v take1=%v", times[0].Round(time.Second), times[1].Round(time.Second))
	if times[1] > 15*time.Second {
		t.Fatalf("second take too slow: %v", times[1])
	}
}
