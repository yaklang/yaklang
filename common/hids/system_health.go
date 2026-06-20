package hids

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec/health"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/healthinfo"
)

func SystemHealthStats() (*health.HealthInfo, error) {
	return healthinfo.NewHealthInfo(utils.TimeoutContextSeconds(3))
}

// MemoryPercent 获取当前系统的内存使用率
//
// 返回值:
//   - 内存使用率（百分比，0-100）
//
// Example:
// ```
// printf("%f%%\n", hids.MemoryPercent())
// ```
func MemoryPercent() float64 {
	if info, err := SystemHealthStats(); err != nil {
		log.Errorf("cannot get system-health-stats, reason: %s", err)
		return 0
	} else {
		return info.MemoryPercent
	}
}

// MemoryPercentCallback 当内存使用率发生变化时，调用 callback
//
// 参数:
//   - callback: 回调函数，入参为当前内存使用率（百分比）
//
// Example:
// ```
// hids.Init()
// hids.MemoryPercentCallback(func(i) {
// if (i > 50) { println("memory precent is over 50%") } // 当内存使用率超过50%时输出信息
// })
// ```
func MemoryPercentCallback(callback func(i float64)) {
	GetGlobalHealthManager().RegisterMemPercentCallback(callback)
}
