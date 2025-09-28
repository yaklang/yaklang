package asynchelper

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

var defaultLogLevel = false

type AsyncPerformanceHelper struct {
	triggerTime       time.Duration
	logRequireTime    time.Duration
	processPrefix     string
	status            string
	currentStatusMark string
	timeMark          *omap.OrderedMap[string, time.Time]
	errorLogMode      bool
	closeFunc         func()
	mockLog           func(string, ...any)
}

// NewAsyncPerformanceHelper creates a new AsyncPerformanceHelper with custom period, log requirement time, prefix, and error log mode.
func NewAsyncPerformanceHelper(period time.Duration, logRequireTime time.Duration, prefix string, ErrorLogMode bool) *AsyncPerformanceHelper {
	performanceHelper := &AsyncPerformanceHelper{
		triggerTime:    period,
		logRequireTime: logRequireTime,
		errorLogMode:   ErrorLogMode,
		processPrefix:  prefix,
		timeMark:       omap.NewOrderedMap[string, time.Time](make(map[string]time.Time)),
	}
	return performanceHelper
}

// NewDefaultAsyncPerformanceHelper creates an AsyncPerformanceHelper with default settings: period of 2 seconds and log requirement time of 4 seconds.
func NewDefaultAsyncPerformanceHelper(prefix string) *AsyncPerformanceHelper {
	return NewAsyncPerformanceHelper(2*time.Second, 4*time.Second, prefix, defaultLogLevel)
}

// Log logs a message using either the mockLog function (if set) or the standard log functions, depending on errorLogMode.
func (a *AsyncPerformanceHelper) Log(fmtString string, arg ...any) {
	if a.mockLog != nil {
		a.mockLog(fmtString, arg...)
		return
	}

	if a.errorLogMode {
		log.Errorf(fmtString, arg...)
	} else {
		log.Infof(fmtString, arg...)
	}

}

// UpdateStatus updates the status field of the helper.
func (a *AsyncPerformanceHelper) UpdateStatus(status string) {
	a.status = status
	a.currentStatusMark = a.MarkNow()
}

// Close triggers the close function if it is set, used to stop the background goroutine.
func (a *AsyncPerformanceHelper) Close() {
	if a.closeFunc != nil {
		a.closeFunc()
	}
}

// Start begins the background goroutine that periodically checks and logs performance based on triggerTime and logRequireTime.
func (a *AsyncPerformanceHelper) Start() {
	closeChan := make(chan struct{})
	closeOnce := &sync.Once{}
	a.closeFunc = func() {
		closeOnce.Do(func() {
			close(closeChan)
		})
	}
	a.status = "default start"
	startTime := time.Now()
	go func() {
		checkAndLog := func() {
			useTime := time.Since(startTime)
			if useTime > a.logRequireTime {
				currentStartTime, ok := a.timeMark.Get(a.currentStatusMark)
				if !ok {
					a.Log("[%s]: took too long: %v, current status: %s", a.processPrefix, useTime, a.status)
				} else {
					a.Log("[%s]: took too long: %v, current status: %s, current status %s use time: %v", a.processPrefix, useTime, a.status, currentStartTime, time.Since(currentStartTime))
				}

			}
		}
		for {
			select {
			case <-closeChan:
				checkAndLog()
				return
			case <-time.After(a.triggerTime):
				checkAndLog()
			}
		}
	}()
}

// MarkNow creates a new time mark with a unique ID and returns the mark string.
func (a *AsyncPerformanceHelper) MarkNow() string {
	mark := uuid.NewString()
	a.timeMark.Set(mark, time.Now())
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
	_, startTime, ok := a.timeMark.Last()
	if !ok {
		log.Errorf("no mark found")
		return 0
	}
	useTime := time.Since(startTime)
	if useTime > timeCondition {
		a.Log("[%s]: took too long: %v, require time %s , current status: %s, verbose: %s", a.processPrefix, useTime, timeCondition, a.status, verbose)
	}
	return useTime
}

// CheckMarkAndLog checks a specific mark against a custom time condition and logs if exceeded.
func (a *AsyncPerformanceHelper) CheckMarkAndLog(mark string, timeCondition time.Duration, verbose string) time.Duration {
	startTime, ok := a.timeMark.Get(mark)
	if !ok {
		log.Errorf("mark %s not found", mark)
		return 0
	}
	useTime := time.Since(startTime)
	if useTime > timeCondition {
		a.Log("[%s]: took too long: %v, require time %s ,  current status: %s, verbose: %s", a.processPrefix, useTime, timeCondition, a.status, verbose)
	}
	return useTime
}
