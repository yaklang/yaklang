package aicache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 关键词: aicache, printer, 相同报告不重复打印
func TestPrinter_NoChangeNoPrint(t *testing.T) {
	lines := captureLogLines()
	defer lines.restore()

	p := newThrottlePrinter(50 * time.Millisecond)
	rep := &HitReport{Model: "m", RequestChunks: 4, RequestBytes: 100, PrefixHitChunks: 4, PrefixHitBytes: 100, TotalRequests: 1}

	for i := 0; i < 5; i++ {
		p.Trigger(rep)
	}

	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int64(1), lines.count(), "identical reports should print only once")
}

// 关键词: aicache, printer, 节流间隔
func TestPrinter_ThrottleIntervalCollapsesPending(t *testing.T) {
	lines := captureLogLines()
	defer lines.restore()

	p := newThrottlePrinter(80 * time.Millisecond)

	// 第一次立即打印
	p.Trigger(&HitReport{Model: "m", RequestChunks: 4, RequestBytes: 100, PrefixHitChunks: 0, TotalRequests: 1})
	require.Equal(t, int64(1), lines.count())

	// 紧接着 8 次不同报告，期望被合并
	for i := 0; i < 8; i++ {
		p.Trigger(&HitReport{
			Model:           "m",
			RequestChunks:   4,
			RequestBytes:    100 + i,
			PrefixHitChunks: i,
			TotalRequests:   int64(i + 2),
		})
	}

	// 间隔到点之后应该再打印 1 次（最后一个 pending）
	time.Sleep(200 * time.Millisecond)
	c := lines.count()
	assert.GreaterOrEqual(t, c, int64(2), "should have printed pending after interval")
	assert.LessOrEqual(t, c, int64(2), "throttling should collapse all pending reports into a single trailing print")
}

// 关键词: aicache, printer, 单行格式
func TestFormatHitReportLine(t *testing.T) {
	rep := &HitReport{
		Model:              "qwen-plus",
		RequestChunks:      4,
		RequestBytes:       16384,
		PrefixHitChunks:    3,
		PrefixHitBytes:     12345,
		PrefixHitRatio:     0.75,
		GlobalUniqueChunks: 18,
		GlobalCacheBytes:   89211,
		TotalRequests:      12,
	}
	line := formatHitReportLine(rep)
	assert.Contains(t, line, "[aicache]")
	assert.Contains(t, line, "model=qwen-plus")
	assert.Contains(t, line, "chunks=4")
	assert.Contains(t, line, "prefix_hit=3/4")
	assert.Contains(t, line, "75.0%")
	assert.Contains(t, line, "bytes=12345/16384")
	assert.Contains(t, line, "cache_uniq=18")
	assert.Contains(t, line, "cache_bytes=89211")

	rep.Advices = []string{"high-static unstable: 3 hashes"}
	line = formatHitReportLine(rep)
	assert.Contains(t, line, "advice=")
	assert.Contains(t, line, "high-static unstable: 3 hashes")
}

// captureLogLines 替换 printerLogFunc，记录所有打印行
// 关键词: aicache, test helper, log capture
type capturedLogs struct {
	prev func(string, ...any)
	mu   sync.Mutex
	cnt  *atomic.Int64
}

func (c *capturedLogs) restore() {
	printerLogFunc = c.prev
}

func (c *capturedLogs) count() int64 {
	return c.cnt.Load()
}

func captureLogLines() *capturedLogs {
	c := &capturedLogs{
		prev: printerLogFunc,
		cnt:  &atomic.Int64{},
	}
	printerLogFunc = func(format string, args ...any) {
		_ = fmt.Sprintf(format, args...)
		c.cnt.Add(1)
	}
	return c
}
