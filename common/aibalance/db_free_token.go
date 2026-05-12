package aibalance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// 关键词: db_free_token, 免费用户每日 Token 限额持久化, 全局共享池 + 模型覆盖

const (
	// freeUserGlobalBucketModel 是「全局共享池」对应的 ModelName 哨兵值。
	// 用空字符串作为 ModelName 入库，配合 (date, model_name) 唯一索引把
	// 「全局共享池」与「模型级独立桶」放在同一张表里管理。
	// 关键词: 免费用户全局共享池, ModelName="" 哨兵
	freeUserGlobalBucketModel = ""

	// FreeUserTokenMUnit 是把 M（百万）token 单位换算为 raw token 的常量。
	FreeUserTokenMUnit int64 = 1_000_000
)

// freeTokenDB 跳过 GORM 软删除过滤；free_user_daily_token_usage 是聚合表，
// 没有「软删除-恢复」语义，全部用 Unscoped 直接对实际行操作，
// 避免 First 找不到 + Create unique constraint 冲突。
// 关键词: freeTokenDB, GORM Unscoped, 跳过软删除
func freeTokenDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsureFreeUserDailyTokenUsageTable ensures the free_user_daily_token_usage table exists.
// 关键词: EnsureFreeUserDailyTokenUsageTable
func EnsureFreeUserDailyTokenUsageTable() error {
	return GetDB().AutoMigrate(&schema.FreeUserDailyTokenUsage{}).Error
}

// freeTokenNowDate 抽出来便于测试 mock。
// 关键词: freeTokenNowDate
var freeTokenNowDate = func() string {
	return time.Now().Format("2006-01-02")
}

// FreeUserTokenModelOverride 描述对某个 -free 模型的限额覆盖配置。
// 关键词: FreeUserTokenModelOverride, 模型级覆盖, 模型豁免
type FreeUserTokenModelOverride struct {
	LimitM int64 `json:"limit_m"` // 该模型独立桶的日 Token 限额（M 单位）；<=0 表示不覆盖全局共享池
	Exempt bool  `json:"exempt"`  // 该模型是否豁免计费（直接放行，不计入任何桶）
}

// parseFreeUserTokenModelOverrides 将 ModelOverrides JSON 字符串解析为 map。
// 解析失败返回空 map，不抛错（避免阻塞 hot path）。
// 关键词: parseFreeUserTokenModelOverrides, JSON 解析
func parseFreeUserTokenModelOverrides(raw string) map[string]FreeUserTokenModelOverride {
	out := make(map[string]FreeUserTokenModelOverride)
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		log.Warnf("parseFreeUserTokenModelOverrides failed: %v", err)
		return make(map[string]FreeUserTokenModelOverride)
	}
	return out
}

// parseFreeUserTokenModelOverridesFromConfig 是 server.go onUsageForward 等
// hot path 路径的便捷封装：直接从 DB config 拉取并解析 overrides，
// 任何异常都退化为空 map（不阻塞业务）。
// 关键词: parseFreeUserTokenModelOverridesFromConfig, hot path 兜底
func parseFreeUserTokenModelOverridesFromConfig() map[string]FreeUserTokenModelOverride {
	cfg, err := GetRateLimitConfig()
	if err != nil {
		log.Warnf("parseFreeUserTokenModelOverridesFromConfig: GetRateLimitConfig failed: %v", err)
		return make(map[string]FreeUserTokenModelOverride)
	}
	return parseFreeUserTokenModelOverrides(cfg.FreeUserTokenModelOverrides)
}

// ternaryStr is a tiny helper for inline string ternary used in log lines.
// 关键词: ternaryStr 日志辅助
func ternaryStr(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}

// AddFreeUserDailyTokenUsage 累加本次请求的加权 token 到「全局共享池」与
// 可选的「模型独立桶」。modelName 为空时只累加全局桶；非空时同时累加全局桶
// 与模型桶（modelHasOwnBucket=true 时还会写一行 model 桶）。
//
// 设计：modelHasOwnBucket=false 时只累加全局桶（这是默认 -free 模型的行为）；
// modelHasOwnBucket=true 表示该模型在配置里指定了独立 limit_m（覆盖全局），
// 此时**不写入全局桶**，只写入模型桶（避免双重计费）。
//
// 跨日：表行天然按 date 拆分，不需要显式 reset；旧日数据由 cleanup 任务清理。
//
// 关键词: AddFreeUserDailyTokenUsage, UPSERT 累加, 全局/模型桶分流
func AddFreeUserDailyTokenUsage(modelName string, weighted int64, modelHasOwnBucket bool) error {
	if weighted <= 0 {
		return nil
	}
	date := freeTokenNowDate()

	bucket := freeUserGlobalBucketModel
	if modelHasOwnBucket && modelName != "" {
		bucket = modelName
	}
	return upsertFreeUserDailyTokenUsage(date, bucket, weighted)
}

// upsertFreeUserDailyTokenUsage 是 (date, model_name) 维度的 UPSERT 累加。
// 关键词: upsertFreeUserDailyTokenUsage, gorm.Expr 累加
func upsertFreeUserDailyTokenUsage(date, model string, delta int64) error {
	if date == "" {
		return fmt.Errorf("upsertFreeUserDailyTokenUsage: date is empty")
	}
	db := freeTokenDB()

	var row schema.FreeUserDailyTokenUsage
	err := db.Where("date = ? AND model_name = ?", date, model).First(&row).Error
	if err == nil {
		return db.Model(&schema.FreeUserDailyTokenUsage{}).
			Where("id = ?", row.ID).
			UpdateColumn("tokens_used", gorm.Expr("tokens_used + ?", delta)).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("upsertFreeUserDailyTokenUsage query failed: %v", err)
	}

	row = schema.FreeUserDailyTokenUsage{
		Date:       date,
		ModelName:  model,
		TokensUsed: delta,
	}
	if createErr := db.Create(&row).Error; createErr != nil {
		// 并发竞态：另一个 goroutine 已经先 Create，退化为 UPDATE 累加。
		var existing schema.FreeUserDailyTokenUsage
		if findErr := db.Where("date = ? AND model_name = ?", date, model).First(&existing).Error; findErr == nil {
			return db.Model(&schema.FreeUserDailyTokenUsage{}).
				Where("id = ?", existing.ID).
				UpdateColumn("tokens_used", gorm.Expr("tokens_used + ?", delta)).Error
		}
		return fmt.Errorf("upsertFreeUserDailyTokenUsage create failed: %v", createErr)
	}
	return nil
}

// GetFreeUserDailyTokenUsage 读取某 (date, model) 桶的累计已用 token。
// 不存在视为 0；任何 DB 错误向上抛。
// 关键词: GetFreeUserDailyTokenUsage
func GetFreeUserDailyTokenUsage(date, model string) (int64, error) {
	var row schema.FreeUserDailyTokenUsage
	err := freeTokenDB().Where("date = ? AND model_name = ?", date, model).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("GetFreeUserDailyTokenUsage failed: %v", err)
	}
	return row.TokensUsed, nil
}

// FreeUserTokenLimitDecision 描述一次免费用户 token 限额检查的判定结果。
// 关键词: FreeUserTokenLimitDecision, 免费用户限额检查结果
type FreeUserTokenLimitDecision struct {
	Allowed     bool   // 是否允许本次请求
	Exempt      bool   // 是否被模型级豁免（exempt=true 时 Allowed 必为 true）
	Bucket      string // 实际使用的桶："global" 或 "model"
	ModelHasOwn bool   // 模型是否有独立桶（true=用模型桶；false=用全局桶）
	TokensUsed  int64  // 当前桶已用 token（raw）
	TokensLimit int64  // 当前桶日限额（raw token，0=无限）
	Date        string // 限额所属日期（YYYY-MM-DD）
}

// CheckFreeUserDailyTokenLimit 在免费用户请求转发前检查日 Token 限额。
// 模型级覆盖优先级：exempt=true > limit_m>0 -> 模型独立桶 > 全局共享池。
//
// 返回 (decision, error)。任何 DB 异常视为「放行」（与现有字节限额检查策略一致），
// 错误日志由调用方处理。
//
// 关键词: CheckFreeUserDailyTokenLimit, 模型覆盖优先, 全局共享池兜底
func CheckFreeUserDailyTokenLimit(modelName string) (*FreeUserTokenLimitDecision, error) {
	cfg, err := GetRateLimitConfig()
	if err != nil {
		return &FreeUserTokenLimitDecision{Allowed: true}, fmt.Errorf("GetRateLimitConfig failed: %v", err)
	}

	overrides := parseFreeUserTokenModelOverrides(cfg.FreeUserTokenModelOverrides)
	override, hasOverride := overrides[modelName]

	// 模型豁免：直接放行
	if hasOverride && override.Exempt {
		return &FreeUserTokenLimitDecision{
			Allowed: true,
			Exempt:  true,
			Bucket:  "model",
			Date:    freeTokenNowDate(),
		}, nil
	}

	date := freeTokenNowDate()
	decision := &FreeUserTokenLimitDecision{Date: date}

	if hasOverride && override.LimitM > 0 {
		// 模型独立桶
		decision.Bucket = "model"
		decision.ModelHasOwn = true
		decision.TokensLimit = override.LimitM * FreeUserTokenMUnit
		used, err := GetFreeUserDailyTokenUsage(date, modelName)
		if err != nil {
			decision.Allowed = true
			return decision, err
		}
		decision.TokensUsed = used
	} else {
		// 全局共享池
		decision.Bucket = "global"
		decision.ModelHasOwn = false
		limitM := cfg.FreeUserTokenLimitM
		if limitM <= 0 {
			limitM = 1200
		}
		decision.TokensLimit = limitM * FreeUserTokenMUnit
		used, err := GetFreeUserDailyTokenUsage(date, freeUserGlobalBucketModel)
		if err != nil {
			decision.Allowed = true
			return decision, err
		}
		decision.TokensUsed = used
	}

	if decision.TokensLimit <= 0 {
		decision.Allowed = true
		return decision, nil
	}
	decision.Allowed = decision.TokensUsed < decision.TokensLimit
	return decision, nil
}

// QueryFreeUserTokenUsageSnapshot 收集当天所有桶的快照（全局 + 各模型独立桶），
// 供 portal /portal/api/rate-limit-status 实时显示。
// 关键词: QueryFreeUserTokenUsageSnapshot, portal 实时面板
type FreeUserTokenBucketSnapshot struct {
	Model      string `json:"model"` // "" = 全局共享池
	TokensUsed int64  `json:"tokens_used"`
	UsedM      float64 `json:"used_m"`
	LimitM     int64  `json:"limit_m"`
	Exempt     bool   `json:"exempt"`
}

// QueryFreeUserTokenUsageSnapshot 返回当天 (global + per-model) 桶的快照。
// 关键词: QueryFreeUserTokenUsageSnapshot
func QueryFreeUserTokenUsageSnapshot() (global FreeUserTokenBucketSnapshot, perModel []FreeUserTokenBucketSnapshot, date string, err error) {
	date = freeTokenNowDate()
	cfg, cfgErr := GetRateLimitConfig()
	if cfgErr != nil {
		err = cfgErr
		return
	}
	overrides := parseFreeUserTokenModelOverrides(cfg.FreeUserTokenModelOverrides)

	var rows []schema.FreeUserDailyTokenUsage
	if scanErr := freeTokenDB().Where("date = ?", date).Find(&rows).Error; scanErr != nil {
		err = fmt.Errorf("QueryFreeUserTokenUsageSnapshot find failed: %v", scanErr)
		return
	}

	rowByModel := make(map[string]int64, len(rows))
	for _, r := range rows {
		rowByModel[r.ModelName] = r.TokensUsed
	}

	// 全局桶
	global = FreeUserTokenBucketSnapshot{
		Model:      "",
		TokensUsed: rowByModel[freeUserGlobalBucketModel],
		LimitM:     cfg.FreeUserTokenLimitM,
	}
	if global.LimitM <= 0 {
		global.LimitM = 1200
	}
	global.UsedM = float64(global.TokensUsed) / float64(FreeUserTokenMUnit)

	// 每模型桶（来自配置 + DB 行的并集）
	seen := make(map[string]bool)
	for model, ov := range overrides {
		seen[model] = true
		snap := FreeUserTokenBucketSnapshot{
			Model:      model,
			TokensUsed: rowByModel[model],
			LimitM:     ov.LimitM,
			Exempt:     ov.Exempt,
		}
		snap.UsedM = float64(snap.TokensUsed) / float64(FreeUserTokenMUnit)
		perModel = append(perModel, snap)
	}
	for model, used := range rowByModel {
		if model == freeUserGlobalBucketModel {
			continue
		}
		if seen[model] {
			continue
		}
		// DB 中有数据但配置里已经移除：仍展示出来（提示运维这一天有累计）
		snap := FreeUserTokenBucketSnapshot{
			Model:      model,
			TokensUsed: used,
		}
		snap.UsedM = float64(snap.TokensUsed) / float64(FreeUserTokenMUnit)
		perModel = append(perModel, snap)
	}
	return
}

// CleanupOldFreeUserTokenUsage deletes rows whose date < (today - keepDays).
// 与其他每日聚合表保持一致的 100 天保留窗。
// 关键词: CleanupOldFreeUserTokenUsage, Unscoped 硬删除
func CleanupOldFreeUserTokenUsage(keepDays int) (int64, error) {
	if keepDays <= 0 {
		keepDays = 100
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	tx := freeTokenDB().Where("date < ?", cutoff).Delete(&schema.FreeUserDailyTokenUsage{})
	if tx.Error != nil {
		return 0, fmt.Errorf("CleanupOldFreeUserTokenUsage failed: %v", tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Infof("CleanupOldFreeUserTokenUsage removed %d rows older than %s", tx.RowsAffected, cutoff)
	}
	return tx.RowsAffected, nil
}

// ==================== API Key Token-dimension helpers ====================

// UpdateAiApiKeyTokenUsed adds to the Token used counter for an API key.
// 与字节维度并行；TokenLimit/TokenUsed 字段单位为 raw token（已经过四维倍率加权）。
// 关键词: UpdateAiApiKeyTokenUsed, Token 维度计费
func UpdateAiApiKeyTokenUsed(apiKey string, additionalToken int64) error {
	if additionalToken <= 0 {
		return nil
	}
	return GetDB().Model(&schema.AiApiKeys{}).
		Where("api_key = ?", apiKey).
		UpdateColumn("token_used", gorm.Expr("token_used + ?", additionalToken)).Error
}

// CheckAiApiKeyTokenLimit checks if an API key has exceeded its Token limit.
// 与 CheckAiApiKeyTrafficLimit 同结构（bool, error），便于上层调用对齐。
// 关键词: CheckAiApiKeyTokenLimit, Token 限额前置检查
func CheckAiApiKeyTokenLimit(apiKey string) (bool, error) {
	var key schema.AiApiKeys
	if err := GetDB().Where("api_key = ?", apiKey).First(&key).Error; err != nil {
		return false, fmt.Errorf("failed to find API key: %v", err)
	}
	if !key.TokenLimitEnable {
		return true, nil
	}
	if key.TokenLimit <= 0 {
		return true, nil
	}
	if key.TokenUsed >= key.TokenLimit {
		return false, nil
	}
	return true, nil
}

// UpdateAiApiKeyTokenLimit updates the Token limit settings for an API key.
// 关键词: UpdateAiApiKeyTokenLimit
func UpdateAiApiKeyTokenLimit(id uint, limit int64, enable bool) error {
	result := GetDB().Model(&schema.AiApiKeys{}).Where("id = ?", id).Updates(map[string]interface{}{
		"token_limit":        limit,
		"token_limit_enable": enable,
	})
	if result.Error != nil {
		return fmt.Errorf("failed to update token limit for API key ID %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	log.Infof("Successfully updated token limit for API key (ID: %d), limit: %d, enabled: %v", id, limit, enable)
	return nil
}

// ResetAiApiKeyTokenUsed resets the token used counter for an API key.
// 关键词: ResetAiApiKeyTokenUsed
func ResetAiApiKeyTokenUsed(id uint) error {
	result := GetDB().Model(&schema.AiApiKeys{}).Where("id = ?", id).Update("token_used", 0)
	if result.Error != nil {
		return fmt.Errorf("failed to reset token used for API key ID %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	log.Infof("Successfully reset token used for API key (ID: %d)", id)
	return nil
}
