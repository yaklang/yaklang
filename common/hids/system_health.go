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
