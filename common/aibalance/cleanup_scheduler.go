package aibalance

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// StatsRetentionDays 是 ai_daily_cache_stats / ai_daily_user_seen 的保留窗口。
// 超过这个窗口的行会被 daily 清理任务删除，
// ai_daily_summary 表（每天 1 行）不参与清理。
// 关键词: StatsRetentionDays, 100 天保留窗
const StatsRetentionDays = 100

// runCleanupOnce 同步执行一次 daily 清理：
//   - DELETE ai_daily_cache_stats WHERE date < today-StatsRetentionDays
//   - DELETE ai_daily_user_seen   WHERE date < today-StatsRetentionDays
//   - ai_daily_summary 不动（每天 1 行，长期保留）
//
// 返回两张表分别清掉的行数。任意一表清理失败会被 Warn 日志吞掉，不阻塞另一张。
// 关键词: runCleanupOnce, daily cleanup, 100 天清理
func runCleanupOnce(keepDays int) (int64, int64) {
	if keepDays <= 0 {
		keepDays = StatsRetentionDays
	}
	cacheRows, err := CleanupOldCacheStats(keepDays)
	if err != nil {
		log.Warnf("daily cleanup CleanupOldCacheStats failed: %v", err)
	}
	userRows, err := CleanupOldUserSeen(keepDays)
	if err != nil {
		log.Warnf("daily cleanup CleanupOldUserSeen failed: %v", err)
	}
	log.Infof("daily cleanup done: cache_stats_removed=%d user_seen_removed=%d keep_days=%d",
		cacheRows, userRows, keepDays)
	return cacheRows, userRows
}

// nextCleanupAt 计算「下一次 0:01」的时间点（基于 now 的当前时区）。
// 关键词: nextCleanupAt, 0:01 每日触发
func nextCleanupAt(now time.Time) time.Time {
	target := time.Date(now.Year(), now.Month(), now.Day(), 0, 1, 0, 0, now.Location())
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target
}

// StartDailyCleanupScheduler 启动后台 goroutine，每天 0:01 触发一次 cleanup。
// 进程关闭通过 ctx.Done() 退出。
// 关键词: StartDailyCleanupScheduler, 每日 0:01 调度
func StartDailyCleanupScheduler(ctx context.Context) {
	go func() {
		log.Infof("daily cleanup scheduler started: keep_days=%d", StatsRetentionDays)
		for {
			next := nextCleanupAt(time.Now())
			d := time.Until(next)
			if d < 0 {
				d = time.Minute
			}
			select {
			case <-ctx.Done():
				log.Infof("daily cleanup scheduler stopped")
				return
			case <-time.After(d):
				runCleanupOnce(StatsRetentionDays)
			}
		}
	}()
}
