package aicache

import (
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// minPrintInterval 是节流打印的最小间隔
// 关键词: aicache, minPrintInterval, 节流间隔
const minPrintInterval = 3 * time.Second

// printerLogFunc 是日志输出函数指针，便于测试时替换
// 关键词: aicache, printerLogFunc, 可注入日志
var printerLogFunc = func(format string, args ...any) {
	log.Infof(format, args...)
}

// throttlePrinter 把"变动驱动 + 最低 3s 一次"的逻辑封装在一个状态机里
// 关键词: aicache, throttlePrinter, 节流打印器
type throttlePrinter struct {
	mu          sync.Mutex
	lastPrinted *HitReport
	lastPrintAt time.Time
	pending     *HitReport
	timer       *time.Timer
	interval    time.Duration
}

// newThrottlePrinter 构造一个新的 throttlePrinter
// 关键词: aicache, newThrottlePrinter
func newThrottlePrinter(interval time.Duration) *throttlePrinter {
	if interval <= 0 {
		interval = minPrintInterval
	}
	return &throttlePrinter{interval: interval}
}

// Trigger 提交一次新报告供节流打印决策
//   - 若与上次已打印的报告全等：直接丢弃
//   - 否则：满足 3s 间隔则立即打印；否则延迟到 3s 时再打印（保留最新一份）
//
// 关键词: aicache, Trigger, 节流提交
func (p *throttlePrinter) Trigger(rep *HitReport) {
	if p == nil || rep == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.lastPrinted != nil && p.lastPrinted.Equal(rep) {
		// 与上次打印完全一致，丢弃；同时清掉排队中的 pending（因为局势已经追平）
		p.pending = nil
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
		return
	}

	now := time.Now()
	elapsed := now.Sub(p.lastPrintAt)
	if p.lastPrintAt.IsZero() || elapsed >= p.interval {
		p.printLocked(rep, now)
		return
	}

	p.pending = rep
	if p.timer == nil {
		wait := p.interval - elapsed
		if wait <= 0 {
			wait = time.Millisecond
		}
		p.timer = time.AfterFunc(wait, p.flush)
	}
}

// flush 由 timer 在 3s 兜底时回调
// 关键词: aicache, flush, 节流兜底
func (p *throttlePrinter) flush() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.timer = nil
	if p.pending == nil {
		return
	}
	if p.lastPrinted != nil && p.lastPrinted.Equal(p.pending) {
		p.pending = nil
		return
	}
	p.printLocked(p.pending, time.Now())
	p.pending = nil
}

// printLocked 实际打印一行；调用方必须持锁
// 关键词: aicache, printLocked, 单行打印
func (p *throttlePrinter) printLocked(rep *HitReport, now time.Time) {
	line := formatHitReportLine(rep)
	printerLogFunc("%s", line)
	p.lastPrinted = rep
	p.lastPrintAt = now
}

// formatHitReportLine 把 HitReport 序列化成单行英文日志
// 关键词: aicache, formatHitReportLine
func formatHitReportLine(rep *HitReport) string {
	if rep == nil {
		return "[aicache] <nil report>"
	}
	advice := FirstAdvice(rep)
	model := rep.Model
	if model == "" {
		model = "-"
	}
	if advice == "" {
		return fmt.Sprintf(
			"[aicache] reqs=%d model=%s chunks=%d prefix_hit=%d/%d(%.1f%%) bytes=%d/%d cache_uniq=%d cache_bytes=%d",
			rep.TotalRequests, model, rep.RequestChunks,
			rep.PrefixHitChunks, rep.RequestChunks, rep.PrefixHitRatio*100,
			rep.PrefixHitBytes, rep.RequestBytes,
			rep.GlobalUniqueChunks, rep.GlobalCacheBytes,
		)
	}
	return fmt.Sprintf(
		"[aicache] reqs=%d model=%s chunks=%d prefix_hit=%d/%d(%.1f%%) bytes=%d/%d cache_uniq=%d cache_bytes=%d advice=%q",
		rep.TotalRequests, model, rep.RequestChunks,
		rep.PrefixHitChunks, rep.RequestChunks, rep.PrefixHitRatio*100,
		rep.PrefixHitBytes, rep.RequestBytes,
		rep.GlobalUniqueChunks, rep.GlobalCacheBytes,
		advice,
	)
}
