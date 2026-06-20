package hids

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const LASTCPUPERCENT_KEY = "LastCPUPercent"

// CPUPercentCallback 当 CPU 使用率发生变化时，调用 callback 函数
//
// 参数:
//   - callback: 回调函数，入参为当前 CPU 使用率（百分比）
//
// Example:
// ```
// hids.Init()
// hids.CPUPercentCallback(func(i) {
// if (i > 50) { println("cpu precent is over 50%") } // 当 CPU 使用率超过50%时输出信息
// })
// ```
func CPUPercentCallback(callback func(i float64)) {
	GetGlobalHealthManager().RegisterCPUPercentCallback(callback)
}

// CPUAverageCallback 当 CPU 使用率平均值发生变化时，调用 callback 函数
//
// 参数:
//   - callback: 回调函数，入参为当前 CPU 使用率平均值（百分比）
//
// Example:
// ```
// hids.Init()
// hids.CPUAverageCallback(func(i) {
// if (i > 50) { println("cpu average precent is over 50%") } // 当 CPU 使用率平均值超过50%时输出信息
// })
// ```
func CPUAverageCallback(callback func(i float64)) {
	GetGlobalHealthManager().RegisterCPUAverageCallback(callback)
}

// CPUPercent 获取当前系统的 CPU 使用率
//
// 返回值:
//   - CPU 使用率（百分比，0-100）
//
// Example:
// ```
// printf("%f%%\n", hids.CPUPercent())
// ```
func CPUPercent() float64 {
	if info, err := SystemHealthStats(); err != nil {
		log.Errorf("cannot get system-health-stats, reason: %s", err)
		return 0
	} else {
		return info.CPUPercent
	}
}

// CPUAverage 获取当前系统的 CPU 使用率平均值
//
// 返回值:
//   - CPU 使用率平均值（百分比，0-100）
//
// Example:
// ```
// printf("%f%%\n", hids.CPUAverage())
// ```
func CPUAverage() float64 {
	if ret := codec.Atof(yakit.GetKey(consts.GetGormProfileDatabase(), LASTCPUPERCENT_KEY)); ret > 0 {
		return (CPUPercent() + ret) / 2.0
	}
	return CPUPercent()
}
