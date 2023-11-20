package yaklib

import (
	"time"
	_ "time/tzdata"
)

// Get 返回具有给定名称的时区与错误
// 如果名称为空字符串 "" 或 "UTC"，LoadLocation 返回 UTC 时区
// 如果名称为 "Local"，LoadLocation 返回本地时区
// 否则，该名称被视为 IANA 时区数据库中的一个位置名称，如 "America/New_York"
// Example:
// ```
// loc, err = timezone.Get("Asia/Shanghai")
// ```
func _timezoneLoadLocation(name string) (*time.Location, error) {
	return time.LoadLocation(name)
}

// Now 根据给定名称的时区返回当前时间结构体
// Example:
// ```
// now = timezone.Now("Asia/Shanghai")
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
