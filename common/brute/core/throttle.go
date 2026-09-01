package core

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// rateLimiter 是无后台 goroutine 的惰性令牌桶：
// 仅在 Take 时按时间差补充令牌，取消通过 ctx 立即返回。
type rateLimiter struct {
	mu       sync.Mutex
	interval time.Duration // 0 表示不限速
	burst    int
	tokens   float64
	last     time.Time
	jitter   time.Duration // 每次取令牌后的随机抖动上限
	rng      *rand.Rand
}

func newRateLimiter(perSecond float64, jitter time.Duration) *rateLimiter {
	rl := &rateLimiter{jitter: jitter, rng: rand.New(rand.NewSource(time.Now().UnixNano()))}
	if perSecond > 0 {
		rl.interval = time.Duration(float64(time.Second) / perSecond)
		rl.burst = 1
		rl.tokens = 1
	}
	rl.last = time.Now()
	return rl
}

// Take 阻塞直到取得令牌或 ctx 结束。
func (rl *rateLimiter) Take(ctx context.Context) error {
	if rl.interval <= 0 {
		// 仅抖动
		if rl.jitter > 0 {
			return sleepCtx(ctx, time.Duration(rl.rng.Int63n(int64(rl.jitter))))
		}
		return ctx.Err()
	}
	for {
		rl.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(rl.last)
		rl.last = now
		rl.tokens += float64(elapsed) / float64(rl.interval)
		if rl.tokens > float64(rl.burst) {
			rl.tokens = float64(rl.burst)
		}
		if rl.tokens >= 1 {
			rl.tokens--
			jitter := time.Duration(0)
			if rl.jitter > 0 {
				jitter = time.Duration(rl.rng.Int63n(int64(rl.jitter)))
			}
			rl.mu.Unlock()
			if jitter > 0 {
				if err := sleepCtx(ctx, jitter); err != nil {
					return err
				}
			}
			return nil
		}
		wait := time.Duration((1 - rl.tokens) * float64(rl.interval))
		rl.mu.Unlock()
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
	}
}

// sleepCtx 是可被取消的 sleep，不创建额外 goroutine。
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// perTargetGate 是单目标/单服务并发信号量。
// 基于 channel 缓冲实现，Add 阻塞在 ctx 上，取消立即返回。
type chanGate struct {
	ch chan struct{}
}

func newChanGate(limit int) *chanGate {
	if limit <= 0 {
		limit = 1
	}
	return &chanGate{ch: make(chan struct{}, limit)}
}

func (g *chanGate) acquire(ctx context.Context) error {
	select {
	case g.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *chanGate) release() {
	select {
	case <-g.ch:
	default:
	}
}
