package aimem

import (
	"sort"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

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
	if maxBatch <= 0 {
		maxBatch = 100
	}

	var entities []schema.AIMemoryEntity
	err := db.Table(tableName).
		Select("memory_id").
		Where("session_id = ? AND expires_at IS NOT NULL AND expires_at < ?", sessionID, time.Now()).
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
	if maxBatch <= 0 {
		maxBatch = 100
	}

	coldThreshold := time.Now().AddDate(0, 0, -config.ColdMemoryDays)

	// 按 CreatedAt 和 T_Score 从 DB 筛选候选集
	var candidates []schema.AIMemoryEntity
	err := db.Table(tableName).
		Where("session_id = ? AND created_at < ? AND t_score < 0.8",
			sessionID, coldThreshold).
		Limit(maxBatch * 2). // 多取一些，后面按综合评分再过滤
		Find(&candidates).Error
	if err != nil {
		return nil, err
	}

	// 在内存中计算综合评分，低于阈值的才入选
	ids := make([]string, 0, len(candidates))
	for _, e := range candidates {
		entity := &aicommon.MemoryEntity{
			C_Score: e.C_Score,
			O_Score: e.O_Score,
			R_Score: e.R_Score,
			E_Score: e.E_Score,
			P_Score: e.P_Score,
			A_Score: e.A_Score,
			T_Score: e.T_Score,
		}
		value := CalcMemoryValue(entity)
		if value < config.MinValueThreshold {
			ids = append(ids, e.MemoryID)
		}
	}

	// 限制最终数量
	if len(ids) > maxBatch {
		ids = ids[:maxBatch]
	}

	return ids, nil
}

// ScanOverCountMemories 扫描因数量超限需要淘汰的记忆 ID（只 SELECT，不删除）
//
// 当 session 内记忆总数超过 MaxMemoryCount 时，按综合评分从低到高选出需要淘汰的记忆。
// 实际淘汰数量 = (totalCount - MaxMemoryCount) + OverEvictMargin，确保清理后不会立即再次触发。
// 长期记忆 (T_Score >= 0.8) 豁免，不参与淘汰。
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
		Where("session_id = ?", sessionID).
		Count(&totalCount).Error; err != nil {
		return nil, err
	}

	// 未超限，无需淘汰
	if int(totalCount) <= config.MaxMemoryCount {
		return nil, nil
	}

	// 需要淘汰的数量 = 超出量 + 安全余量
	toEvict := int(totalCount) - config.MaxMemoryCount + config.OverEvictMargin
	if toEvict <= 0 {
		return nil, nil
	}

	// 查询所有非豁免记忆 (T_Score < 0.8)，按综合评分从低到高排序
	var candidates []schema.AIMemoryEntity
	err := db.Table(tableName).
		Where("session_id = ? AND t_score < 0.8", sessionID).
		Find(&candidates).Error
	if err != nil {
		return nil, err
	}

	// 计算综合评分并排序
	type scoredEntity struct {
		id    string
		value float64
	}
	scored := make([]scoredEntity, 0, len(candidates))
	for _, e := range candidates {
		entity := &aicommon.MemoryEntity{
			C_Score: e.C_Score,
			O_Score: e.O_Score,
			R_Score: e.R_Score,
			E_Score: e.E_Score,
			P_Score: e.P_Score,
			A_Score: e.A_Score,
			T_Score: e.T_Score,
		}
		scored = append(scored, scoredEntity{
			id:    e.MemoryID,
			value: CalcMemoryValue(entity),
		})
	}

	// 按综合评分升序（最低价值优先淘汰）
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].value < scored[j].value
	})

	// 取前 toEvict 个
	ids := make([]string, 0, toEvict)
	for i := 0; i < toEvict && i < len(scored); i++ {
		ids = append(ids, scored[i].id)
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
	return allIDs, nil
}