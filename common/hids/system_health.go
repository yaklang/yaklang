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

func MemoryPercent() float64 {
	if info, err := SystemHealthStats(); err != nil {
		log.Errorf("cannot get system-health-stats, reason: %s", err)
		return 0
	} else {
		return info.MemoryPercent
	}
}

func MemoryPercentCallback(callback func(i float64)) {
	GetGlobalHealthManager().RegisterMemPercentCallback(callback)
}
