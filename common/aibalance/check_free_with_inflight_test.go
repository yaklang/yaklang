package aibalance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: checkFreeUserDailyTokenLimitWithInFlight 单元测试, in-flight 加入 used 判决

// TestCheckFreeUserDailyTokenLimitWithInFlight_NoInFlight_PassThrough 验证没有
// in-flight 预扣时包装版与原版判决完全一致 (回归保护)。
// 关键词: checkFreeUserDailyTokenLimitWithInFlight 无 in-flight 行为透传
func TestCheckFreeUserDailyTokenLimitWithInFlight_NoInFlight_PassThrough(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 401).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 全局 1M，无模型覆盖
	setRateLimitConfigForFreeTokenTest(t, 1, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	c := NewServerConfig()
	defer c.Close()
	// tracker 干净（无任何 Add）

	d, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed, "fresh bucket should allow")
	assert.Equal(t, int64(0), d.TokensUsed)
	assert.Equal(t, int64(1*FreeUserTokenMUnit), d.TokensLimit)
}

// TestCheckFreeUserDailyTokenLimitWithInFlight_GlobalBucket_BlocksOnInFlight
// 核心场景：bucket DB used = 0、limit = 1M，但 in-flight 已预扣 1M，
// effective_used = 1M >= limit，必须拒绝。这是过冲防御的关键证据。
// 关键词: in-flight 让全局桶提前到顶 拒绝, 过冲防御核心证据
func TestCheckFreeUserDailyTokenLimitWithInFlight_GlobalBucket_BlocksOnInFlight(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 402).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	setRateLimitConfigForFreeTokenTest(t, 1, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	c := NewServerConfig()
	defer c.Close()

	// 全局桶预扣 = 完整 1M（刚好顶到 limit）
	c.inFlightTokens.Add("", 1*FreeUserTokenMUnit)

	d, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.False(t, d.Allowed, "in-flight should push used to limit and reject")
	assert.Equal(t, int64(1*FreeUserTokenMUnit), d.TokensUsed,
		"TokensUsed must include in-flight portion (for 429 response header)")
	assert.Equal(t, int64(1*FreeUserTokenMUnit), d.TokensLimit)
}

// TestCheckFreeUserDailyTokenLimitWithInFlight_GlobalBucket_DBPlusInFlight
// 验证 DB used + in-flight 加和判决：DB 占 60%、in-flight 占 50% → 共 110% >= limit。
// 单看 DB（60%）原本会放行，加上 in-flight 必须被拒。
// 关键词: DB+in-flight 加和 越过 limit, 过冲精准防御
func TestCheckFreeUserDailyTokenLimitWithInFlight_GlobalBucket_DBPlusInFlight(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 403).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	setRateLimitConfigForFreeTokenTest(t, 10, "{}") // 10M limit
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	c := NewServerConfig()
	defer c.Close()

	require.NoError(t, AddFreeUserDailyTokenUsage("memfit-light-free", 6*FreeUserTokenMUnit, false))

	// 没有 in-flight 时应放行 (6M < 10M)
	d1, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.True(t, d1.Allowed, "6M used < 10M limit must allow")

	// 加入 5M in-flight 后 effective_used = 11M >= 10M，必须拒
	c.inFlightTokens.Add("", 5*FreeUserTokenMUnit)
	d2, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.False(t, d2.Allowed, "DB(6M)+in-flight(5M) >= 10M must reject")
	assert.Equal(t, int64(11*FreeUserTokenMUnit), d2.TokensUsed,
		"effective used should include in-flight")
}

// TestCheckFreeUserDailyTokenLimitWithInFlight_ModelBucket_UsesModelKey
// 验证模型独立桶时 in-flight 用 model name 作 key、与全局桶完全隔离。
// 关键词: in-flight 模型独立桶 key 隔离
func TestCheckFreeUserDailyTokenLimitWithInFlight_ModelBucket_UsesModelKey(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 404).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	// 全局 100M，但 memfit-light-free 独立桶 10M
	setRateLimitConfigForFreeTokenTest(t, 100,
		`{"memfit-light-free":{"limit_m":10,"exempt":false}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	c := NewServerConfig()
	defer c.Close()

	// 在全局桶加 50M in-flight 不应影响 memfit-light-free（独立桶）
	c.inFlightTokens.Add("", 50*FreeUserTokenMUnit)
	d, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed, "model bucket isolated from global in-flight")
	assert.True(t, d.ModelHasOwn)

	// 在 memfit-light-free 独立桶加 10M in-flight → 顶到 limit，必须拒
	c.inFlightTokens.Add("memfit-light-free", 10*FreeUserTokenMUnit)
	d2, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.False(t, d2.Allowed, "model in-flight pushed to limit must reject")

	// 没覆盖的模型仍走全局桶，看到全局 50M in-flight
	d3, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-standard-free")
	require.NoError(t, err)
	assert.True(t, d3.Allowed,
		"non-overridden model uses global bucket (50M < 100M limit)")
}

// TestCheckFreeUserDailyTokenLimitWithInFlight_ExemptModel_NeverBlocked
// 验证 exempt 模型即使 in-flight 已超限也永远放行（exempt 优先级最高）。
// 关键词: exempt 模型 in-flight 不参与判决
func TestCheckFreeUserDailyTokenLimitWithInFlight_ExemptModel_NeverBlocked(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 405).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	setRateLimitConfigForFreeTokenTest(t, 1,
		`{"foo-free":{"limit_m":0,"exempt":true}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	c := NewServerConfig()
	defer c.Close()

	// 即便 in-flight 加爆，exempt 也必须放行
	c.inFlightTokens.Add("", 1_000_000_000)
	d, err := c.checkFreeUserDailyTokenLimitWithInFlight("foo-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.True(t, d.Exempt)
}

// TestCheckFreeUserDailyTokenLimitWithInFlight_NilTracker_FallbackToDB
// 验证 c.inFlightTokens 为 nil 时（极端 case），包装版退化为原版行为，
// 不会 panic（防御性回归保护）。
// 关键词: nil tracker 回退原版, 防御性 NPE 守护
func TestCheckFreeUserDailyTokenLimitWithInFlight_NilTracker_FallbackToDB(t *testing.T) {
	require.NoError(t, EnsureFreeUserDailyTokenUsageTable())

	date := time.Now().AddDate(0, 0, 406).Format("2006-01-02")
	defer cleanupFreeUserTokenForDate(t, date)
	defer setFreeTokenNowDate(date)()

	setRateLimitConfigForFreeTokenTest(t, 1, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	c := NewServerConfig()
	defer c.Close()
	c.inFlightTokens = nil // 故意置空

	d, err := c.checkFreeUserDailyTokenLimitWithInFlight("memfit-light-free")
	require.NoError(t, err)
	assert.True(t, d.Allowed, "fresh bucket should allow even without tracker")
	assert.Equal(t, int64(0), d.TokensUsed)
}

// TestComputeInFlightTokenEstimate_PromptOnly 验证纯文本 prompt 的估算 >0、
// 并且与 ytoken 估算的 token 数同体系（间接确认走了 ComputeWeightedTokens）。
// 关键词: computeInFlightTokenEstimate 纯文本估算
func TestComputeInFlightTokenEstimate_PromptOnly(t *testing.T) {
	est := computeInFlightTokenEstimate("memfit-light-free", "hello world how are you", 0)
	assert.Greater(t, est, int64(0), "non-empty prompt must estimate >0 tokens")
}

// TestComputeInFlightTokenEstimate_ImageOnlyHasFloor 验证纯图片请求也有非零估算
// （每图 4096 token 预扣 + 8K completion budget）。
// 关键词: computeInFlightTokenEstimate 图片地板
func TestComputeInFlightTokenEstimate_ImageOnlyHasFloor(t *testing.T) {
	estText := computeInFlightTokenEstimate("memfit-light-free", "", 0)
	estWithImage := computeInFlightTokenEstimate("memfit-light-free", "", 2)
	assert.Greater(t, estWithImage, estText,
		"image count should add at least 2*4096 token estimate")
}

// TestResolveInFlightBucketKey_GlobalDefault 验证默认（无 override）映射到全局桶。
// 关键词: resolveInFlightBucketKey 默认全局桶
func TestResolveInFlightBucketKey_GlobalDefault(t *testing.T) {
	setRateLimitConfigForFreeTokenTest(t, 1200, "{}")
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	key, exempt := resolveInFlightBucketKey("memfit-standard-free")
	assert.Equal(t, "", key, "no override -> global bucket")
	assert.False(t, exempt)
}

// TestResolveInFlightBucketKey_ModelOverride 验证模型有 limit_m override → key=model name。
// 关键词: resolveInFlightBucketKey 模型覆盖
func TestResolveInFlightBucketKey_ModelOverride(t *testing.T) {
	setRateLimitConfigForFreeTokenTest(t, 100,
		`{"memfit-light-free":{"limit_m":5,"exempt":false}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	key, exempt := resolveInFlightBucketKey("memfit-light-free")
	assert.Equal(t, "memfit-light-free", key)
	assert.False(t, exempt)
}

// TestResolveInFlightBucketKey_ModelExempt 验证 exempt 模型返回 exempt=true（不预扣）。
// 关键词: resolveInFlightBucketKey 豁免
func TestResolveInFlightBucketKey_ModelExempt(t *testing.T) {
	setRateLimitConfigForFreeTokenTest(t, 100,
		`{"foo-free":{"limit_m":0,"exempt":true}}`)
	defer setRateLimitConfigForFreeTokenTest(t, 1200, "{}")

	_, exempt := resolveInFlightBucketKey("foo-free")
	assert.True(t, exempt, "exempt model should signal caller to skip in-flight Add")
}
