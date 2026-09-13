package aimem

import (
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

// Keep the same arithmetic order as CalcMemoryValue.
const memoryValueSQL = "(COALESCE(r_score,0)*0.25 + COALESCE(a_score,0)*0.20 + COALESCE(p_score,0)*0.15 + COALESCE(c_score,0)*0.15 + COALESCE(o_score,0)*0.10 + COALESCE(e_score,0)*0.05 + COALESCE(t_score,0)*0.10)"

func cleanupBatchSize(size int) int {
	if size <= 0 {
		return 100
	}
	// Bound SQLite variables and allocations even for a malformed config.
	if size > 1000 {
		return 1000
	}
	return size
}

// CleanupConfig 清理策略配置
type CleanupConfig struct {
	// TTL 策略
	EnableTTLExpiry  bool
	TTLTransientDays float64 // T_Score 0.0-0.3: 瞬时记忆过期天数，默认 7
	TTLShortTermDays float64 // T_Score 0.3-0.6: 短期记忆过期天数，默认 30
	TTLMidTermDays   float64 // T_Score 0.6-0.8: 中期记忆过期天数，默认 90

	// 低价值淘汰策略
	EnableLowValueEviction bool
	MinValueThreshold      float64 // 综合评分低于此值才考虑淘汰，默认 0.35
	ColdMemoryDays         int     // 记忆存在超过此天数且评分低才考虑淘汰，默认 14

	// 数量上限策略
	EnableMaxMemoryCount bool
	MaxMemoryCount       int // 记忆总数上限，默认 200。超过时按综合评分从低到高淘汰
	OverEvictMargin      int // 超量淘汰安全余量，默认 20。实际删除 (超出量 + margin) 条，避免刚删完又触发

	// 批量限制
	MaxBatchSize int // 每次扫描最多返回多少条，默认 100
}

// DefaultCleanupConfig 默认清理配置
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		EnableTTLExpiry:        true,
		TTLTransientDays:       TTLTransientDays,
		TTLShortTermDays:       TTLShortTermDays,
		TTLMidTermDays:         TTLMidTermDays,
		EnableLowValueEviction: true,
		MinValueThreshold:      0.35,
		ColdMemoryDays:         14,
		EnableMaxMemoryCount:   true,
		MaxMemoryCount:         200,
		OverEvictMargin:        20,
		MaxBatchSize:           100,
	}
}

// CalcMemoryValue 计算记忆的综合价值评分 (0.0-1.0)
// 权重: R=0.25, A=0.20, P=0.15, C=0.15, O=0.10, E=0.05, T=0.10
func CalcMemoryValue(entity *aicommon.MemoryEntity) float64 {
	return entity.R_Score*0.25 +
		entity.A_Score*0.20 +
		entity.P_Score*0.15 +
		entity.C_Score*0.15 +
		entity.O_Score*0.10 +
		entity.E_Score*0.05 +
		entity.T_Score*0.10
}

// ScanExpiredMemories 扫描已过期的记忆 ID（只 SELECT，不删除）
// 查询条件: expires_at IS NOT NULL AND expires_at < NOW()
func ScanExpiredMemories(db *gorm.DB, tableName, sessionID string, maxBatch int) ([]string, error) {
	if db == nil || tableName == "" || sessionID == "" {
		return nil, nil
	}
	maxBatch = cleanupBatchSize(maxBatch)

	var entities []schema.AIMemoryEntity
	err := db.Table(tableName).
		Select("memory_id").
		Where("session_id = ? AND deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at < ?", sessionID, time.Now()).
		Limit(maxBatch).
		Find(&entities).Error
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.MemoryID)
	}
	return ids, nil
}

// ScanLowValueMemories 扫描低价值记忆 ID（只 SELECT，不删除）
//
// 淘汰条件（同时满足）:
//   - CreatedAt 距今 > ColdMemoryDays（存在足够久）
//   - T_Score < 0.8（长期记忆豁免）
//   - 综合评分 < MinValueThreshold
func ScanLowValueMemories(db *gorm.DB, tableName, sessionID string, config CleanupConfig) ([]string, error) {
	if db == nil || tableName == "" || sessionID == "" {
		return nil, nil
	}
	if !config.EnableLowValueEviction {
		return nil, nil
	}

	maxBatch := config.MaxBatchSize
	maxBatch = cleanupBatchSize(maxBatch)

	coldThreshold := time.Now().AddDate(0, 0, -config.ColdMemoryDays)

	// Filter and rank in SQLite. Never materialize content, tags or embeddings
	// to select a small cleanup batch. COALESCE matches Go's zero-value scores
	// for legacy rows containing NULL.
	var candidates []schema.AIMemoryEntity
	err := db.Table(tableName).
		Select("memory_id").
		Where("session_id = ? AND deleted_at IS NULL AND created_at < ? AND COALESCE(t_score,0) < 0.8", sessionID, coldThreshold).
		Where(memoryValueSQL+" < ?", config.MinValueThreshold).
		Order(memoryValueSQL + ", id").
		Limit(maxBatch).Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(candidates))
	for _, e := range candidates {
		ids = append(ids, e.MemoryID)
	}
	return ids, nil
}

// ScanOverCountMemories 扫描因数量超限需要淘汰的记忆 ID（只 SELECT，不删除）
//
// 当 session 内记忆总数超过 MaxMemoryCount 时，按综合评分从低到高选出需要淘汰的记忆。
// 实际淘汰数量 = (totalCount - MaxMemoryCount) + OverEvictMargin，确保清理后不会立即再次触发。
//
// 注意：所有记忆（包括 T_Score >= 0.8 的长期记忆）都参与排序。
// 综合评分 CalcMemoryValue 中 T_Score 占 0.10 权重，高 T 记忆天然排在最后被淘汰。
// 这确保了即使低 T 记忆全部清理完仍然超限时，高 T 记忆中价值最低的也会被淘汰，
// 不会出现僵尸记忆。
func ScanOverCountMemories(db *gorm.DB, tableName, sessionID string, config CleanupConfig) ([]string, error) {
	if db == nil || tableName == "" || sessionID == "" {
		return nil, nil
	}
	if !config.EnableMaxMemoryCount || config.MaxMemoryCount <= 0 {
		return nil, nil
	}

	// 统计当前 session 内的记忆总数
	var totalCount int64
	if err := db.Table(tableName).
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
		Count(&totalCount).Error; err != nil {
		return nil, err
	}

	// 未超限，无需淘汰
	if int(totalCount) <= config.MaxMemoryCount {
		return nil, nil
	}

	// 需要淘汰的数量 = 超出量 + 安全余量
	maxBatch := cleanupBatchSize(config.MaxBatchSize)
	excess := totalCount - int64(config.MaxMemoryCount)
	if excess > int64(maxBatch) {
		excess = int64(maxBatch)
	}
	toEvict := int(excess)
	toEvict += min(max(config.OverEvictMargin, 0), maxBatch-toEvict)
	var candidates []schema.AIMemoryEntity
	err := db.Table(tableName).
		Select("memory_id").
		Where("session_id = ? AND deleted_at IS NULL", sessionID).
		Order(memoryValueSQL + ", id").Limit(toEvict).
		Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(candidates))
	for _, e := range candidates {
		ids = append(ids, e.MemoryID)
	}
	return ids, nil
}

// ScanAllCleanupMemories 一次性扫描过期 + 低价值 + 超量记忆 ID，合并去重后返回
func ScanAllCleanupMemories(db *gorm.DB, tableName, sessionID string, config CleanupConfig) ([]string, error) {
	var allIDs []string

	if config.EnableTTLExpiry {
		expiredIDs, err := ScanExpiredMemories(db, tableName, sessionID, config.MaxBatchSize)
		if err != nil {
			return nil, err
		}
		allIDs = append(allIDs, expiredIDs...)
	}

	if config.EnableLowValueEviction {
		lowValueIDs, err := ScanLowValueMemories(db, tableName, sessionID, config)
		if err != nil {
			return nil, err
		}
		allIDs = append(allIDs, lowValueIDs...)
	}

	if config.EnableMaxMemoryCount {
		overCountIDs, err := ScanOverCountMemories(db, tableName, sessionID, config)
		if err != nil {
			return nil, err
		}
		allIDs = append(allIDs, overCountIDs...)
	}

	// 去重
	allIDs = uniqueNonEmptyStrings(allIDs)
	if limit := cleanupBatchSize(config.MaxBatchSize); len(allIDs) > limit {
		allIDs = allIDs[:limit]
	}
	return allIDs, nil
}
