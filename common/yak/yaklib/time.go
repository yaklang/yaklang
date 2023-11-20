package yaklib

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// now 用于获取当前时间的时间结构体
// Example:
// ```
// dur = time.ParseDuration("1m")~
// ctx, cancel = context.WithDeadline(context.New(), now().Add(dur))
//
// println(now().Format("2006-01-02 15:04:05"))
// ```
func _timeNow() time.Time {
	return time.Now()
}

// now 用于获取当前时间的时间结构体
// 它实际是 time.Now 的别名
// Example:
// ```
// dur = time.ParseDuration("1m")~
// ctx, cancel = context.WithDeadline(context.New(), now().Add(dur))
//
// println(now().Format("2006-01-02 15:04:05"))
// ```
func _timenow() time.Time {
	return time.Now()
}

// GetCurrentMonday 返回精确到本周星期一的时间结构体与错误
// Example:
// ```
// monday, err = time.GetCurrentMonday()
// ```
func _getCurrentMonday() (time.Time, error) {
	return utils.GetCurrentWeekMonday()
}

// GetCurrentDate 返回精确到当前日期的时间结构体与错误
// Example:
// ```
// date, err = time.GetCurrentDate()
// ```
func _getCurrentDate() (time.Time, error) {
	return utils.GetCurrentDate()
}

// Parse 根据给定的格式解析时间字符串，返回时间结构体与错误
// 一个参考的格式为：2006-01-02 15:04:05
// Example:
// ```
// t, err = time.Parse("2006-01-02 15:04:05", "2020-01-01 00:00:00")
// ```
func _timeparse(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

// ParseDuration 根据给定的格式解析时间间隔字符串，返回时间间隔结构体与错误
// 时间间隔字符串是一个可能带有符号的十进制数字序列，每个数字可以带有可选的小数和单位后缀，例如 "300ms"，"-1.5h" 或 "2h45m"
// 有效的时间单位有 "ns"（纳秒）, "us"（或 "µs" 微秒）, "ms"（毫秒）, "s"（秒）, "m"（分）, "h"（小时）
// Example:
// ```
// d, err = time.ParseDuration("1h30m")
// ```
func _timeParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// Unix 函数根据给定的 Unix 时间戳（从 1970 年 1 月 1 日 UTC 开始的 sec 秒和 nsec 纳秒）返回相应的本地时间结构体
// Example:
// ```
// time.Unix(1577808000, 0) // 2020-01-01 00:00:00 +0800 CST
// ```
func _timeUnix(sec int64, nsec int64) time.Time {
	return time.Unix(sec, nsec)
}

// After 用于创建一个定时器，它会在指定的时间后向返回的通道发送当前时间
// Example:
// ```
// d, err = time.ParseDuration("5s")
// <-time.After(d) // 等待5秒后执行后续的语句
// tln("after 5s")
// ```
func _timeAfter(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// AfterFunc 用于创建一个定时器，它会在指定的时间后执行指定的函数，该函数会在另一个协程中执行
// 该函数本身会立刻返回一个定时器结构体引用，你可以通过调用该引用的Stop方法来取消定时器
// Example:
// ```
// d, err = time.ParseDuration("5s")
// timer = time.AfterFunc(d, () => println("after 5s")) // 你可以通过调用 timer.Stop() 来取消定时器
// time.sleep(10)
// ```
func _timeAfterFunc(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}

// NewTimer 根据给定的时间间隔(单位：秒)返回一个定时器结构体引用
// 你可以通过 <- timer.C 来等待定时器到期
// 你也可以通过调用 timer.Stop 来取消定时器
// Example:
// ```
// timer = time.NewTimer(5) // 你可以通过调用 timer.Stop() 来取消定时器
// <-timer.C // 等待定时器到期
// ```
func _timeNewTimer(d float64) *time.Timer {
	return time.NewTimer(utils.FloatSecondDuration(d))
}

// NewTicker 根据给定的时间间隔(单位：秒)返回一个循环定时器结构体引用，它会周期性的向返回的通道发送当前时间
// 你可以通过 <- timer.C 来等待循环定时器到期
// 你也可以通过调用 timer.Stop 来取消循环定时器
// Example:
// ```
// timer = time.NewTicker(5) // 你可以通过调用 timer.Stop() 来取消定时器
// ticker = time.NewTicker(1)
// for t in ticker.C {
// println("tick") // 每 1 秒打印一次 tick
// }
// ```
func _timeNewTicker(d float64) *time.Ticker {
	return time.NewTicker(utils.FloatSecondDuration(d))
}

// Until 函数返回当前时间到 t (未来时间)的时间间隔
// Example:
// ```
// t = time.Unix(1704038400, 0) // 2024-1-1 00:00:00 +0800 CST
// time.Until(t) // 返回当前时间到 t 的时间间隔
// ```
func _timeUntil(t time.Time) time.Duration {
	return time.Until(t)
}

// Since 函数返回自 t (过去时间)到当前时间的时间间隔
// Example:
// ```
// t = time.Unix(1577808000, 0) // 2020-01-01 00:00:00 +0800 CST
// time.Since(t) // 返回 t 到当前时间的时间间隔
// ```
func _timeSince(t time.Time) time.Duration {
	return time.Since(t)
}

var TimeExports = map[string]interface{}{
	"Now":              _timeNow,
	"now":              _timenow,
	"GetCurrentMonday": _getCurrentMonday,
	"GetCurrentDate":   _getCurrentDate,
	"sleep":            sleep,
	"Sleep":            sleep,
	"Parse":            _timeparse,
	"ParseDuration":    _timeParseDuration,
	"Unix":             _timeUnix,
	"After":            _timeAfter,
	"AfterFunc":        _timeAfterFunc,
	"NewTimer":         _timeNewTimer,
	"NewTicker":        _timeNewTicker,
	"Until":            _timeUntil,
	"Since":            _timeSince,
}
