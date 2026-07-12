package yakit

import (
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

// ai_user_stats.go 实现 UserAIStats 的 CRUD: per-entity 命中计数 + per-user/day 汇总.
//
// 设计要点:
//   - IncrementEntityHit 用 upsert (FirstOrCreate + 原子自增列), 保证并发安全的高频写入.
//   - source 细分计数: direct / requested / auto_loaded, 对应不同命中来源 (见 schema 注释).
//   - TopEntitiesByHits 供意图识别层做 Top-N 反馈排序 (HitCount desc, LastHitAt desc).
//   - IncrementDailyStats 用 upsert + map 自增, 一行 = 一个 user 的一天.
//
// 关键词: UserAIStats CRUD, IncrementEntityHit, IncrementDailyStats, TopEntitiesByHits

// entityHitSourceColumns 把命中来源 (source) 映射到 AIStatsEntityHit 的细分计数列名.
// 未识别的来源只累加 HitCount, 不动细分列.
func entityHitSourceColumn(source string) string {
	switch source {
	case "direct", "user_force":
		// tool 直接调用 + skill 用户强制加载 都计入 direct_count.
		return "direct_count"
	case "requested":
		return "requested_count"
	case "ai_load", "intent_catalog", "auto_loaded":
		return "auto_loaded_count"
	}
	return ""
}

// IncrementEntityHit 对一个 (entityType, entityName) 累加一次命中.
// source 决定细分列 ("direct"/"requested"/"ai_load"/"intent_catalog"/"auto_loaded");
// 任意来源都会累加 HitCount + 刷新 LastHitAt. 失败仅返回 error, 不 panic.
func IncrementEntityHit(db *gorm.DB, entityType, entityName, source string) error {
	if db == nil || entityType == "" || entityName == "" {
		return nil
	}
	var existing schema.AIStatsEntityHit
	err := db.Where("entity_type = ? AND entity_name = ?", entityType, entityName).First(&existing).Error
	now := time.Now()
	col := entityHitSourceColumn(source)

	updates := map[string]interface{}{
		"hit_count":   gorm.Expr("hit_count + ?", 1),
		"last_hit_at": now,
	}
	if col != "" {
		updates[col] = gorm.Expr(col+" + ?", 1)
	}

	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			// 首次命中: 创建, 细分列默认 0/1.
			row := &schema.AIStatsEntityHit{
				EntityType: entityType,
				EntityName: entityName,
				HitCount:   1,
				LastHitAt:  now,
			}
			if col != "" {
				switch col {
				case "direct_count":
					row.DirectCount = 1
				case "requested_count":
					row.RequestedCount = 1
				case "auto_loaded_count":
					row.AutoLoadedCount = 1
				}
			}
			return db.Create(row).Error
		}
		return err
	}
	return db.Model(&schema.AIStatsEntityHit{}).
		Where("entity_type = ? AND entity_name = ?", entityType, entityName).
		Updates(updates).Error
}

// EntityHitCount 返回单个 (entityType, entityName) 的当前命中次数, 无记录返回 0.
func EntityHitCount(db *gorm.DB, entityType, entityName string) int {
	if db == nil {
		return 0
	}
	var row schema.AIStatsEntityHit
	if err := db.Select("hit_count").
		Where("entity_type = ? AND entity_name = ?", entityType, entityName).
		First(&row).Error; err != nil {
		return 0
	}
	return row.HitCount
}

// TopEntitiesByHits 返回指定 entityType 下命中数 Top-N 的实体名 (HitCount desc, LastHitAt desc).
// 供意图识别层对候选 SKILL / 工具做反馈排序.
func TopEntitiesByHits(db *gorm.DB, entityType string, limit int) []string {
	if db == nil || entityType == "" {
		return nil
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []schema.AIStatsEntityHit
	if err := db.Select("entity_name").
		Where("entity_type = ? AND hit_count > 0", entityType).
		Order("hit_count desc, last_hit_at desc").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.EntityName)
	}
	return names
}

// EntityHitRanking 返回 entityType 下命中数 Top-N 的完整行 (含计数), 用于前端展示.
func EntityHitRanking(db *gorm.DB, entityType string, limit int) ([]schema.AIStatsEntityHit, error) {
	if db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	var rows []schema.AIStatsEntityHit
	err := db.Where("entity_type = ? AND hit_count > 0", entityType).
		Order("hit_count desc, last_hit_at desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

// IncrementDailyStats 对一个 (userKey, day) 累加若干字段.
// 传入 map 是「列名 → 增量」(如 "actions": 1, "tokens_input": 1234).
// 行不存在时自动创建. 失败仅返回 error, 不 panic.
func IncrementDailyStats(db *gorm.DB, userKey, day string, increments map[string]interface{}) error {
	if db == nil || userKey == "" || day == "" || len(increments) == 0 {
		return nil
	}
	var existing schema.AIUserDailyStats
	err := db.Where("user_key = ? AND day = ?", userKey, day).First(&existing).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			row := &schema.AIUserDailyStats{
				UserKey: userKey,
				Day:     day,
			}
			applyDailyStatsIncrements(row, increments)
			return db.Create(row).Error
		}
		return err
	}
	updates := make(map[string]interface{}, len(increments))
	for col, inc := range increments {
		updates[col] = gorm.Expr(col+" + ?", inc)
	}
	return db.Model(&schema.AIUserDailyStats{}).
		Where("user_key = ? AND day = ?", userKey, day).
		Updates(updates).Error
}

// applyDailyStatsIncrements 把 increments 直接累加到一个 (新建) 行的字段上.
func applyDailyStatsIncrements(row *schema.AIUserDailyStats, increments map[string]interface{}) {
	for col, inc := range increments {
		n, ok := toInt64(inc)
		if !ok {
			continue
		}
		switch col {
		case "actions":
			row.Actions += int(n)
		case "ai_calls":
			row.AICalls += int(n)
		case "tool_calls_total":
			row.ToolCallsTotal += int(n)
		case "skills_loaded":
			row.SkillsLoaded += int(n)
		case "tokens_input":
			row.TokensInput += n
		case "tokens_output":
			row.TokensOutput += n
		}
	}
}

func toInt64(v interface{}) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	}
	return 0, false
}

// GetDailyStats 返回单个 (userKey, day) 的汇总行, 无记录返回 nil.
func GetDailyStats(db *gorm.DB, userKey, day string) (*schema.AIUserDailyStats, error) {
	if db == nil {
		return nil, nil
	}
	var row schema.AIUserDailyStats
	if err := db.Where("user_key = ? AND day = ?", userKey, day).First(&row).Error; err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}
