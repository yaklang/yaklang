package aibalance

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// 关键词: db_free_ip, 单 IP 免费模型每日用量限额, 防盗刷, 公共免费接口公平
//
// 设计目标：免费接口应该对所有用户公平，而不希望被某些 IP 高频打满。
// 这里按「真实客户端 IP」维度记录每天对「计费免费模型」的请求次数与加权 Token，
// 超出每日上限即拦截，提示用户自行配置 AI 后端；切日点与免费 Token 限额一致
// （北京时间每日 06:00，复用 freeTokenNowDate），旧日数据由 cleanup 任务清理。

// freeIPDB 跳过 GORM 软删除过滤；free_user_ip_daily_usage 是聚合表，
// 没有「软删除-恢复」语义，全部用 Unscoped 直接对实际行操作。
// 关键词: freeIPDB, GORM Unscoped, 跳过软删除
func freeIPDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsureFreeUserIPDailyUsageTable ensures the free_user_ip_daily_usage table exists.
// 关键词: EnsureFreeUserIPDailyUsageTable
func EnsureFreeUserIPDailyUsageTable() error {
	return GetDB().AutoMigrate(&FreeUserIPDailyUsage{}).Error
}

// freeIPUsageIgnoredIP 判断给定 IP 是否应被忽略（不参与统计与限额）。
// 空串、unknown 占位都直接放行，避免把无法识别的客户端聚到同一个桶里误伤。
// 关键词: freeIPUsageIgnoredIP, 空 IP 放行, unknown 放行
func freeIPUsageIgnoredIP(ip string) bool {
	ip = strings.TrimSpace(ip)
	return ip == "" || ip == "unknown"
}

// upsertFreeUserIPDailyUsage 是 (date, ip) 维度的 UPSERT 累加，请求数与 Token 可分别增量。
// 关键词: upsertFreeUserIPDailyUsage, gorm.Expr 累加, 并发竞态 fallback
func upsertFreeUserIPDailyUsage(date, ip string, deltaReq, deltaTokens int64) error {
	if date == "" {
		return fmt.Errorf("upsertFreeUserIPDailyUsage: date is empty")
	}
	if freeIPUsageIgnoredIP(ip) {
		return nil
	}
	if deltaReq <= 0 && deltaTokens <= 0 {
		return nil
	}
	db := freeIPDB()

	updateExisting := func(id uint) error {
		updates := map[string]interface{}{
			"last_seen_at": time.Now(),
		}
		if deltaReq > 0 {
			updates["request_count"] = gorm.Expr("request_count + ?", deltaReq)
		}
		if deltaTokens > 0 {
			updates["tokens_used"] = gorm.Expr("tokens_used + ?", deltaTokens)
		}
		return db.Model(&FreeUserIPDailyUsage{}).Where("id = ?", id).Updates(updates).Error
	}

	var row FreeUserIPDailyUsage
	err := db.Where("date = ? AND ip = ?", date, ip).First(&row).Error
	if err == nil {
		return updateExisting(row.ID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("upsertFreeUserIPDailyUsage query failed: %v", err)
	}

	row = FreeUserIPDailyUsage{
		Date:         date,
		IP:           ip,
		RequestCount: deltaReq,
		TokensUsed:   deltaTokens,
		LastSeenAt:   time.Now(),
	}
	if createErr := db.Create(&row).Error; createErr != nil {
		// 并发竞态：另一个 goroutine 已经先 Create，退化为 UPDATE 累加。
		var existing FreeUserIPDailyUsage
		if findErr := db.Where("date = ? AND ip = ?", date, ip).First(&existing).Error; findErr == nil {
			return updateExisting(existing.ID)
		}
		return fmt.Errorf("upsertFreeUserIPDailyUsage create failed: %v", createErr)
	}
	return nil
}

// AddFreeUserIPDailyRequest 为某 IP 当天累加一次「计费免费模型」请求计数。
// 关键词: AddFreeUserIPDailyRequest, 请求次数计费
func AddFreeUserIPDailyRequest(ip string) error {
	if freeIPUsageIgnoredIP(ip) {
		return nil
	}
	return upsertFreeUserIPDailyUsage(freeTokenNowDate(), ip, 1, 0)
}

// AddFreeUserIPDailyTokens 为某 IP 当天累加加权 Token（来自 ComputeWeightedTokens）。
// 关键词: AddFreeUserIPDailyTokens, 加权 Token 计费
func AddFreeUserIPDailyTokens(ip string, weighted int64) error {
	if weighted <= 0 || freeIPUsageIgnoredIP(ip) {
		return nil
	}
	return upsertFreeUserIPDailyUsage(freeTokenNowDate(), ip, 0, weighted)
}

// GetFreeUserIPDailyUsage 读取某 (date, ip) 当天已用请求数与加权 Token。
// 不存在视为 0；任何 DB 错误向上抛。
// 关键词: GetFreeUserIPDailyUsage
func GetFreeUserIPDailyUsage(date, ip string) (requestCount, tokensUsed int64, err error) {
	var row FreeUserIPDailyUsage
	qErr := freeIPDB().Where("date = ? AND ip = ?", date, ip).First(&row).Error
	if qErr != nil {
		if errors.Is(qErr, gorm.ErrRecordNotFound) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("GetFreeUserIPDailyUsage failed: %v", qErr)
	}
	return row.RequestCount, row.TokensUsed, nil
}

// FreeUserIPLimitDecision 描述一次单 IP 免费模型每日限额检查的判定结果。
// 关键词: FreeUserIPLimitDecision, 单 IP 限额检查结果
type FreeUserIPLimitDecision struct {
	Allowed      bool   // 是否允许本次请求
	IP           string // 检查的客户端 IP
	RequestUsed  int64  // 当天已用请求数
	RequestLimit int64  // 每日请求上限（0 = 不限）
	TokensUsed   int64  // 当天已用加权 Token
	TokensLimit  int64  // 每日加权 Token 上限（raw token，0 = 不限）
	ExceededKind string // 超限维度："request" | "token" | ""（未超限）
	Date         string // 限额所属日期（YYYY-MM-DD）
}

// CheckFreeUserIPDailyLimit 在免费模型请求转发前检查单 IP 每日限额。
// config 未启用、IP 不可识别、上下限均为 0 时一律放行；任何 DB 异常视为「放行」
// （与现有 Token / 流量限额检查策略一致，避免限额逻辑可用性反噬业务）。
// 关键词: CheckFreeUserIPDailyLimit, 单 IP 每日限额前置检查, DB 异常放行
func CheckFreeUserIPDailyLimit(ip string) (*FreeUserIPLimitDecision, error) {
	date := freeTokenNowDate()
	decision := &FreeUserIPLimitDecision{Allowed: true, IP: ip, Date: date}

	if freeIPUsageIgnoredIP(ip) {
		return decision, nil
	}

	cfg, err := GetRateLimitConfig()
	if err != nil {
		return decision, fmt.Errorf("GetRateLimitConfig failed: %v", err)
	}
	if !cfg.FreeUserIPLimitEnable {
		return decision, nil
	}

	requestLimit := cfg.FreeUserIPDailyRequestLimit
	tokensLimit := int64(0)
	if cfg.FreeUserIPDailyTokenLimitM > 0 {
		tokensLimit = cfg.FreeUserIPDailyTokenLimitM * FreeUserTokenMUnit
	}
	decision.RequestLimit = requestLimit
	decision.TokensLimit = tokensLimit
	if requestLimit <= 0 && tokensLimit <= 0 {
		return decision, nil
	}

	requestUsed, tokensUsed, gErr := GetFreeUserIPDailyUsage(date, ip)
	if gErr != nil {
		return decision, gErr
	}
	decision.RequestUsed = requestUsed
	decision.TokensUsed = tokensUsed

	if requestLimit > 0 && requestUsed >= requestLimit {
		decision.Allowed = false
		decision.ExceededKind = "request"
		return decision, nil
	}
	if tokensLimit > 0 && tokensUsed >= tokensLimit {
		decision.Allowed = false
		decision.ExceededKind = "token"
		return decision, nil
	}
	return decision, nil
}

// FreeUserIPUsageRow 是单个 IP 当天用量快照行，供 portal 面板展示。
// 关键词: FreeUserIPUsageRow, portal 单 IP 用量
type FreeUserIPUsageRow struct {
	IP           string  `json:"ip"`
	RequestCount int64   `json:"request_count"`
	TokensUsed   int64   `json:"tokens_used"`
	UsedM        float64 `json:"used_m"`
}

// QueryFreeUserIPUsageSnapshot 返回当天「有多少个 IP 在使用免费模型」与按 Token 降序的 Top-N IP。
// 关键词: QueryFreeUserIPUsageSnapshot, distinct IP 计数, Top-N 用量榜
func QueryFreeUserIPUsageSnapshot(topN int) (distinctCount int64, top []FreeUserIPUsageRow, date string, err error) {
	date = freeTokenNowDate()
	if topN <= 0 {
		topN = 20
	}

	db := freeIPDB()
	if cErr := db.Model(&FreeUserIPDailyUsage{}).Where("date = ?", date).Count(&distinctCount).Error; cErr != nil {
		err = fmt.Errorf("QueryFreeUserIPUsageSnapshot count failed: %v", cErr)
		return
	}

	var rows []FreeUserIPDailyUsage
	if fErr := db.Where("date = ?", date).
		Order("tokens_used DESC, request_count DESC").
		Limit(topN).
		Find(&rows).Error; fErr != nil {
		err = fmt.Errorf("QueryFreeUserIPUsageSnapshot find failed: %v", fErr)
		return
	}
	for _, r := range rows {
		top = append(top, FreeUserIPUsageRow{
			IP:           r.IP,
			RequestCount: r.RequestCount,
			TokensUsed:   r.TokensUsed,
			UsedM:        float64(r.TokensUsed) / float64(FreeUserTokenMUnit),
		})
	}
	return
}

// CleanupOldFreeUserIPUsage deletes rows whose date < (today - keepDays).
// 这张表按 (date, ip) 展开，行数随独立 IP 数增长，保留窗设短即可（仅够面板看今日）。
// 关键词: CleanupOldFreeUserIPUsage, Unscoped 硬删除, 短保留窗
func CleanupOldFreeUserIPUsage(keepDays int) (int64, error) {
	if keepDays <= 0 {
		keepDays = 2
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	tx := freeIPDB().Where("date < ?", cutoff).Delete(&FreeUserIPDailyUsage{})
	if tx.Error != nil {
		return 0, fmt.Errorf("CleanupOldFreeUserIPUsage failed: %v", tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Infof("CleanupOldFreeUserIPUsage removed %d rows older than %s", tx.RowsAffected, cutoff)
	}
	return tx.RowsAffected, nil
}
