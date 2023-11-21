package hids

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const LASTCPUPERCENT_KEY = "LastCPUPercent"

// CPUPercentCallback 当 CPU 使用率发生变化时，调用 callback 函数
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

// CPUPercentCallback 当 CPU 使用率平均值发生变化时，调用 callback 函数
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
