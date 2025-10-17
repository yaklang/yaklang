package asynchelper

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

var defaultLogLevel = false

type AsyncPerformanceHelper struct {
	processPrefix string

	triggerTime    time.Duration
	logRequireTime time.Duration
	ctx            context.Context
	cancel         context.CancelFunc

	status            string
	statusToDuration  *omap.OrderedMap[string, []time.Duration]
	currentStatusMark string

	markToTime   *omap.OrderedMap[string, time.Time]
	errorLogMode bool

	mockLog func(string, ...any)
}

type Option func(*AsyncPerformanceHelper)

func WithTriggerTime(d time.Duration) Option {
	return func(a *AsyncPerformanceHelper) {
		a.triggerTime = d
	}
}

func WithLogRequireTime(d time.Duration) Option {
	return func(a *AsyncPerformanceHelper) {
		a.logRequireTime = d
	}
}

func WithCtx(ctx context.Context) Option {
	return func(a *AsyncPerformanceHelper) {
		a.ctx, a.cancel = context.WithCancel(ctx)
	}
}

func WithErrorLogMode(errorLogMode bool) Option {
	return func(a *AsyncPerformanceHelper) {
		a.errorLogMode = errorLogMode
	}
}

func withMockLog(mockLog func(string, ...any)) Option {
	return func(a *AsyncPerformanceHelper) {
		a.mockLog = mockLog
	}
}

func NewAsyncPerformanceHelper(prefix string, opts ...Option) *AsyncPerformanceHelper {
	performanceHelper := &AsyncPerformanceHelper{
		triggerTime:    2 * time.Second,
		logRequireTime: 4 * time.Second,
		errorLogMode:   false,
		processPrefix:  prefix,
		markToTime:     omap.NewOrderedMap[string, time.Time](make(map[string]time.Time)),
	}

	for _, opt := range opts {
		opt(performanceHelper)
	}
	if performanceHelper.ctx == nil {
		performanceHelper.ctx, performanceHelper.cancel = context.WithCancel(context.Background())
	}

	return performanceHelper
}

func (a *AsyncPerformanceHelper) _loop() {
	startTime := time.Now()
	go func() {
		checkAndLog := func() {
			useTime := time.Since(startTime)
			if useTime > a.logRequireTime {
				currentStartTime, ok := a.markToTime.Get(a.currentStatusMark)
				if !ok {
					a.Logf("[%s]: took too long: %v, current status: %s", a.processPrefix, useTime, a.status)
				} else {
					a.Logf("[%s]: took too long: %v, current status: %s use time: %v", a.processPrefix, useTime, a.status, time.Since(currentStartTime))
				}

			}
		}
		for {
			select {
			case <-a.ctx.Done():
				checkAndLog()
				return
			case <-time.After(a.triggerTime):
				checkAndLog()
			}
		}
	}()
}

func (a *AsyncPerformanceHelper) DumpStatusDurations(status string) {
	durations, ok := a.statusToDuration.Get(status)
	if !ok || len(durations) == 0 {
		a.Logf("No durations recorded for status: %s", status)
		return
	}

	// Copy and sort durations descending
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })

	// Top 3
	topN := 3
	if len(sorted) < topN {
		topN = len(sorted)
	}
	topDurations := sorted[:topN]

	// Average
	var total time.Duration
	for _, d := range durations {
		total += d
	}
	avg := time.Duration(0)
	if len(durations) > 0 {
		avg = total / time.Duration(len(durations))
	}

	// Table with prefix and status in title
	title := fmt.Sprintf(" Durations for [%s] status [%s] ", a.processPrefix, status)
	border := "+" + string(make([]byte, len(title)+2))
	for i := range border {
		border = border[:i] + "-" + border[i+1:]
	}
	table := ""
	table += border + "\n"
	table += "|" + title + "|\n"
	table += border + "\n"
	table += "+-------+-------------------+\n"
	table += "| Rank  | Duration          |\n"
	table += "+-------+-------------------+\n"
	for i, d := range topDurations {
		table += fmt.Sprintf("| Top%-2d | %-17s |\n", i+1, d.String())
	}
	table += "+-------+-------------------+\n"
	table += fmt.Sprintf("| Avg   | %-17s |\n", avg.String())
	table += "+-------+-------------------+\n"
	table += fmt.Sprintf("| Count | %-17d |\n", len(durations))
	table += "+-------+-------------------+\n"

	a.Logf("Status [%s] durations:\n%s", status, table)
}

func (a *AsyncPerformanceHelper) DumpAllStatus() {
	if a.statusToDuration == nil || a.statusToDuration.Len() == 0 {
		a.Logf("No status durations recorded for process: %s", a.processPrefix)
		return
	}

	type stat struct {
		status string
		total  time.Duration
		count  int
		avg    time.Duration
	}

	var stats []stat
	maxStatusLen := 6 // for header
	a.statusToDuration.ForEach(func(status string, durations []time.Duration) bool {
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		count := len(durations)
		avg := time.Duration(0)
		if count > 0 {
			avg = total / time.Duration(count)
		}
		stats = append(stats, stat{status, total, count, avg})
		if len(status) > maxStatusLen {
			maxStatusLen = len(status)
		}
		return true
	})

	title := fmt.Sprintf(" All Status Summary for [%s] ", a.processPrefix)
	border := "+" + string(make([]byte, len(title)+2))
	for i := range border {
		border = border[:i] + "-" + border[i+1:]
	}

	// Table header
	table := ""
	table += border + "\n"
	table += "|" + title + "|\n"
	table += border + "\n"
	table += fmt.Sprintf("+-%-*s-+------------+-------+------------+\n", maxStatusLen, "------")
	table += fmt.Sprintf("| %-*s | Total      | Count | Average    |\n", maxStatusLen, "Status")
	table += fmt.Sprintf("+-%-*s-+------------+-------+------------+\n", maxStatusLen, "------")
	for _, s := range stats {
		table += fmt.Sprintf("| %-*s | %-10s | %-5d | %-10s |\n", maxStatusLen, s.status, s.total.String(), s.count, s.avg.String())
	}
	table += fmt.Sprintf("+-%-*s-+------------+-------+------------+\n", maxStatusLen, "------")

	a.Logf("All status durations:\n%s", table)
}

// Logf logs a message using either the mockLog function (if set) or the standard log functions, depending on errorLogMode.
func (a *AsyncPerformanceHelper) Logf(format string, args ...interface{}) {
	if a.mockLog != nil {
		a.mockLog(format, args...)
		return
	}

	if a.errorLogMode {
		log.Errorf(format, args...)
	} else {
		log.Infof(format, args...)
	}

}

// SetStatus updates the status field of the helper.
func (a *AsyncPerformanceHelper) SetStatus(status string) {
	// collect the duration for the previous status
	durationList, ok := a.statusToDuration.Get(a.status)
	if !ok {
		durationList = make([]time.Duration, 0)
	}
	durationList = append(durationList, time.Since(a.markToTime.GetMust(a.currentStatusMark)))
	a.statusToDuration.Set(a.status, durationList)
	a.status = status
	a.currentStatusMark = a.MarkNow()
}

// Close triggers the close function if it is set, used to stop the background goroutine.
func (a *AsyncPerformanceHelper) Close() {
	a.cancel()
}

// MarkNow creates a new time mark with a unique ID and returns the mark string.
func (a *AsyncPerformanceHelper) MarkNow() string {
	mark := uuid.NewString()
	a.markToTime.Set(mark, time.Now())
	return mark
}

// CheckLastMark1Second checks if the last mark exceeds one second and logs if necessary.
func (a *AsyncPerformanceHelper) CheckLastMark1Second(verbose string) time.Duration {
	return a.CheckLastMarkAndLog(time.Second, verbose)
}

// CheckLastMarkDefaultConfig checks if the last mark exceeds the configured logRequireTime and logs if necessary.
func (a *AsyncPerformanceHelper) CheckLastMarkDefaultConfig(verbose string) time.Duration {
	return a.CheckLastMarkAndLog(a.logRequireTime, verbose)
}

// CheckLastMarkAndLog checks the last mark against a custom time condition and logs if exceeded.
func (a *AsyncPerformanceHelper) CheckLastMarkAndLog(timeCondition time.Duration, verbose string) time.Duration {
	_, startTime, ok := a.markToTime.Last()
	if !ok {
		log.Errorf("no mark found")
		return 0
	}
	useTime := time.Since(startTime)
	if useTime > timeCondition {
		a.Logf("[%s]: took too long: %v, require time %s , current status: %s, verbose: %s", a.processPrefix, useTime, timeCondition, a.status, verbose)
	}
	return useTime
}

// CheckMarkAndLog checks a specific mark against a custom time condition and logs if exceeded.
func (a *AsyncPerformanceHelper) CheckMarkAndLog(mark string, timeCondition time.Duration, verbose string) time.Duration {
	startTime, ok := a.markToTime.Get(mark)
	if !ok {
		log.Errorf("mark %s not found", mark)
		return 0
	}
	useTime := time.Since(startTime)
	if useTime > timeCondition {
		a.Logf("[%s]: took too long: %v, require time %s ,  current status: %s, verbose: %s", a.processPrefix, useTime, timeCondition, a.status, verbose)
	}
	return useTime
}
