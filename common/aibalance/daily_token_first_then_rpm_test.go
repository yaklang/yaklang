package aibalance

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: TestE2E_DailyTokenBeforeRPM_DoesNotPollute_RPMBucket
//
// 验证 daily token check 已经被前置到 RPM check 之前：
//
//   - 全局桶预先用满 -> 任何 free model 请求应被 daily_token 429
//   - 同时 chatRateLimiter 的 (apiKeyForStat="free-user", modelName) 桶
//     不应该因为这些被挡掉的请求多记一笔 timestamp。
//   - 也就是 GetModelRPMStats 不应该把"被 daily token 挡的请求"算进去，
//     portal 的"最近 RPM"显示与"真实转发流量"对齐。
//
// 这条契约的退化会导致 portal 出现"已超额却还有 RPM 在跑"的误导现象。
// 关键词: daily token 先于 RPM, RPM 桶不脱 +1, portal 数字直观
func TestE2E_DailyTokenBeforeRPM_DoesNotPollute_RPMBucket(t *testing.T) {
	addr, cfg, _, cleanup := startDailyTokenTestServer(t, 421)
	defer cleanup()

	const model = "pollute-check-free"
	const apiKeyForFree = "free-user" // server.go: isFreeModel -> apiKeyForStat = "free-user"

	// 全局桶预先填满，让 daily token check 必然拒绝
	setRateLimitConfigForFreeTokenTest(t, 2, "{}")
	require.NoError(t, AddFreeUserDailyTokenUsage("any-other-free", 2*FreeUserTokenMUnit, false))

	// 起始基线：模型 RPM 桶为空
	baseline := cfg.chatRateLimiter.GetModelRPMStats(0)
	for _, m := range baseline {
		require.NotEqual(t, model, m.Model, "test model bucket must be empty before run")
	}

	// 连发 3 个请求，全部预期 429 daily_token
	const totalAttempts = 3
	for i := 0; i < totalAttempts; i++ {
		status, headers, _ := sendChatCompletionHTTP(t, addr, "", model)
		assert.Equal(t, http.StatusTooManyRequests, status,
			"attempt #%d should be 429 daily_token", i+1)
		assert.Equal(t, "daily_token", headers["x-aibalance-limit-kind"],
			"attempt #%d should be marked as daily_token kind", i+1)
	}

	// 关键断言：RPM 桶不应包含被 daily token 挡掉的请求
	// 在 GetModelRPMStats 里查 model 是否被记录。
	// 注意：min_rpm=0 表示返回所有有数据的模型，1 表示只返回 RPM>=1
	stats := cfg.chatRateLimiter.GetModelRPMStats(0)
	for _, m := range stats {
		if m.Model == model {
			assert.Fail(t,
				"RPM bucket should NOT be polluted by daily_token-rejected requests",
				"got model=%s rpm=%d (expected 0 entries, since daily_token check is now BEFORE RPM check)",
				m.Model, m.RPM)
		}
	}
}

// TestE2E_DailyTokenBeforeRPM_AllowedRequest_StillBumpsRPM
// 反向断言：daily token 放行后，RPM 桶仍应记录该请求。
// 关键词: daily token 放行 -> RPM 桶正常 +1
func TestE2E_DailyTokenBeforeRPM_AllowedRequest_StillBumpsRPM(t *testing.T) {
	addr, cfg, _, cleanup := startDailyTokenTestServer(t, 422)
	defer cleanup()

	const model = "allow-then-rpm-free"

	// 全局桶很大、未填，daily check 必然放行
	setRateLimitConfigForFreeTokenTest(t, 10000, "{}")

	// 起始基线
	baseline := cfg.chatRateLimiter.GetModelRPMStats(0)
	for _, m := range baseline {
		require.NotEqual(t, model, m.Model)
	}

	// 发 2 个请求；daily check 放行后会被下游 provider 缺失打挂（非 429）。
	// 但 RPM 桶应该记录这 2 笔（因为 daily check 已通过，到了 RPM gate）。
	for i := 0; i < 2; i++ {
		status, headers, _ := sendChatCompletionHTTP(t, addr, "", model)
		assert.NotEqual(t, http.StatusTooManyRequests, status,
			"attempt #%d should not be 429 (daily allows, no rpm cap configured)", i+1)
		_ = headers
	}

	// 给 limiter 一点时间稳态（time.Now() 已经在 CheckRateLimit 内完成 append）
	time.Sleep(50 * time.Millisecond)

	stats := cfg.chatRateLimiter.GetModelRPMStats(0)
	var found *ModelRPMStat
	for i := range stats {
		if stats[i].Model == model {
			found = &stats[i]
			break
		}
	}
	require.NotNil(t, found, "RPM bucket should have entries for allowed requests")
	assert.Equal(t, int64(2), found.RPM,
		"RPM bucket should have exactly 2 timestamps for 2 allowed requests")
}
