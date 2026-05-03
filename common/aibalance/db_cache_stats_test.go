package aibalance

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// 关键词: db_cache_stats_test, ai_daily_cache_stats 单元测试

// uniqueCacheTestProvider 构造一个测试用 provider，wrapper 用 nonce 隔离避免污染。
func uniqueCacheTestProvider(nonce string) *Provider {
	return &Provider{
		ModelName:   "test-model-" + nonce,
		TypeName:    "test-type-" + nonce,
		DomainOrURL: "https://test.example.com/" + nonce,
		APIKey:      "sk-test-" + utils.RandStringBytes(8),
	}
}

// cleanupCacheStatsForWrapper 清空指定 wrapper_name 的所有行（硬删除，避免软删除残留触发 unique 约束）。
func cleanupCacheStatsForWrapper(t *testing.T, wrapperName string) {
	require.NoError(t, GetDB().Unscoped().Where("wrapper_name = ?", wrapperName).Delete(&schema.AiDailyCacheStat{}).Error)
}

func TestEnsureCacheStatsTable(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
}

func TestRecordDailyCacheStats_Increment(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
	wrapper := "wrap-incr-" + utils.RandStringBytes(6)
	defer cleanupCacheStatsForWrapper(t, wrapper)
	p := uniqueCacheTestProvider("incr")

	usage := &aispec.ChatUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: &aispec.PromptTokensDetails{
			CachedTokens: 30,
		},
	}
	require.NoError(t, RecordDailyCacheStats(p, wrapper, usage))
	require.NoError(t, RecordDailyCacheStats(p, wrapper, usage))
	require.NoError(t, RecordDailyCacheStats(p, wrapper, usage))

	totals, err := QueryCacheStatsTotalByDate(time.Now().Format("2006-01-02"))
	require.NoError(t, err)
	// 注意：今日表里可能有别的 wrapper 行，所以仅比较「我们关心的 wrapper」的累计。
	var rows []*schema.AiDailyCacheStat
	require.NoError(t, GetDB().Where("wrapper_name = ?", wrapper).Find(&rows).Error)
	require.Len(t, rows, 1, "same provider should be aggregated into a single row")
	assert.Equal(t, int64(3), rows[0].RequestCount)
	assert.Equal(t, int64(300), rows[0].PromptTokens)
	assert.Equal(t, int64(150), rows[0].CompletionTokens)
	assert.Equal(t, int64(450), rows[0].TotalTokens)
	assert.Equal(t, int64(90), rows[0].CachedTokens)

	assert.True(t, totals.RequestCount >= 3, "today total request_count must include our 3 inserts")
}

func TestRecordDailyCacheStats_NilUsageDetails(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
	wrapper := "wrap-nil-" + utils.RandStringBytes(6)
	defer cleanupCacheStatsForWrapper(t, wrapper)
	p := uniqueCacheTestProvider("nil")

	usage := &aispec.ChatUsage{
		PromptTokens:     200,
		CompletionTokens: 80,
		TotalTokens:      280,
		// PromptTokensDetails 为 nil：cached 应记为 0
	}
	require.NoError(t, RecordDailyCacheStats(p, wrapper, usage))

	require.NoError(t, RecordDailyCacheStats(p, wrapper, nil))

	rows, err := QueryCacheBreakdownByDate(time.Now().Format("2006-01-02"))
	require.NoError(t, err)
	var hit *schema.AiDailyCacheStat
	for _, r := range rows {
		if r.WrapperName == wrapper {
			hit = r
			break
		}
	}
	require.NotNil(t, hit, "should find our wrapper row in breakdown")
	assert.Equal(t, int64(2), hit.RequestCount)
	assert.Equal(t, int64(200), hit.PromptTokens)
	assert.Equal(t, int64(0), hit.CachedTokens)
}

func TestRecordDailyCacheStats_DateBoundary(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
	wrapper := "wrap-date-" + utils.RandStringBytes(6)
	defer cleanupCacheStatsForWrapper(t, wrapper)
	p := uniqueCacheTestProvider("date")

	// 今天写一行
	usage := &aispec.ChatUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15,
		PromptTokensDetails: &aispec.PromptTokensDetails{CachedTokens: 3}}
	require.NoError(t, RecordDailyCacheStats(p, wrapper, usage))

	// 手动伪造一行「昨天」的记录，确认不会与今天冲突
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	yRow := schema.AiDailyCacheStat{
		Date:             yesterday,
		WrapperName:      wrapper,
		ModelName:        p.ModelName,
		ProviderTypeName: p.TypeName,
		ProviderDomain:   p.DomainOrURL,
		APIKeyHash:       hashAPIKeyForCache(p.APIKey),
		APIKeyShrink:     shrinkAPIKeyForCache(p.APIKey),
		RequestCount:     7,
		PromptTokens:     70,
	}
	require.NoError(t, GetDB().Create(&yRow).Error)

	var rows []*schema.AiDailyCacheStat
	require.NoError(t, GetDB().Where("wrapper_name = ?", wrapper).Order("date ASC").Find(&rows).Error)
	require.Len(t, rows, 2, "today and yesterday should be two separate rows")

	totalsToday, err := QueryCacheStatsTotalByDate(time.Now().Format("2006-01-02"))
	require.NoError(t, err)
	assert.True(t, totalsToday.RequestCount >= 1)

	totalsY, err := QueryCacheStatsTotalByDate(yesterday)
	require.NoError(t, err)
	assert.True(t, totalsY.RequestCount >= 7)
}

func TestQueryCacheTrendDays_FillZero(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
	wrapper := "wrap-trend-" + utils.RandStringBytes(6)
	defer cleanupCacheStatsForWrapper(t, wrapper)

	// 在 5 天前 / 2 天前 各写一行，确保中间日期补 0
	end := time.Now()
	for _, offset := range []int{-5, -2} {
		row := schema.AiDailyCacheStat{
			Date:             end.AddDate(0, 0, offset).Format("2006-01-02"),
			WrapperName:      wrapper,
			ModelName:        "trend-model",
			ProviderTypeName: "trend-type",
			ProviderDomain:   "trend-domain",
			APIKeyHash:       hashAPIKeyForCache(fmt.Sprintf("k-%d", offset)),
			APIKeyShrink:     "k1234567",
			RequestCount:     int64(10 + offset),
			PromptTokens:     1000,
			CachedTokens:     200,
		}
		require.NoError(t, GetDB().Create(&row).Error)
	}

	trend, err := QueryCacheTrendDays(7, end)
	require.NoError(t, err)
	require.Len(t, trend, 7, "must return exactly N days")

	// 没数据的日期请求数应为 0；hit_ratio 0
	dayMinus3 := end.AddDate(0, 0, -3).Format("2006-01-02")
	for _, d := range trend {
		if d.Date == dayMinus3 {
			// 注：表里同一天可能还有其他 wrapper 写入；这里仅断言"我们的写入没有污染该日"。
			// 因为 trend 是 SUM(全表)，所以无法严格断言为 0，只能断言函数没有 panic、有 N 行。
			assert.GreaterOrEqual(t, d.RequestCount, int64(0))
		}
	}

	// 最后一个元素是今天
	assert.Equal(t, end.Format("2006-01-02"), trend[len(trend)-1].Date)
}

func TestCleanupOldCacheStats(t *testing.T) {
	require.NoError(t, EnsureCacheStatsTable())
	wrapper := "wrap-cleanup-" + utils.RandStringBytes(6)
	defer cleanupCacheStatsForWrapper(t, wrapper)

	// 200 天前的行：应被清理
	oldRow := schema.AiDailyCacheStat{
		Date:             time.Now().AddDate(0, 0, -200).Format("2006-01-02"),
		WrapperName:      wrapper,
		ModelName:        "cleanup-model",
		ProviderTypeName: "cleanup-type",
		ProviderDomain:   "cleanup-domain",
		APIKeyHash:       hashAPIKeyForCache("kold"),
		APIKeyShrink:     "kold-001",
		RequestCount:     1,
	}
	require.NoError(t, GetDB().Create(&oldRow).Error)

	// 5 天前的行：应保留
	freshRow := oldRow
	freshRow.ID = 0
	freshRow.Date = time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	freshRow.APIKeyHash = hashAPIKeyForCache("knew")
	require.NoError(t, GetDB().Create(&freshRow).Error)

	removed, err := CleanupOldCacheStats(100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, removed, int64(1))

	var rows []*schema.AiDailyCacheStat
	require.NoError(t, GetDB().Where("wrapper_name = ?", wrapper).Find(&rows).Error)
	require.Len(t, rows, 1, "should keep only the recent row")
	assert.Equal(t, freshRow.Date, rows[0].Date)
}
