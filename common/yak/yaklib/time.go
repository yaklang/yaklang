package yaklib

import (
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

// Now 用于获取当前时间的时间结构体
// 返回值:
//   - 当前时间的 time.Time 结构体
//
// Example:
// ```
// // 获取当前时间(结果随运行时刻变化，作示意)
// now = time.Now()
// println(now.Format("2006-01-02 15:04:05"))
// ```
func _timeNow() time.Time {
	return time.Now()
}

// now 用于获取当前时间的时间结构体
// 它实际是 time.Now 的别名
// 返回值:
//   - 当前时间的 time.Time 结构体
//
// Example:
// ```
// // now 是 time.Now 的别名(结果随运行时刻变化，作示意)
// cur = now()
// println(cur.Format("2006-01-02 15:04:05"))
// ```
func _timenow() time.Time {
	return time.Now()
}

// GetCurrentMonday 返回精确到本周星期一的时间结构体与错误
// 返回值:
//   - 本周星期一 00:00:00 的时间结构体
//   - 计算失败时返回的错误
//
// Example:
// ```
// // 获取本周一(结果随运行时刻变化，作示意)
// monday, err = time.GetCurrentMonday()
// println(monday.Format("2006-01-02"))
// ```
func _getCurrentMonday() (time.Time, error) {
	return utils.GetCurrentWeekMonday()
}

// GetCurrentDate 返回精确到当前日期的时间结构体与错误
// 返回值:
//   - 今天 00:00:00 的时间结构体
//   - 计算失败时返回的错误
//
// Example:
// ```
// // 获取今天日期(结果随运行时刻变化，作示意)
// date, err = time.GetCurrentDate()
// println(date.Format("2006-01-02"))
// ```
func _getCurrentDate() (time.Time, error) {
	return utils.GetCurrentDate()
}

// Parse 根据给定的格式解析时间字符串，返回时间结构体与错误
// 一个参考的格式为：2006-01-02 15:04:05
// 参数:
//   - layout: 时间格式模板，参考时间为 2006-01-02 15:04:05
//   - value: 待解析的时间字符串
//
// 返回值:
//   - 解析得到的时间结构体
//   - 解析失败时返回的错误
//
// Example:
// ```
// // VARS: 解析固定时间字符串
// t = time.Parse("2006-01-02 15:04:05", "2020-01-01 00:00:00")~
// // STDOUT: 打印解析出的年份
// println(t.Year())   // OUT: 2020
// // assert: 锁定结论
// assert t.Year() == 2020, "Parse should read the year as 2020"
// ```
func _timeparse(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

// ParseDuration 根据给定的格式解析时间间隔字符串，返回时间间隔结构体与错误
// 时间间隔字符串是一个可能带有符号的十进制数字序列，每个数字可以带有可选的小数和单位后缀，例如 "300ms"，"-1.5h" 或 "2h45m"
// 有效的时间单位有 "ns"（纳秒）, "us"（或 "µs" 微秒）, "ms"（毫秒）, "s"（秒）, "m"（分）, "h"（小时）
// 参数:
//   - s: 时间间隔字符串，如 "300ms"、"-1.5h"、"2h45m"
//
// 返回值:
//   - 解析得到的时间间隔
//   - 解析失败时返回的错误
//
// Example:
// ```
// // VARS: 解析 1 小时 30 分
// d = time.ParseDuration("1h30m")~
// // STDOUT: 打印总秒数
// println(d.Seconds())   // OUT: 5400
// // assert: 锁定结论
// assert d.Seconds() == 5400, "1h30m should be 5400 seconds"
// ```
func _timeParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// Unix 函数根据给定的 Unix 时间戳（从 1970 年 1 月 1 日 UTC 开始的 sec 秒和 nsec 纳秒）返回相应的本地时间结构体
// 参数:
//   - sec: 自 1970-01-01 UTC 起的秒数
//   - nsec: 额外的纳秒数
//
// 返回值:
//   - 对应时间戳的本地时间结构体
//
// Example:
// ```
// // VARS: 由时间戳还原时间
// t = time.Unix(1577808000, 0)
// // STDOUT: 打印还原出的时间戳
// println(t.Unix())   // OUT: 1577808000
// // assert: 锁定结论
// assert t.Unix() == 1577808000, "Unix should round-trip the timestamp"
// ```
func _timeUnix(sec int64, nsec int64) time.Time {
	return time.Unix(sec, nsec)
}

// After 用于创建一个定时器，它会在指定的时间后向返回的通道发送当前时间
// 参数:
//   - d: 等待的时间间隔
//
// 返回值:
//   - 一个通道，到期后会收到当前时间
//
// Example:
// ```
// // 等待 5 秒后继续(作示意)
// d = time.ParseDuration("5s")~
// <-time.After(d)
// println("after 5s")
// ```
func _timeAfter(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// AfterFunc 用于创建一个定时器，它会在指定的时间后执行指定的函数，该函数会在另一个协程中执行
// 该函数本身会立刻返回一个定时器结构体引用，你可以通过调用该引用的Stop方法来取消定时器
// 参数:
//   - d: 等待的时间间隔
//   - f: 到期后要执行的回调函数
//
// 返回值:
//   - 定时器引用，可调用 Stop 取消
//
// Example:
// ```
// // 5 秒后执行回调(作示意)
// d = time.ParseDuration("5s")~
// timer = time.AfterFunc(d, () => println("after 5s"))
// time.sleep(10)
// ```
func _timeAfterFunc(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}

// NewTimer 根据给定的时间间隔(单位：秒)返回一个定时器结构体引用
// 你可以通过 <- timer.C 来等待定时器到期
// 你也可以通过调用 timer.Stop 来取消定时器
// 参数:
//   - d: 定时器时长，单位为秒
//
// 返回值:
//   - 定时器引用，可通过 timer.C 等待到期或 timer.Stop 取消
//
// Example:
// ```
// // 5 秒定时器(作示意)
// timer = time.NewTimer(5)
// <-timer.C
// ```
func _timeNewTimer(d float64) *time.Timer {
	return time.NewTimer(utils.FloatSecondDuration(d))
}

// NewTicker 根据给定的时间间隔(单位：秒)返回一个循环定时器结构体引用，它会周期性的向返回的通道发送当前时间
// 你可以通过 <- timer.C 来等待循环定时器到期
// 你也可以通过调用 timer.Stop 来取消循环定时器
// 参数:
//   - d: 循环周期，单位为秒
//
// 返回值:
//   - 循环定时器引用，可通过 ticker.C 周期性接收时间或 ticker.Stop 取消
//
// Example:
// ```
// // 每 1 秒触发一次(作示意)
// ticker = time.NewTicker(1)
//
//	for t in ticker.C {
//	    println("tick")
//	}
//
// ```
func _timeNewTicker(d float64) *time.Ticker {
	return time.NewTicker(utils.FloatSecondDuration(d))
}

// Until 函数返回当前时间到 t (未来时间)的时间间隔
// 参数:
//   - t: 目标(未来)时间
//
// 返回值:
//   - 从当前时间到 t 的时间间隔
//
// Example:
// ```
// // 计算距离某未来时间还有多久(结果随运行时刻变化，作示意)
// t = time.Unix(1704038400, 0)
// println(time.Until(t).String())
// ```
func _timeUntil(t time.Time) time.Duration {
	return time.Until(t)
}

// Since 函数返回自 t (过去时间)到当前时间的时间间隔
// 参数:
//   - t: 起始(过去)时间
//
// 返回值:
//   - 从 t 到当前时间的时间间隔
//
// Example:
// ```
// // 计算从某过去时间到现在的间隔(结果随运行时刻变化，作示意)
// t = time.Unix(1577808000, 0)
// println(time.Since(t).String())
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
