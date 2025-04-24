package utils

import (
	"context"
	"time"
)

const DefaultTimeFormat = "2006_01_02-15_04_05"
const DefaultTimeFormat2 = "20060102_15_04_05"
const DefaultTimeFormat3 = "2006/01/02 15:04:05"
const DefaultDateFormat = "2006-01-02"

func TimestampNano() int64 {
	return time.Now().UnixNano()
}

func TimestampMs() int64 {
	return TimestampNano() / int64(time.Millisecond)
}

func TimestampSecond() int64 {
	return time.Now().Unix()
}

func DatetimePretty() string {
	return time.Now().Format(DefaultTimeFormat)
}

func DatetimePretty2() string {
	return time.Now().Format(DefaultTimeFormat2)
}

func DatePretty() string {
	return time.Now().Format(DefaultDateFormat)
}

func weekdayToOffsetWithMondayFirst(w time.Weekday) int {
	switch w {
	case time.Monday:
		return 0
	case time.Tuesday:
		return 1
	case time.Wednesday:
		return 2
	case time.Thursday:
		return 3
	case time.Friday:
		return 4
	case time.Saturday:
		return 5
	case time.Sunday:
		return 6
	default:
		return 0
	}
}

func weekdayToOffsetWithSundayFirst(w time.Weekday) int {
	offset := weekdayToOffsetWithMondayFirst(w) + 1
	if offset >= 7 {
		return 0
	} else {
		return offset
	}
}

func GetCurrentDate() (time.Time, error) {
	return GetDate(time.Now())
}

func GetDate(t time.Time) (time.Time, error) {
	dateFmt := "2006-01-02"
	return time.Parse(dateFmt, t.Format(dateFmt))
}

func GetCurrentWeekMonday() (time.Time, error) {
	nowDate, err := GetCurrentDate()
	if err != nil {
		return time.Time{}, Errorf("BUG: parse current date failed: %s", err)
	}

	return nowDate.Add(-time.Duration(weekdayToOffsetWithMondayFirst(nowDate.Weekday())) * 24 * time.Hour), nil
}

func GetWeekStartMonday(t time.Time) (time.Time, error) {
	nowDate, err := GetDate(t)
	if err != nil {
		return time.Time{}, Errorf("BUG: parse current date failed: %s", err)
	}

	result := nowDate.Add(-time.Duration(weekdayToOffsetWithMondayFirst(nowDate.Weekday())) * 24 * time.Hour)
	if result.Weekday() != time.Monday {
	}
	return result, nil
}

func GetWeekStartSunday() (time.Time, error) {
	nowDate, err := GetCurrentDate()
	if err != nil {
		return time.Time{}, Errorf("BUG: parse current date failed: %s", err)
	}

	return nowDate.Add(-time.Duration(weekdayToOffsetWithSundayFirst(nowDate.Weekday())) * 24 * time.Hour), nil
}

func TickEvery1s(falseToBreak func() bool) {
	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-t.C:
			if !falseToBreak() {
				return
			}
		}
	}
}

func TickWithTimeoutContext(ctx context.Context, timeout, interval time.Duration, falseToBreak func() bool) (exitedByCondition bool) {
	newCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-newCtx.Done():
			return false
		case <-t.C:
			if !falseToBreak() {
				return true
			}
		}
	}
}

func TickWithTimeout(timeout, interval time.Duration, falseToBreak func() bool) (exitedByCondition bool) {
	return TickWithTimeoutContext(context.Background(), timeout, interval, falseToBreak)
}

func Tick1sWithTimeout(timeout time.Duration, falseToBreak func() bool) (exitedByCondition bool) {
	return TickWithTimeoutContext(context.Background(), timeout, 1*time.Second, falseToBreak)
}

func LoopEvery1sBreakUntil(until func() bool) {
	t := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-t.C:
			if until() {
				return
			}
		}
	}
}
