package aibalance

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// userSeenDailyCap 限制 ai_daily_user_seen 表每天每 source_kind 写入的最大行数，
// 用以防止恶意 / 异常情况下 DB 行数爆炸。超过此阈值后当天该 source_kind 的
// 后续 RecordDailyUserSeen 调用会被静默 drop（统计口径退化但 DB 安全）。
// 关键词: ai_daily_user_seen cap, DAU 防爆, 1M 行硬上限
const userSeenDailyCap int64 = 1_000_000

// SourceKind* 三类 sourceKind 取值常量。
// 关键词: source_kind, api_key, free_trace, free_ip
const (
	SourceKindAPIKey    = "api_key"
	SourceKindFreeTrace = "free_trace"
	SourceKindFreeIP    = "free_ip"
)

// userSeenCounters 记录每天每 source_kind 已经成功 INSERT 的行数（近似），
// 用作 cap 判定。重启后归零（计数重新开始，cap 仍能保护本进程内的爆量）。
// 关键词: userSeenCounters, 每日防爆计数器
var userSeenCounters sync.Map // key: "date|sourceKind" -> *int64

func userSeenCounterKey(date, sourceKind string) string {
	return date + "|" + sourceKind
}

func loadOrCreateUserSeenCounter(date, sourceKind string) *int64 {
	key := userSeenCounterKey(date, sourceKind)
	if v, ok := userSeenCounters.Load(key); ok {
		return v.(*int64)
	}
	counter := new(int64)
	actual, _ := userSeenCounters.LoadOrStore(key, counter)
	return actual.(*int64)
}

// resetUserSeenCounters 清空内存计数器，仅供测试使用。
// 关键词: resetUserSeenCounters, 测试入口
func resetUserSeenCounters() {
	userSeenCounters = sync.Map{}
}

// userSeenDB 跳过 GORM 软删除过滤；ai_daily_user_seen 是聚合去重表，
// 没有「软删除-恢复」语义，直接对实际行操作。
// 关键词: userSeenDB, GORM Unscoped, 跳过软删除
func userSeenDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsureUserSeenTable ensures the ai_daily_user_seen table exists.
// 关键词: ai_daily_user_seen migrate
func EnsureUserSeenTable() error {
	return GetDB().AutoMigrate(&schema.AiDailyUserSeen{}).Error
}

// RecordDailyUserSeen registers one user fingerprint for the given date / source_kind.
// 重复指纹不增加新行（INSERT IGNORE 语义），但会刷新 last_seen_at；
// 当当天该 source_kind 的写入行数超过 userSeenDailyCap 时，函数直接返回 nil
// 不写 DB，并以 Warn 级别记录第一次触发 cap 时的日志。
// 关键词: RecordDailyUserSeen, INSERT IGNORE 语义, cap 防爆, last_seen_at 刷新
func RecordDailyUserSeen(date, sourceKind, userHash string) error {
	if date == "" || sourceKind == "" || userHash == "" {
		return fmt.Errorf("RecordDailyUserSeen: date/sourceKind/userHash must be non-empty")
	}

	db := userSeenDB()

	var existing schema.AiDailyUserSeen
	err := db.Where("date = ? AND source_kind = ? AND user_hash = ?", date, sourceKind, userHash).
		First(&existing).Error
	if err == nil {
		return db.Model(&schema.AiDailyUserSeen{}).
			Where("id = ?", existing.ID).
			UpdateColumn("last_seen_at", time.Now()).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("RecordDailyUserSeen query failed: %v", err)
	}

	counter := loadOrCreateUserSeenCounter(date, sourceKind)
	if atomic.LoadInt64(counter) >= userSeenDailyCap {
		// 超过当日阈值，直接 drop。日志只在第一次触发 cap 时发，避免刷屏。
		if atomic.CompareAndSwapInt64(counter, userSeenDailyCap, userSeenDailyCap+1) {
			log.Warnf("ai_daily_user_seen cap reached: date=%s source_kind=%s cap=%d (subsequent inserts dropped silently)",
				date, sourceKind, userSeenDailyCap)
		}
		return nil
	}

	row := schema.AiDailyUserSeen{
		Date:       date,
		SourceKind: sourceKind,
		UserHash:   userHash,
		LastSeenAt: time.Now(),
	}
	if createErr := db.Create(&row).Error; createErr != nil {
		var dup schema.AiDailyUserSeen
		if findErr := db.Where("date = ? AND source_kind = ? AND user_hash = ?", date, sourceKind, userHash).
			First(&dup).Error; findErr == nil {
			return db.Model(&schema.AiDailyUserSeen{}).
				Where("id = ?", dup.ID).
				UpdateColumn("last_seen_at", time.Now()).Error
		}
		return fmt.Errorf("RecordDailyUserSeen create failed: %v", createErr)
	}
	atomic.AddInt64(counter, 1)
	return nil
}

// DAUDay 是「某一天 DAU 拆分」聚合行：包含 4 个数字（按 source_kind 拆 + total）。
// 关键词: DAUDay, 日活拆分, total = sum source_kind
type DAUDay struct {
	Date       string `json:"date"`
	APIKey     int64  `json:"api_key"`
	FreeTrace  int64  `json:"free_trace"`
	FreeIP     int64  `json:"free_ip"`
	Total      int64  `json:"total"`
}

// QueryDAU60Days returns last 60 calendar days (today inclusive) of DAU per source_kind,
// missing dates filled with zero rows.
// 关键词: QueryDAU60Days, COUNT DISTINCT user_hash, GROUP BY date source_kind
func QueryDAU60Days() ([]*DAUDay, error) {
	return QueryDAUDays(60, time.Now())
}

// QueryDAUDays is the test-friendly variant of QueryDAU60Days.
// 关键词: QueryDAUDays
func QueryDAUDays(days int, end time.Time) ([]*DAUDay, error) {
	if days <= 0 {
		days = 60
	}
	startDate := end.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	type row struct {
		Date       string
		SourceKind string
		Cnt        int64
	}
	var rows []row
	if err := userSeenDB().Table((&schema.AiDailyUserSeen{}).TableName()).
		Select("date, source_kind, COUNT(DISTINCT user_hash) AS cnt").
		Where("date >= ?", startDate).
		Group("date, source_kind").
		Order("date ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("QueryDAUDays failed: %v", err)
	}

	idx := make(map[string]*DAUDay, days)
	for i := 0; i < days; i++ {
		d := end.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		idx[d] = &DAUDay{Date: d}
	}
	for _, r := range rows {
		dst, ok := idx[r.Date]
		if !ok {
			continue
		}
		switch r.SourceKind {
		case SourceKindAPIKey:
			dst.APIKey += r.Cnt
		case SourceKindFreeTrace:
			dst.FreeTrace += r.Cnt
		case SourceKindFreeIP:
			dst.FreeIP += r.Cnt
		}
		dst.Total = dst.APIKey + dst.FreeTrace + dst.FreeIP
	}

	out := make([]*DAUDay, 0, days)
	for i := 0; i < days; i++ {
		d := end.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		out = append(out, idx[d])
	}
	return out, nil
}

// QueryTodayDAUTotal returns today's total DAU across all source_kinds.
// 关键词: QueryTodayDAUTotal, 单数字卡片
func QueryTodayDAUTotal() (int64, error) {
	return QueryDAUTotalByDate(time.Now().Format("2006-01-02"))
}

// QueryDAUTotalByDate is the test-friendly variant of QueryTodayDAUTotal.
// 关键词: QueryDAUTotalByDate
func QueryDAUTotalByDate(date string) (int64, error) {
	var total int64
	err := userSeenDB().Table((&schema.AiDailyUserSeen{}).TableName()).
		Where("date = ?", date).
		Select("COUNT(DISTINCT (source_kind || '|' || user_hash))").
		Row().Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("QueryDAUTotalByDate failed: %v", err)
	}
	return total, nil
}

// CleanupOldUserSeen deletes ai_daily_user_seen rows whose date < (today - keepDays).
// 用 Unscoped() 硬删除，避免软删除残留行触发 unique 约束冲突。
// 关键词: CleanupOldUserSeen, 100 天保留, Unscoped 硬删除
func CleanupOldUserSeen(keepDays int) (int64, error) {
	if keepDays <= 0 {
		keepDays = 100
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	tx := GetDB().Unscoped().Where("date < ?", cutoff).Delete(&schema.AiDailyUserSeen{})
	if tx.Error != nil {
		return 0, fmt.Errorf("CleanupOldUserSeen failed: %v", tx.Error)
	}
	if tx.RowsAffected > 0 {
		log.Infof("CleanupOldUserSeen removed %d rows older than %s", tx.RowsAffected, cutoff)
	}
	return tx.RowsAffected, nil
}
