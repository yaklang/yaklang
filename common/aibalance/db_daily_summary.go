package aibalance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// dailySummaryAccumulator 是「当天」内存聚合器。
// chat hot path 直接 atomic 累加，由后台 goroutine 每 30s flush 到 DB。
// 当跨过自然日时，getOrSwapAccumulator 会先把上一天 flush 干净再切到新一天，
// 避免数据落到错误的 date row。
// 关键词: dailySummaryAccumulator, 内存聚合 + 后台 flush
type dailySummaryAccumulator struct {
	date             string
	totalRequests    int64
	promptTokens     int64
	completionTokens int64
	cachedTokens     int64
}

// dailySummarySwapMu 仅串行化「日切换 + flush」，不参与单请求的 atomic 累加路径。
// 关键词: dailySummarySwapMu, 自然日切换互斥
var (
	dailySummarySwapMu  sync.Mutex
	dailySummaryCurrent atomic.Pointer[dailySummaryAccumulator]
)

// dailySummaryDB 跳过 GORM 软删除过滤；ai_daily_summary 是聚合快照表，
// 没有「软删除-恢复」语义，全部用 Unscoped 直接对实际行操作。
// 关键词: dailySummaryDB, GORM Unscoped, 跳过软删除
func dailySummaryDB() *gorm.DB {
	return GetDB().Unscoped()
}

// EnsureSummaryTable ensures the ai_daily_summary table exists.
// 关键词: ai_daily_summary migrate
func EnsureSummaryTable() error {
	return GetDB().AutoMigrate(&schema.AiDailySummary{}).Error
}

// resetDailySummaryAccumulator 清空内存累加器，仅供测试使用。
// 关键词: resetDailySummaryAccumulator, 测试入口
func resetDailySummaryAccumulator() {
	dailySummarySwapMu.Lock()
	defer dailySummarySwapMu.Unlock()
	dailySummaryCurrent.Store(nil)
}

// nowDateString 抽出来便于测试 mock。
// 关键词: nowDateString, 自然日字符串
var nowDateString = func() string {
	return time.Now().Format("2006-01-02")
}

func getOrSwapAccumulator(today string) *dailySummaryAccumulator {
	cur := dailySummaryCurrent.Load()
	if cur != nil && cur.date == today {
		return cur
	}
	dailySummarySwapMu.Lock()
	defer dailySummarySwapMu.Unlock()
	cur = dailySummaryCurrent.Load()
	if cur != nil && cur.date == today {
		return cur
	}
	if cur != nil && cur.date != today {
		// 跨自然日：先把上一天残余落库，再创建今天的累加器。
		if err := flushAccumulator(cur); err != nil {
			log.Warnf("flushAccumulator on day rollover failed: %v", err)
		}
	}
	next := &dailySummaryAccumulator{date: today}
	dailySummaryCurrent.Store(next)
	return next
}

// RecordDailySummaryDelta accumulates one request's usage into today's in-memory counter.
// 永远不阻塞、不返回错误：失败仅靠后台 flush 重试。
// 关键词: RecordDailySummaryDelta, atomic 累加, hot path 无 DB 写
func RecordDailySummaryDelta(usage *aispec.ChatUsage) {
	today := nowDateString()
	acc := getOrSwapAccumulator(today)
	atomic.AddInt64(&acc.totalRequests, 1)
	if usage == nil {
		return
	}
	if usage.PromptTokens > 0 {
		atomic.AddInt64(&acc.promptTokens, int64(usage.PromptTokens))
	}
	if usage.CompletionTokens > 0 {
		atomic.AddInt64(&acc.completionTokens, int64(usage.CompletionTokens))
	}
	if usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > 0 {
		atomic.AddInt64(&acc.cachedTokens, int64(usage.PromptTokensDetails.CachedTokens))
	}
}

// flushAccumulator atomically takes a snapshot of the accumulator (resetting it to 0)
// and UPSERTs the delta into ai_daily_summary for that date.
// 关键词: flushAccumulator, swap-then-add, UPSERT 累加
func flushAccumulator(acc *dailySummaryAccumulator) error {
	if acc == nil {
		return nil
	}
	reqs := atomic.SwapInt64(&acc.totalRequests, 0)
	prompt := atomic.SwapInt64(&acc.promptTokens, 0)
	completion := atomic.SwapInt64(&acc.completionTokens, 0)
	cached := atomic.SwapInt64(&acc.cachedTokens, 0)
	if reqs == 0 && prompt == 0 && completion == 0 && cached == 0 {
		return nil
	}
	if err := upsertDailySummaryDelta(acc.date, reqs, prompt, completion, cached); err != nil {
		atomic.AddInt64(&acc.totalRequests, reqs)
		atomic.AddInt64(&acc.promptTokens, prompt)
		atomic.AddInt64(&acc.completionTokens, completion)
		atomic.AddInt64(&acc.cachedTokens, cached)
		return err
	}
	return nil
}

// flushSummaryAccumulator 触发一次手动 flush（无后台 goroutine 也可调用）。
// 关键词: flushSummaryAccumulator, 手动触发落库
func flushSummaryAccumulator() error {
	cur := dailySummaryCurrent.Load()
	if cur == nil {
		return nil
	}
	return flushAccumulator(cur)
}

// upsertDailySummaryDelta UPSERT-累加一行 ai_daily_summary。
// 关键词: upsertDailySummaryDelta, gorm.Expr 累加
func upsertDailySummaryDelta(date string, reqs, prompt, completion, cached int64) error {
	if date == "" {
		return fmt.Errorf("upsertDailySummaryDelta: date is empty")
	}
	db := dailySummaryDB()
	var row schema.AiDailySummary
	err := db.Where("date = ?", date).First(&row).Error
	if err == nil {
		return db.Model(&schema.AiDailySummary{}).
			Where("id = ?", row.ID).
			UpdateColumns(map[string]interface{}{
				"total_requests":    gorm.Expr("total_requests + ?", reqs),
				"prompt_tokens":     gorm.Expr("prompt_tokens + ?", prompt),
				"completion_tokens": gorm.Expr("completion_tokens + ?", completion),
				"cached_tokens":     gorm.Expr("cached_tokens + ?", cached),
			}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("upsertDailySummaryDelta query failed: %v", err)
	}

	row = schema.AiDailySummary{
		Date:             date,
		TotalRequests:    reqs,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		CachedTokens:     cached,
	}
	if createErr := db.Create(&row).Error; createErr != nil {
		var existing schema.AiDailySummary
		if findErr := db.Where("date = ?", date).First(&existing).Error; findErr == nil {
			return db.Model(&schema.AiDailySummary{}).
				Where("id = ?", existing.ID).
				UpdateColumns(map[string]interface{}{
					"total_requests":    gorm.Expr("total_requests + ?", reqs),
					"prompt_tokens":     gorm.Expr("prompt_tokens + ?", prompt),
					"completion_tokens": gorm.Expr("completion_tokens + ?", completion),
					"cached_tokens":     gorm.Expr("cached_tokens + ?", cached),
				}).Error
		}
		return fmt.Errorf("upsertDailySummaryDelta create failed: %v", createErr)
	}
	return nil
}

// StartDailySummaryFlusher 启动后台 goroutine，每 interval 触发一次 flush。
// 进程退出时通过 ctx.Done() 收尾，并尝试做最后一次 flush。
// 关键词: StartDailySummaryFlusher, 后台 30s tick
func StartDailySummaryFlusher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				if err := flushSummaryAccumulator(); err != nil {
					log.Warnf("StartDailySummaryFlusher final flush failed: %v", err)
				}
				log.Infof("daily summary flusher stopped")
				return
			case <-ticker.C:
				if err := flushSummaryAccumulator(); err != nil {
					log.Warnf("daily summary periodic flush failed: %v", err)
				}
			}
		}
	}()
}

// QuerySummary60Days returns last 60 calendar days (today inclusive),
// missing dates filled with zero rows.
// 关键词: QuerySummary60Days, 缺失日期补 0
func QuerySummary60Days() ([]*schema.AiDailySummary, error) {
	return QuerySummaryDays(60, time.Now())
}

// QuerySummaryDays is the test-friendly variant of QuerySummary60Days.
// 关键词: QuerySummaryDays
func QuerySummaryDays(days int, end time.Time) ([]*schema.AiDailySummary, error) {
	if days <= 0 {
		days = 60
	}
	startDate := end.AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	var rows []*schema.AiDailySummary
	if err := dailySummaryDB().Where("date >= ?", startDate).
		Order("date ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("QuerySummaryDays failed: %v", err)
	}
	idx := make(map[string]*schema.AiDailySummary, len(rows))
	for _, r := range rows {
		idx[r.Date] = r
	}
	out := make([]*schema.AiDailySummary, 0, days)
	for i := 0; i < days; i++ {
		d := end.AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		if r, ok := idx[d]; ok {
			out = append(out, r)
		} else {
			out = append(out, &schema.AiDailySummary{Date: d})
		}
	}
	return out, nil
}
