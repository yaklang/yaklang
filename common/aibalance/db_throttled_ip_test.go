package aibalance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: db_throttled_ip_test, 一键限流 IP 持久化/缓存/限流器单元测试

func cleanupThrottledIP(t *testing.T, ip string) {
	require.NoError(t, DeleteThrottledIP(ip))
}

func TestEnsureThrottledIPTable(t *testing.T) {
	require.NoError(t, EnsureThrottledIPTable())
}

// TestUpsertAndLookupThrottledIP 验证：写入限流 IP 后缓存命中且值正确；解除后缓存清空。
// 关键词: TestUpsertAndLookupThrottledIP, 一键限流读写缓存
func TestUpsertAndLookupThrottledIP(t *testing.T) {
	require.NoError(t, EnsureThrottledIPTable())

	ip := "203.0.113.200"
	defer cleanupThrottledIP(t, ip)

	// 初始未限流
	_, _, ok := lookupThrottledIP(ip)
	assert.False(t, ok, "ip should not be throttled initially")

	require.NoError(t, UpsertThrottledIP(ip, 3, 15, "abuse"))
	rpm, tps, ok := lookupThrottledIP(ip)
	assert.True(t, ok)
	assert.Equal(t, int64(3), rpm)
	assert.Equal(t, int64(15), tps)
	assert.True(t, IsIPThrottled(ip))

	// 覆盖更新
	require.NoError(t, UpsertThrottledIP(ip, 5, 20, "updated"))
	rpm, tps, ok = lookupThrottledIP(ip)
	assert.True(t, ok)
	assert.Equal(t, int64(5), rpm)
	assert.Equal(t, int64(20), tps)

	// 解除
	require.NoError(t, DeleteThrottledIP(ip))
	_, _, ok = lookupThrottledIP(ip)
	assert.False(t, ok, "ip should be cleared after delete")
	assert.False(t, IsIPThrottled(ip))
}

// TestUpsertThrottledIP_InvalidIP 验证：空 / unknown IP 拒绝写入。
// 关键词: TestUpsertThrottledIP_InvalidIP, 非法 IP 拒绝
func TestUpsertThrottledIP_InvalidIP(t *testing.T) {
	require.NoError(t, EnsureThrottledIPTable())
	assert.Error(t, UpsertThrottledIP("", 3, 15, ""))
	assert.Error(t, UpsertThrottledIP("  ", 3, 15, ""))
	assert.Error(t, UpsertThrottledIP("unknown", 3, 15, ""))
}

// TestReloadThrottledIPCache 验证：从 DB 全量重建缓存可恢复限流状态。
// 关键词: TestReloadThrottledIPCache, 缓存重建
func TestReloadThrottledIPCache(t *testing.T) {
	require.NoError(t, EnsureThrottledIPTable())

	ip := "203.0.113.201"
	defer cleanupThrottledIP(t, ip)

	require.NoError(t, UpsertThrottledIP(ip, 7, 21, "persist"))

	// 手动清空进程内缓存，再从 DB 重建
	throttledIPMu.Lock()
	throttledIPCache = map[string]throttledIPEntry{}
	throttledIPMu.Unlock()
	_, _, ok := lookupThrottledIP(ip)
	assert.False(t, ok, "cache cleared")

	require.NoError(t, ReloadThrottledIPCache())
	rpm, tps, ok := lookupThrottledIP(ip)
	assert.True(t, ok, "reload should restore from DB")
	assert.Equal(t, int64(7), rpm)
	assert.Equal(t, int64(21), tps)
}

// TestCheckIPRateLimit 验证：按 IP 维度 RPM 滑动窗口在达到上限后拒绝；rpm<=0 放行。
// 关键词: TestCheckIPRateLimit, per-IP RPM 滑动窗口
func TestCheckIPRateLimit(t *testing.T) {
	rl := NewChatRateLimiter()
	defer rl.Stop()

	ip := "203.0.113.202"
	// rpm=3：前 3 次放行，第 4 次拒绝
	for i := 0; i < 3; i++ {
		allowed, _ := rl.CheckIPRateLimit(ip, 3)
		assert.True(t, allowed, "request %d should be allowed", i+1)
	}
	allowed, qlen := rl.CheckIPRateLimit(ip, 3)
	assert.False(t, allowed, "4th request should be rejected")
	assert.GreaterOrEqual(t, qlen, int64(1))

	// 不同 IP 互不影响
	allowed2, _ := rl.CheckIPRateLimit("203.0.113.203", 3)
	assert.True(t, allowed2, "different ip uses an independent bucket")

	// rpm<=0 / 空 IP 一律放行
	for i := 0; i < 10; i++ {
		allowed3, _ := rl.CheckIPRateLimit(ip, 0)
		assert.True(t, allowed3, "rpm<=0 must allow")
	}
	allowed4, _ := rl.CheckIPRateLimit("", 3)
	assert.True(t, allowed4, "empty ip must allow")
}

// TestThrottledIPDefaultsInConfig 验证：限流默认值在配置中具备 3/15 兜底。
// 关键词: TestThrottledIPDefaultsInConfig, 一键限流默认值兜底
func TestThrottledIPDefaultsInConfig(t *testing.T) {
	require.NoError(t, EnsureRateLimitConfigTable())
	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	assert.Greater(t, cfg.ThrottledIPDefaultRPM, int64(0))
	assert.Greater(t, cfg.ThrottledIPDefaultTPS, int64(0))
}
