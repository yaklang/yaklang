package aibalance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// TestComputeModelWeightedTokens_IsFreeExemption 验证「实际模型标记为免费(IsFree)」后，
// 无论四维倍率设置得多高，ComputeModelWeightedTokens 一律返回 0（计费豁免）；
// 取消免费后同样的倍率与 usage 应当产生 > 0 的加权计费 token。
// 关键词: IsFree 计费豁免测试, ComputeModelWeightedTokens 返回 0
func TestComputeModelWeightedTokens_IsFreeExemption(t *testing.T) {
	require.NoError(t, EnsureModelMultiplierTable())
	require.NoError(t, EnsureModelMultiplierConfigTable())

	modelName := "test-isfree-model-" + time.Now().Format("150405.000000")
	defer GetDB().Unscoped().Where("internal_model_name = ?", modelName).Delete(&AiModelMultiplier{})

	// 故意设置较高的四维倍率，并打开 IsFree（isFree=1）。
	require.NoError(t, SaveModelMultiplierWithFree(modelName, 10, 10, 10, 10, 1))

	usage := &aispec.ChatUsage{
		PromptTokens:     1000,
		CompletionTokens: 1000,
	}

	weighted := ComputeModelWeightedTokens(modelName, usage)
	assert.Equal(t, int64(0), weighted, "IsFree model must always be billed as 0 weighted tokens")

	// 取消免费（isFree=0），倍率保持不变，现在应当真正扣费。
	require.NoError(t, SaveModelMultiplierWithFree(modelName, 10, 10, 10, 10, 0))
	weighted2 := ComputeModelWeightedTokens(modelName, usage)
	assert.Greater(t, weighted2, int64(0), "non-free model with positive multipliers must be billed > 0")
}

// TestPaidUserDailyTokenLimit_HardGate 验证付费用户全局日 Token 总额度（第二道硬门）：
//   - 限额内放行；
//   - 累计用量达到/超过限额后一律拒绝（触发 429 余额不足）；
//   - 限额为 0 时不限制。
// 关键词: 付费全局额度 429 测试, CheckPaidUserDailyTokenLimit
func TestPaidUserDailyTokenLimit_HardGate(t *testing.T) {
	require.NoError(t, EnsureRateLimitConfigTable())
	require.NoError(t, EnsurePaidUserDailyTokenUsageTable())

	date := freeTokenNowDate()
	require.NotEmpty(t, date)

	// 清理当天付费桶，保证测试隔离；测试结束后再清理一次。
	GetDB().Unscoped().Where("date = ?", date).Delete(&PaidUserDailyTokenUsage{})
	defer GetDB().Unscoped().Where("date = ?", date).Delete(&PaidUserDailyTokenUsage{})

	// 保存原始配置并在结束后恢复，避免污染其他用例。
	original, err := GetRateLimitConfig()
	require.NoError(t, err)
	originalPaid := original.PaidUserTokenLimitM
	defer func() {
		cfg, e := GetRateLimitConfig()
		if e == nil {
			cfg.PaidUserTokenLimitM = originalPaid
			_ = SaveRateLimitConfig(cfg)
		}
	}()

	// 设置付费全局日总额度为 1M token。
	cfg, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg.PaidUserTokenLimitM = 1
	require.NoError(t, SaveRateLimitConfig(cfg))

	limitRaw := int64(1) * FreeUserTokenMUnit

	// 初始用量为 0，应放行。
	dec1, err := CheckPaidUserDailyTokenLimit()
	require.NoError(t, err)
	assert.True(t, dec1.Allowed, "empty bucket should be allowed")
	assert.Equal(t, limitRaw, dec1.TokensLimit)

	// 累加到刚好等于限额：used >= limit 应拒绝（硬门）。
	require.NoError(t, AddPaidUserDailyTokenUsage(limitRaw))
	dec2, err := CheckPaidUserDailyTokenLimit()
	require.NoError(t, err)
	assert.False(t, dec2.Allowed, "used == limit must be rejected (hard gate)")
	assert.Equal(t, limitRaw, dec2.TokensUsed)

	// IsFree/weighted<=0 的累加应被忽略，不改变用量。
	require.NoError(t, AddPaidUserDailyTokenUsage(0))
	used, err := GetPaidUserDailyTokenUsage(date)
	require.NoError(t, err)
	assert.Equal(t, limitRaw, used, "weighted<=0 must not change paid bucket")

	// 限额为 0 表示不限制：即便桶里已经超量也应放行。
	cfg2, err := GetRateLimitConfig()
	require.NoError(t, err)
	cfg2.PaidUserTokenLimitM = 0
	require.NoError(t, SaveRateLimitConfig(cfg2))

	dec3, err := CheckPaidUserDailyTokenLimit()
	require.NoError(t, err)
	assert.True(t, dec3.Allowed, "PaidUserTokenLimitM=0 means unlimited")
}

// TestAiApiKeyMetaPersistenceAndFilter 验证 API Key 三个新字段（Username/Remark/MetaInfo）：
//   - 创建时可携带并持久化；
//   - 列表接口可按 username 精确过滤（用户名可重复 -> 一个用户名命中多条）；
//   - UpdateAiApiKeyMeta 可更新三字段。
// 关键词: API Key 三字段持久化与过滤测试, GetAiApiKeysPaginatedFiltered, UpdateAiApiKeyMeta
func TestAiApiKeyMetaPersistenceAndFilter(t *testing.T) {
	require.NoError(t, GetDB().AutoMigrate(&AiApiKeys{}).Error)

	suffix := time.Now().Format("150405.000000")
	uname := "alice-" + suffix
	other := "bob-" + suffix

	key1 := "mf-meta1-" + suffix
	key2 := "mf-meta2-" + suffix
	key3 := "mf-meta3-" + suffix
	for _, k := range []string{key1, key2, key3} {
		kk := k
		defer GetDB().Unscoped().Where("api_key = ?", kk).Delete(&AiApiKeys{})
	}

	// 同一用户名 alice 关联两条 key（验证用户名可重复）。
	require.NoError(t, SaveAiApiKeyRecord(&AiApiKeys{
		APIKey: key1, AllowedModels: "m1", Active: true,
		Username: uname, Remark: "first key", MetaInfo: `{"oauth":"github"}`,
	}))
	require.NoError(t, SaveAiApiKeyRecord(&AiApiKeys{
		APIKey: key2, AllowedModels: "m1", Active: true,
		Username: uname, Remark: "second key",
	}))
	// 另一个用户名 bob 关联一条 key。
	require.NoError(t, SaveAiApiKeyRecord(&AiApiKeys{
		APIKey: key3, AllowedModels: "m1", Active: true,
		Username: other,
	}))

	// 持久化校验：metainfo 等字段确实落库。
	var got1 AiApiKeys
	require.NoError(t, GetDB().Where("api_key = ?", key1).First(&got1).Error)
	assert.Equal(t, uname, got1.Username)
	assert.Equal(t, "first key", got1.Remark)
	assert.Equal(t, `{"oauth":"github"}`, got1.MetaInfo)

	// 按 username 过滤：alice 命中 2 条，bob 命中 1 条。
	aliceKeys, aliceTotal, err := GetAiApiKeysPaginatedFiltered(1, 100, "created_at", "desc", uname)
	require.NoError(t, err)
	assert.Equal(t, int64(2), aliceTotal, "alice should have 2 keys")
	assert.Len(t, aliceKeys, 2)
	for _, k := range aliceKeys {
		assert.Equal(t, uname, k.Username)
	}

	bobKeys, bobTotal, err := GetAiApiKeysPaginatedFiltered(1, 100, "created_at", "desc", other)
	require.NoError(t, err)
	assert.Equal(t, int64(1), bobTotal, "bob should have 1 key")
	require.Len(t, bobKeys, 1)
	assert.Equal(t, other, bobKeys[0].Username)

	// 更新 meta：把 key3 改绑到 alice，并设置备注与 metainfo。
	require.NoError(t, UpdateAiApiKeyMeta(got1ID(t, key3), uname, "rebound to alice", `{"sub":"123"}`))

	var got3 AiApiKeys
	require.NoError(t, GetDB().Where("api_key = ?", key3).First(&got3).Error)
	assert.Equal(t, uname, got3.Username)
	assert.Equal(t, "rebound to alice", got3.Remark)
	assert.Equal(t, `{"sub":"123"}`, got3.MetaInfo)

	// 现在 alice 应命中 3 条。
	_, aliceTotal2, err := GetAiApiKeysPaginatedFiltered(1, 100, "created_at", "desc", uname)
	require.NoError(t, err)
	assert.Equal(t, int64(3), aliceTotal2, "alice should now have 3 keys after rebind")

	// 更新不存在的 id 应报错。
	err = UpdateAiApiKeyMeta(0, "x", "y", "z")
	assert.Error(t, err, "updating non-existent api key id should error")
}

// got1ID 是测试辅助：按 api_key 取出其自增 ID。
func got1ID(t *testing.T, apiKey string) uint {
	t.Helper()
	var k AiApiKeys
	require.NoError(t, GetDB().Where("api_key = ?", apiKey).First(&k).Error)
	require.NotZero(t, k.ID, fmt.Sprintf("api key %s should have a non-zero id", apiKey))
	return k.ID
}
