package aibalance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestOutputTPSLimiter_Disabled verifies that limit<=0 disables the limiter
// (Throttle always returns 0).
// 关键词: OutputTPSLimiter 禁用, limit<=0
func TestOutputTPSLimiter_Disabled(t *testing.T) {
	l := NewOutputTPSLimiter(0)
	assert.Equal(t, time.Duration(0), l.Throttle(100))
	assert.Equal(t, time.Duration(0), l.Throttle(10000))

	l2 := NewOutputTPSLimiter(-5)
	assert.Equal(t, time.Duration(0), l2.Throttle(1))
}

// TestOutputTPSLimiter_NilSafe verifies that nil receiver is safe.
// 关键词: OutputTPSLimiter nil-safe
func TestOutputTPSLimiter_NilSafe(t *testing.T) {
	var l *OutputTPSLimiter
	assert.Equal(t, time.Duration(0), l.Throttle(100))
	assert.Equal(t, int64(0), l.Limit())
	l.SetLimit(10) // should not panic
	l.Reset()      // should not panic
}

// TestOutputTPSLimiter_BasicSleep verifies that with limit=10 tokens/sec,
// writing 30 tokens at once requires roughly 3 seconds of sleep on the first
// call (because startTime is initialized on first call).
// 我们不真正 sleep，只断言返回的 deficit 长度在合理区间。
// 关键词: OutputTPSLimiter 累计补偿, 首次写入 deficit
func TestOutputTPSLimiter_BasicSleep(t *testing.T) {
	l := NewOutputTPSLimiter(10) // 10 token/sec
	// 第一次写入 30 token：startTime 立即初始化，elapsed≈0，
	// expected = 30 * 1e9 / 10 = 3s -> 返回 ~3s 的 deficit。
	deficit := l.Throttle(30)
	assert.GreaterOrEqual(t, deficit, 2900*time.Millisecond, "deficit should be ~3s")
	assert.LessOrEqual(t, deficit, 3*time.Second+50*time.Millisecond)
}

// TestOutputTPSLimiter_NoCatchUpNeeded verifies that if enough wall-clock time
// has passed between writes, no sleep is required.
// 关键词: OutputTPSLimiter 已追上, 0 deficit
func TestOutputTPSLimiter_NoCatchUpNeeded(t *testing.T) {
	l := NewOutputTPSLimiter(1000) // very high limit
	d1 := l.Throttle(1)            // first write
	assert.LessOrEqual(t, d1, 5*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	d2 := l.Throttle(1)
	assert.Equal(t, time.Duration(0), d2, "after sleeping past expected pace, no deficit")
}

// TestOutputTPSLimiter_CumulativeBudget verifies that the limiter measures
// cumulative tokens vs. cumulative elapsed time, not per-write rate.
// 关键词: OutputTPSLimiter 累计预算
func TestOutputTPSLimiter_CumulativeBudget(t *testing.T) {
	l := NewOutputTPSLimiter(100) // 100 token/sec
	// 1) 第一次写 50 token: expected=500ms, elapsed≈0 -> deficit≈500ms
	d1 := l.Throttle(50)
	assert.InDelta(t, float64(500*time.Millisecond), float64(d1), float64(50*time.Millisecond))

	// 模拟实际 sleep 之后再调用一次（这里直接 sleep 实现等待）
	time.Sleep(d1)

	// 2) 再写 50 token: expected=1s, elapsed≈500ms -> deficit≈500ms
	d2 := l.Throttle(50)
	assert.InDelta(t, float64(500*time.Millisecond), float64(d2), float64(80*time.Millisecond))
}

// TestOutputTPSLimiter_SetLimit_Dynamic verifies that SetLimit takes effect
// without resetting accumulated state.
// 关键词: OutputTPSLimiter SetLimit 动态
func TestOutputTPSLimiter_SetLimit_Dynamic(t *testing.T) {
	l := NewOutputTPSLimiter(10)
	_ = l.Throttle(5)
	assert.Equal(t, int64(10), l.Limit())

	l.SetLimit(50)
	assert.Equal(t, int64(50), l.Limit())

	l.SetLimit(0)
	assert.Equal(t, int64(0), l.Limit())
	assert.Equal(t, time.Duration(0), l.Throttle(100), "after disabling, no throttle")
}

// TestOutputTPSLimiter_Reset verifies Reset wipes accumulated state.
// 关键词: OutputTPSLimiter Reset 归零
func TestOutputTPSLimiter_Reset(t *testing.T) {
	l := NewOutputTPSLimiter(10)
	_ = l.Throttle(100)
	l.Reset()
	// After reset, startTime is zero again; the next Throttle should treat
	// itself as the first call.
	d := l.Throttle(30)
	assert.GreaterOrEqual(t, d, 2900*time.Millisecond)
	assert.LessOrEqual(t, d, 3*time.Second+50*time.Millisecond)
}
