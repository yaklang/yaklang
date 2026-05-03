package aibalance

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// cacheStatsDB 返回一个跳过 GORM 软删除过滤的 *gorm.DB。
// ai_daily_cache_stats 是聚合表，没有「软删除-恢复」语义，
// 所有 query / update / delete 都直接对实际行操作，避免被 deleted_at IS NULL
// 过滤导致 UPSERT 时出现「First 找不到 + Create unique constraint 冲突」的状况。
// 关键词: cacheStatsDB, GORM Unscoped, 跳过软删除
func cacheStatsDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsureCacheStatsTable ensures the ai_daily_cache_stats table exists.
// 关键词: ai_daily_cache_stats migrate, EnsureCacheStatsTable
func EnsureCacheStatsTable() error {
	return GetDB().AutoMigrate(&schema.AiDailyCacheStat{}).Error
}

// hashAPIKeyForCache returns a stable 32-char fingerprint of an API key.
// Used as part of the unique index on ai_daily_cache_stats so that
// rotated/multiple API keys against the same provider get separate rows.
// 关键词: api_key_hash, sha1 prefix 32
func hashAPIKeyForCache(apiKey string) string {
	if apiKey == "" {
		return "noapikey-padding-padding-padding"
	}
	sum := sha1.Sum([]byte(apiKey))
	return hex.EncodeToString(sum[:])[:32]
}

// shrinkAPIKeyForCache returns a short prefix view of the API key for display.
// 关键词: api_key_shrink, 显示用前缀
func shrinkAPIKeyForCache(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	return utils.ShrinkString(apiKey, 8)
}

// RecordDailyCacheStats UPSERT-accumulates one request's usage into today's row
// for the (date, wrapper, model, providerType, providerDomain, apiKeyHash) tuple.
// 当 usage == nil 时记 1 次请求 + tokens=0。
// 关键词: RecordDailyCacheStats, UPSERT, gorm.Expr 累加, cached_tokens 持久化
func RecordDailyCacheStats(provider *Provider, wrapperName string, usage *aispec.ChatUsage) error {
	if provider == nil {
		return fmt.Errorf("RecordDailyCacheStats: provider is nil")
	}
	if wrapperName == "" {
		wrapperName = provider.ModelName
	}

	date := time.Now().Format("2006-01-02")
	apiKeyHash := hashAPIKeyForCache(provider.APIKey)
	apiKeyShrink := shrinkAPIKeyForCache(provider.APIKey)

	var prompt, completion, total, cached int64
	if usage != nil {
		prompt = int64(usage.PromptTokens)
		completion = int64(usage.CompletionTokens)
		total = int64(usage.TotalTokens)
		if usage.PromptTokensDetails != nil {
			cached = int64(usage.PromptTokensDetails.CachedTokens)
		}
	}

	db := cacheStatsDB()

	// 先尝试找到当天已有行；找到就用 UpdateColumn + gorm.Expr 累加，
	// 没找到就 Create 一行新的（GORM v1 没有原生 OnConflict 语法，
	// 这里用 SELECT-then-UPDATE/INSERT 两步实现 UPSERT 语义）。
	var row schema.AiDailyCacheStat
	err := db.Where("date = ? AND wrapper_name = ? AND model_name = ? AND provider_type_name = ? AND provider_domain = ? AND api_key_hash = ?",
		date, wrapperName, provider.ModelName, provider.TypeName, provider.DomainOrURL, apiKeyHash).
		First(&row).Error

	if err == nil {
		return db.Model(&schema.AiDailyCacheStat{}).
			Where("id = ?", row.ID).
			UpdateColumns(map[string]interface{}{
				"request_count":     gorm.Expr("request_count + ?", 1),
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", prompt),
				"completion_tokens": gorm.Expr("completion_tokens + ?", completion),
				"total_tokens":      gorm.Expr("total_tokens + ?", total),
				"cached_tokens":     gorm.Expr("cached_tokens + ?", cached),
				"api_key_shrink":    apiKeyShrink,
			}).Error
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("RecordDailyCacheStats query failed: %v", err)
	}

	row = schema.AiDailyCacheStat{
		Date:             date,
		WrapperName:      wrapperName,
		ModelName:        provider.ModelName,
		ProviderTypeName: provider.TypeName,
		ProviderDomain:   provider.DomainOrURL,
		APIKeyHash:       apiKeyHash,
		APIKeyShrink:     apiKeyShrink,
		RequestCount:     1,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
		CachedTokens:     cached,
	}
	if createErr := db.Create(&row).Error; createErr != nil {
		// 并发场景：另一个 goroutine 已经先 Create 了同样唯一键的行，
		// 此时退化为 UPDATE 累加一次即可。
		var existing schema.AiDailyCacheStat
		if findErr := db.Where("date = ? AND wrapper_name = ? AND model_name = ? AND provider_type_name = ? AND provider_domain = ? AND api_key_hash = ?",
			date, wrapperName, provider.ModelName, provider.TypeName, provider.DomainOrURL, apiKeyHash).
			First(&existing).Error; findErr == nil {
			return db.Model(&schema.AiDailyCacheStat{}).
				Where("id = ?", existing.ID).
				UpdateColumns(map[string]interface{}{
					"request_count":     gorm.Expr("request_count + ?", 1),
					"prompt_tokens":     gorm.Expr("prompt_tokens + ?", prompt),
					"completion_tokens": gorm.Expr("completion_tokens + ?", completion),
					"total_tokens":      gorm.Expr("total_tokens + ?", total),
					"cached_tokens":     gorm.Expr("cached_tokens + ?", cached),
					"api_key_shrink":    apiKeyShrink,
				}).Error
		}
		return fmt.Errorf("RecordDailyCacheStats create failed: %v", createErr)
	}
	return nil
}

// CacheTotalRow 是「今日累计缓存命中」聚合结果。
// 关键词: QueryTodayCacheStatsTotal, 今日缓存聚合
type CacheTotalRow struct {
	RequestCount     int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
	CachedTokens     int64
}

// QueryTodayCacheStatsTotal returns aggregate totals for today's date.
// 关键词: QueryTodayCacheStatsTotal, sum cached_tokens
func QueryTodayCacheStatsTotal() (*CacheTotalRow, error) {
	return QueryCacheStatsTotalByDate(time.Now().Format("2006-01-02"))
}

// QueryCacheStatsTotalByDate aggregates cache stats for a single date (test friendly).
// 关键词: QueryCacheStatsTotalByDate, 测试友好
func QueryCacheStatsTotalByDate(date string) (*CacheTotalRow, error) {
	row := &CacheTotalRow{}
	err := cacheStatsDB().Table((&schema.AiDailyCacheStat{}).TableName()).
		Where("date = ?", date).
		Select("COALESCE(SUM(request_count),0) AS request_count, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, COALESCE(SUM(cached_tokens),0) AS cached_tokens").
		Row().Scan(&row.RequestCount, &row.PromptTokens, &row.CompletionTokens, &row.TotalTokens, &row.CachedTokens)
	if err != nil {
		return nil, fmt.Errorf("QueryCacheStatsTotalByDate failed: %v", err)
	}
	return row, nil
}

// QueryTodayCacheBreakdown returns today's per-(wrapper,model,provider,key) rows
// sorted by request_count desc.
// 关键词: QueryTodayCacheBreakdown, 今日明细表
func QueryTodayCacheBreakdown() ([]*schema.AiDailyCacheStat, error) {
	return QueryCacheBreakdownByDate(time.Now().Format("2006-01-02"))
}

// QueryCacheBreakdownByDate is the test-friendly variant of QueryTodayCacheBreakdown.
// 关键词: QueryCacheBreakdownByDate
func QueryCacheBreakdownByDate(date string) ([]*schema.AiDailyCacheStat, error) {
	var rows []*schema.AiDailyCacheStat
	if err := cacheStatsDB().Where("date = ?", date).
		Order("request_count DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("QueryCacheBreakdownByDate failed: %v", err)
	}
	return rows, nil
}

// CacheTrendDay 是 60 天缓存趋势的单日聚合点。
// 关键词: CacheTrendDay, 缓存命中趋势
type CacheTrendDay struct {
	Date             string `json:"date"`
	RequestCount     int64  `json:"request_count"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	HitRatio         float64 `json:"hit_ratio"`
}

// QueryCacheTrend60Days returns last 60 calendar days (today inclusive),
// missing dates filled with zero rows. HitRatio = cached / prompt (0 when prompt=0).
// 关键词: QueryCacheTrend60Days, 缺失日期补 0, 缓存命中比例
func QueryCacheTrend60Days() ([]*CacheTrendDay, error) {
	return QueryCacheTrendDays(60, time.Now())
}

// QueryCacheTrendDays is the test-friendly variant of QueryCacheTrend60Days.
// 关键词: QueryCacheTrendDays, 测试入口
func QueryCacheTrendDays(days int, end time.Time) ([]*CacheTrendDay, error) {
	if days <= 0 {
		days = 60
	}

	startDate := end.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	type aggRow struct {
		Date             string
		RequestCount     int64
		PromptTokens     int64
		CompletionTokens int64
		TotalTokens      int64
		CachedTokens     int64
	}
	rows := []aggRow{}
	if err := cacheStatsDB().Table((&schema.AiDailyCacheStat{}).TableName()).
		Select("date, COALESCE(SUM(request_count),0) AS request_count, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, COALESCE(SUM(cached_tokens),0) AS cached_tokens").
		Where("date >= ?", startDate).
		Group("date").
		Order("date ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("QueryCacheTrendDays failed: %v", err)
	}

	idx := make(map[string]aggRow, len(rows))
	for _, r := range rows {
		idx[r.Date] = r
	}

	out := make([]*CacheTrendDay, 0, days)
	for i := 0; i < days; i++ {
		d := end.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		row := idx[d]
		hit := 0.0
		if row.PromptTokens > 0 {
			hit = float64(row.CachedTokens) / float64(row.PromptTokens)
		}
		out = append(out, &CacheTrendDay{
			Date:             d,
			RequestCount:     row.RequestCount,
			PromptTokens:     row.PromptTokens,
			CompletionTokens: row.CompletionTokens,
			TotalTokens:      row.TotalTokens,
			CachedTokens:     row.CachedTokens,
			HitRatio:         hit,
		})
	}
	return out, nil
}

// CleanupOldCacheStats deletes ai_daily_cache_stats rows whose date < (today - keepDays).
// 用 Unscoped() 硬删除，避免 GORM 软删除残留行触发 unique 约束冲突。
// 关键词: CleanupOldCacheStats, 100 天保留窗, Unscoped 硬删除
func CleanupOldCacheStats(keepDays int) (int64, error) {
	if keepDays <= 0 {
		keepDays = 100
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	tx := GetDB().Unscoped().Where("date < ?", cutoff).Delete(&schema.AiDailyCacheStat{})
	if tx.Error != nil {
		return 0, fmt.Errorf("CleanupOldCacheStats failed: %v", tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Infof("CleanupOldCacheStats removed %d rows older than %s", tx.RowsAffected, cutoff)
	}
	return tx.RowsAffected, nil
}
