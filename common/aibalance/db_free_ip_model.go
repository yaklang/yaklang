package aibalance

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// 关键词: db_free_ip_model, 单 IP 按模型每日用量, per-IP TOP 模型展示
//
// 设计：这张表只服务于面板「每个免费 IP 用得最多的模型」展示，不参与任何限额判定。
// 写入点与 FreeUserIPDailyUsage 完全并行（同样在请求计数 / Token 计费两处累加），
// 只是多带一个 model 维度。读取走批量查询，把当天若干 IP 的 (model -> 用量) 一次取回，
// 在内存里分组取 TopN，避免 N 次小查询。

// freeIPModelDB 与 freeIPDB 一样跳过 GORM 软删除；这是纯聚合表，无软删除-恢复语义。
// 关键词: freeIPModelDB, GORM Unscoped, 跳过软删除
func freeIPModelDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsureFreeUserIPModelDailyUsageTable ensures the free_user_ip_model_daily_usage table exists.
// 关键词: EnsureFreeUserIPModelDailyUsageTable
func EnsureFreeUserIPModelDailyUsageTable() error {
	return GetDB().AutoMigrate(&FreeUserIPModelDailyUsage{}).Error
}

// upsertFreeUserIPModelDailyUsage 是 (date, ip, model) 维度的 UPSERT 累加。
// 空 IP / unknown 直接放行；空 model 也跳过（无法归类）。
// deltaRawTokens 是原始 Token 数量（数量，所有免费模型都累加）；
// deltaWeighted 是加权/计费 Token（金额基准，仅计费模型 >0，不计费模型传 0）。
// 关键词: upsertFreeUserIPModelDailyUsage, gorm.Expr 累加, 原始Token+加权Token, 并发竞态 fallback
func upsertFreeUserIPModelDailyUsage(date, ip, model string, deltaReq, deltaRawTokens, deltaWeighted int64) error {
	if date == "" {
		return fmt.Errorf("upsertFreeUserIPModelDailyUsage: date is empty")
	}
	if freeIPUsageIgnoredIP(ip) || model == "" {
		return nil
	}
	if deltaReq <= 0 && deltaRawTokens <= 0 && deltaWeighted <= 0 {
		return nil
	}
	db := freeIPModelDB()

	updateExisting := func(id uint) error {
		updates := map[string]interface{}{
			"last_seen_at": time.Now(),
		}
		if deltaReq > 0 {
			updates["request_count"] = gorm.Expr("request_count + ?", deltaReq)
		}
		if deltaRawTokens > 0 {
			updates["tokens_used"] = gorm.Expr("tokens_used + ?", deltaRawTokens)
		}
		if deltaWeighted > 0 {
			updates["weighted_tokens"] = gorm.Expr("weighted_tokens + ?", deltaWeighted)
		}
		return db.Model(&FreeUserIPModelDailyUsage{}).Where("id = ?", id).Updates(updates).Error
	}

	var row FreeUserIPModelDailyUsage
	err := db.Where("date = ? AND ip = ? AND model = ?", date, ip, model).First(&row).Error
	if err == nil {
		return updateExisting(row.ID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("upsertFreeUserIPModelDailyUsage query failed: %v", err)
	}

	row = FreeUserIPModelDailyUsage{
		Date:           date,
		IP:             ip,
		ModelName:      model,
		RequestCount:   deltaReq,
		TokensUsed:     deltaRawTokens,
		WeightedTokens: deltaWeighted,
		LastSeenAt:     time.Now(),
	}
	if createErr := db.Create(&row).Error; createErr != nil {
		// 并发竞态：另一个 goroutine 已先 Create，退化为 UPDATE 累加。
		var existing FreeUserIPModelDailyUsage
		if findErr := db.Where("date = ? AND ip = ? AND model = ?", date, ip, model).First(&existing).Error; findErr == nil {
			return updateExisting(existing.ID)
		}
		return fmt.Errorf("upsertFreeUserIPModelDailyUsage create failed: %v", createErr)
	}
	return nil
}

// AddFreeUserIPModelDailyRequest 为某 (IP, model) 当天累加一次请求计数。
// 关键词: AddFreeUserIPModelDailyRequest, 按模型请求计数
func AddFreeUserIPModelDailyRequest(ip, model string) error {
	if freeIPUsageIgnoredIP(ip) || model == "" {
		return nil
	}
	return upsertFreeUserIPModelDailyUsage(freeTokenNowDate(), ip, model, 1, 0, 0)
}

// AddFreeUserIPModelDailyUsageTokens 为某 (IP, model) 当天累加用量：
// rawTokens 是原始 Token 数量（数量，所有免费模型都传真实值，含不计费模型）；
// weighted 是加权/计费 Token（金额基准，不计费/豁免模型传 0）。
// 这样面板能对不计费模型「计数量、不算钱」(¥0)。
// 关键词: AddFreeUserIPModelDailyUsageTokens, 按模型 原始Token+加权Token, 不计费计数量
func AddFreeUserIPModelDailyUsageTokens(ip, model string, rawTokens, weighted int64) error {
	if freeIPUsageIgnoredIP(ip) || model == "" {
		return nil
	}
	if rawTokens <= 0 && weighted <= 0 {
		return nil
	}
	return upsertFreeUserIPModelDailyUsage(freeTokenNowDate(), ip, model, 0, rawTokens, weighted)
}

// FreeIPModelUsageRow 是单个 IP 的某个模型用量行，供面板「TOP 模型」展示。
// TokensUsed/UsedM 是原始 Token 数量（数量，含不计费模型）；
// WeightedTokens/WeightedM 是加权/计费 Token（金额基准，不计费模型为 0）。
// 关键词: FreeIPModelUsageRow, per-IP 模型用量, 数量 vs 金额
type FreeIPModelUsageRow struct {
	Model          string  `json:"model"`
	RequestCount   int64   `json:"request_count"`
	TokensUsed     int64   `json:"tokens_used"`
	UsedM          float64 `json:"used_m"`
	WeightedTokens int64   `json:"weighted_tokens"`
	WeightedM      float64 `json:"weighted_m"`
}

// QueryFreeIPTopModelsBatch 一次取回当天给定若干 IP 的「按模型用量」并在内存里分组，
// 每个 IP 取按加权 Token 降序的 TopN 模型，返回 map[ip][]FreeIPModelUsageRow。
// 入参 ips 为空时直接返回空 map，不打 DB。
// 关键词: QueryFreeIPTopModelsBatch, 批量取 per-IP TOP 模型, 内存分组
func QueryFreeIPTopModelsBatch(ips []string, topN int) (map[string][]FreeIPModelUsageRow, error) {
	result := make(map[string][]FreeIPModelUsageRow)
	if len(ips) == 0 {
		return result, nil
	}
	if topN <= 0 {
		topN = 3
	}
	date := freeTokenNowDate()

	var rows []FreeUserIPModelDailyUsage
	if err := freeIPModelDB().
		Where("date = ? AND ip IN (?)", date, ips).
		Find(&rows).Error; err != nil {
		return result, fmt.Errorf("QueryFreeIPTopModelsBatch find failed: %v", err)
	}

	grouped := make(map[string][]FreeIPModelUsageRow)
	for _, r := range rows {
		grouped[r.IP] = append(grouped[r.IP], FreeIPModelUsageRow{
			Model:          r.ModelName,
			RequestCount:   r.RequestCount,
			TokensUsed:     r.TokensUsed,
			UsedM:          float64(r.TokensUsed) / float64(FreeUserTokenMUnit),
			WeightedTokens: r.WeightedTokens,
			WeightedM:      float64(r.WeightedTokens) / float64(FreeUserTokenMUnit),
		})
	}
	for ip, list := range grouped {
		// 按「原始 Token 数量」降序（数量是 TOP 排序依据，不计费模型也能凭真实用量上榜），
		// 数量相同再看加权(金额)，最后看请求次数。
		sort.Slice(list, func(i, j int) bool {
			if list[i].TokensUsed != list[j].TokensUsed {
				return list[i].TokensUsed > list[j].TokensUsed
			}
			if list[i].WeightedTokens != list[j].WeightedTokens {
				return list[i].WeightedTokens > list[j].WeightedTokens
			}
			return list[i].RequestCount > list[j].RequestCount
		})
		if len(list) > topN {
			list = list[:topN]
		}
		result[ip] = list
	}
	return result, nil
}

// CleanupOldFreeUserIPModelUsage deletes rows whose date < (today - keepDays).
// 这张表按 (date, ip, model) 展开，行数随组合增长，保留窗设短即可（仅够面板看今日）。
// 关键词: CleanupOldFreeUserIPModelUsage, Unscoped 硬删除, 短保留窗
func CleanupOldFreeUserIPModelUsage(keepDays int) (int64, error) {
	if keepDays <= 0 {
		keepDays = 2
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	tx := freeIPModelDB().Where("date < ?", cutoff).Delete(&FreeUserIPModelDailyUsage{})
	if tx.Error != nil {
		return 0, fmt.Errorf("CleanupOldFreeUserIPModelUsage failed: %v", tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Infof("CleanupOldFreeUserIPModelUsage removed %d rows older than %s", tx.RowsAffected, cutoff)
	}
	return tx.RowsAffected, nil
}
