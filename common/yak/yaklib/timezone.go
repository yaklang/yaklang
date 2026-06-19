package yaklib

import (
	"time"
	_ "time/tzdata"
)

// Get 返回具有给定名称的时区与错误
// 如果名称为空字符串 "" 或 "UTC"，LoadLocation 返回 UTC 时区
// 如果名称为 "Local"，LoadLocation 返回本地时区
// 否则，该名称被视为 IANA 时区数据库中的一个位置名称，如 "America/New_York"
// 参数:
//   - name: 时区名称，如 "UTC"、"Local"、"Asia/Shanghai"
//
// 返回值:
//   - 解析得到的时区对象
//   - 名称无效时返回的错误
//
// Example:
// ```
// // VARS: 加载上海时区
// loc = timezone.Get("Asia/Shanghai")~
// // STDOUT: 打印时区名称
// println(loc.String())   // OUT: Asia/Shanghai
// // assert: 锁定结论
// assert loc.String() == "Asia/Shanghai", "Get should load the named location"
// ```
func _timezoneLoadLocation(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}

// Now 根据给定名称的时区返回当前时间结构体
// 参数:
//   - name: 时区名称，如 "UTC"、"Asia/Shanghai"；名称无效时回退到本地时间
//
// 返回值:
//   - 该时区下的当前时间
//
// Example:
// ```
// // 获取上海时区下的当前时间(结果随运行时刻变化，仅作示意)
// now = timezone.Now("Asia/Shanghai")
// println(now.String())
// ```
func _timezoneNow(name string) time.Time {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.Now()
	}
	return time.Now().In(loc)
}

var TimeZoneExports = map[string]interface{}{
	"Get": _timezoneLoadLocation,
	"Now": _timezoneNow,
}
