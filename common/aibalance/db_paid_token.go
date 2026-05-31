package aibalance

import (
	"errors"
	"fmt"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// 付费用户全局日 Token 总额度（第二道硬门）的持久化层。
//
// 与免费用户日 Token 限额（db_free_token.go）并列：
//   - 免费门：CheckFreeUserDailyTokenLimit，按免费模型日桶（全局共享池 + 模型独立桶）
//   - 付费门：CheckPaidUserDailyTokenLimit，把所有付费 API Key 当天产生的加权计费 Token
//     聚合到一个全局桶，超过 PaidUserTokenLimitM 后所有付费请求一律 429。
//
// 两道门都是「硬门」：都会造成 429 余额不足；都在北京时间每日 06:00 清零
// （复用 db_free_token.go 的 freeTokenNowDate / beijingTZ / FreeUserTokenMUnit）。
//
// 关键词: db_paid_token, 付费用户全局日 Token 总额度, 第二道硬门, 429 余额不足

// PaidUserDailyTokenUsage 持久化付费用户「每日加权计费 Token 已用量」全局聚合行。
// 一行 = 一个自然日（date 唯一）；聚合所有付费 API Key 当天产生的加权计费 Token。
// 跨日由 date 维度天然拆分；旧日数据由 cleanup 任务清理。
// 关键词: paid_user_daily_token_usage, 付费 Token 全局桶, 每日聚合
type PaidUserDailyTokenUsage struct {
	gorm.Model

	Date       string `json:"date" gorm:"size:10;unique_index:idx_paid_token;not null"`
	TokensUsed int64  `json:"tokens_used"`
}

func (a *PaidUserDailyTokenUsage) TableName() string {
	return "paid_user_daily_token_usage"
}

// paidTokenDB 与 freeTokenDB 一致：跳过 GORM 软删除过滤，聚合表直接对实际行操作。
// 关键词: paidTokenDB, GORM Unscoped, 跳过软删除
func paidTokenDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsurePaidUserDailyTokenUsageTable ensures the paid_user_daily_token_usage table exists.
// 关键词: EnsurePaidUserDailyTokenUsageTable
func EnsurePaidUserDailyTokenUsageTable() error {
	return GetDB().AutoMigrate(&PaidUserDailyTokenUsage{}).Error
}

// AddPaidUserDailyTokenUsage 把本次付费请求的加权计费 Token 累加到当天全局桶。
// weighted <= 0 时直接返回（IsFree 模型 weighted=0，天然不计入付费桶）。
// 关键词: AddPaidUserDailyTokenUsage, 付费全局桶 UPSERT 累加
func AddPaidUserDailyTokenUsage(weighted int64) error {
	if weighted <= 0 {
		return nil
	}
	date := freeTokenNowDate()
	if date == "" {
		return fmt.Errorf("AddPaidUserDailyTokenUsage: date is empty")
	}
	db := paidTokenDB()

	var row PaidUserDailyTokenUsage
	err := db.Where("date = ?", date).First(&row).Error
	if err == nil {
		return db.Model(&PaidUserDailyTokenUsage{}).
			Where("id = ?", row.ID).
			UpdateColumn("tokens_used", gorm.Expr("tokens_used + ?", weighted)).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("AddPaidUserDailyTokenUsage query failed: %v", err)
	}

	row = PaidUserDailyTokenUsage{Date: date, TokensUsed: weighted}
	if createErr := db.Create(&row).Error; createErr != nil {
		// 并发竞态：另一个 goroutine 已经先 Create，退化为 UPDATE 累加。
		var existing PaidUserDailyTokenUsage
		if findErr := db.Where("date = ?", date).First(&existing).Error; findErr == nil {
			return db.Model(&PaidUserDailyTokenUsage{}).
				Where("id = ?", existing.ID).
				UpdateColumn("tokens_used", gorm.Expr("tokens_used + ?", weighted)).Error
		}
		return fmt.Errorf("AddPaidUserDailyTokenUsage create failed: %v", createErr)
	}
	return nil
}

// GetPaidUserDailyTokenUsage 读取某自然日全局桶已用 token；不存在视为 0。
// 关键词: GetPaidUserDailyTokenUsage
func GetPaidUserDailyTokenUsage(date string) (int64, error) {
	var row PaidUserDailyTokenUsage
	err := paidTokenDB().Where("date = ?", date).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("GetPaidUserDailyTokenUsage failed: %v", err)
	}
	return row.TokensUsed, nil
}

// PaidUserTokenLimitDecision 描述一次付费用户全局日 Token 总额度检查的判定结果。
// 关键词: PaidUserTokenLimitDecision, 付费全局额度检查结果
type PaidUserTokenLimitDecision struct {
	Allowed     bool   // 是否允许本次付费请求
	TokensUsed  int64  // 当天全局桶已用 token（raw）
	TokensLimit int64  // 当天全局桶上限（raw token，0=不限制）
	Date        string // 限额所属日期（YYYY-MM-DD）
}

// CheckPaidUserDailyTokenLimit 在付费 API Key 请求转发前检查全局日 Token 总额度。
// PaidUserTokenLimitM <= 0 时不限制（Allowed=true）。
// 任何 DB 异常视为「放行」（与免费/字节限额检查策略一致），错误日志由调用方处理。
// 关键词: CheckPaidUserDailyTokenLimit, 付费全局硬门, 0 不限制
func CheckPaidUserDailyTokenLimit() (*PaidUserTokenLimitDecision, error) {
	cfg, err := GetRateLimitConfig()
	if err != nil {
		return &PaidUserTokenLimitDecision{Allowed: true}, fmt.Errorf("GetRateLimitConfig failed: %v", err)
	}

	date := freeTokenNowDate()
	decision := &PaidUserTokenLimitDecision{Date: date}

	if cfg.PaidUserTokenLimitM <= 0 {
		decision.Allowed = true
		return decision, nil
	}
	decision.TokensLimit = cfg.PaidUserTokenLimitM * FreeUserTokenMUnit

	used, err := GetPaidUserDailyTokenUsage(date)
	if err != nil {
		decision.Allowed = true
		return decision, err
	}
	decision.TokensUsed = used
	decision.Allowed = decision.TokensUsed < decision.TokensLimit
	return decision, nil
}

// PaidUserTokenUsageSnapshot 是付费全局日 Token 总额度的当日快照，供 portal 实时面板显示。
// 关键词: PaidUserTokenUsageSnapshot, portal 付费额度面板
type PaidUserTokenUsageSnapshot struct {
	TokensUsed int64   `json:"tokens_used"`
	UsedM      float64 `json:"used_m"`
	LimitM     int64   `json:"limit_m"` // 0=不限制
	Date       string  `json:"date"`
}

// QueryPaidUserTokenUsageSnapshot 返回当天付费全局桶快照。
// 关键词: QueryPaidUserTokenUsageSnapshot
func QueryPaidUserTokenUsageSnapshot() (PaidUserTokenUsageSnapshot, error) {
	date := freeTokenNowDate()
	snap := PaidUserTokenUsageSnapshot{Date: date}
	cfg, err := GetRateLimitConfig()
	if err != nil {
		return snap, err
	}
	snap.LimitM = cfg.PaidUserTokenLimitM
	used, err := GetPaidUserDailyTokenUsage(date)
	if err != nil {
		return snap, err
	}
	snap.TokensUsed = used
	snap.UsedM = float64(used) / float64(FreeUserTokenMUnit)
	return snap, nil
}

// CleanupOldPaidUserDailyTokenUsage deletes rows whose date < (today - keepDays).
// 与其他每日聚合表保持一致的 100 天保留窗。
// 关键词: CleanupOldPaidUserDailyTokenUsage, Unscoped 硬删除
func CleanupOldPaidUserDailyTokenUsage(keepDays int) (int64, error) {
	if keepDays <= 0 {
		keepDays = 100
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	tx := paidTokenDB().Where("date < ?", cutoff).Delete(&PaidUserDailyTokenUsage{})
	if tx.Error != nil {
		return 0, fmt.Errorf("CleanupOldPaidUserDailyTokenUsage failed: %v", tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Infof("CleanupOldPaidUserDailyTokenUsage removed %d rows older than %s", tx.RowsAffected, cutoff)
	}
	return tx.RowsAffected, nil
}
