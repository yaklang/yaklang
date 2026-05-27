package aibalance

import (
	"sync"
	"time"
)

// OutputTPSLimiter 是一个 token-per-second 限速器，按累计 token 数与
// 实际经过的时间反推应有的 sleep 时长，强行把流式输出的吞吐拉低到 limit。
//
// 工作模式：
//   - 每次写入若干 token 后调用 Throttle(n)。
//   - limiter 内部跟踪 startTime 与 tokensWritten。
//   - 期望完成时间 expected = tokensWritten / limit (秒)。
//   - elapsed = now - startTime。
//   - 若 expected > elapsed，返回需要补偿 sleep 的差值；否则返回 0。
//
// 该实现是单流单实例，假定每个 chatJSONChunkWriter 持有一个独立的
// limiter，不在多个请求之间共享。limit <= 0 时 Throttle 始终返回 0。
//
// 关键词: OutputTPSLimiter, token-per-second 流式节流, 累计补偿 sleep
type OutputTPSLimiter struct {
	mu            sync.Mutex
	limit         int64 // tokens/sec; <=0 disabled
	startTime     time.Time
	tokensWritten int64
}

// NewOutputTPSLimiter 创建一个新的 TPS 限速器；limit <=0 表示不限速。
// 关键词: NewOutputTPSLimiter
func NewOutputTPSLimiter(limit int64) *OutputTPSLimiter {
	return &OutputTPSLimiter{limit: limit}
}

// SetLimit 动态调整 TPS 上限；<=0 关闭限速。
// 修改 limit 不会重置已有的 startTime / tokensWritten，保证语义连续。
// 关键词: OutputTPSLimiter SetLimit
func (l *OutputTPSLimiter) SetLimit(limit int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = limit
}

// Limit 返回当前 TPS 上限（线程安全）。
// 关键词: OutputTPSLimiter Limit
func (l *OutputTPSLimiter) Limit() int64 {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// Throttle 累计本次写入的 tokensThisWrite 数量，并返回需要 sleep 的时长。
// 返回 0 表示无需 sleep（limit 关闭，或当前 elapsed 已经追上 expected）。
//
// 在第一次调用时把 startTime 初始化为当前时间，避免空闲期被计入。
// 关键词: OutputTPSLimiter Throttle, 累计补偿
func (l *OutputTPSLimiter) Throttle(tokensThisWrite int64) time.Duration {
	if l == nil || tokensThisWrite <= 0 {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.limit <= 0 {
		return 0
	}
	if l.startTime.IsZero() {
		l.startTime = time.Now()
	}
	l.tokensWritten += tokensThisWrite

	// expected duration in nanoseconds = tokensWritten * 1e9 / limit
	expectedNs := l.tokensWritten * int64(time.Second) / l.limit
	elapsed := time.Since(l.startTime)
	deficit := time.Duration(expectedNs) - elapsed
	if deficit <= 0 {
		return 0
	}
	return deficit
}

// Reset 把累计状态归零，主要用于测试与显式复位场景。
// 关键词: OutputTPSLimiter Reset
func (l *OutputTPSLimiter) Reset() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.startTime = time.Time{}
	l.tokensWritten = 0
}
