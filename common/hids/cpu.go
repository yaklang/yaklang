package hids

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const LASTCPUPERCENT_KEY = "LastCPUPercent"

func CPUPercentCallback(callback func(i float64)) {
	GetGlobalHealthManager().RegisterCPUPercentCallback(callback)
}

func CPUAverageCallback(callback func(i float64)) {
	GetGlobalHealthManager().RegisterCPUAverageCallback(callback)
}

func CPUPercent() float64 {
	if info, err := SystemHealthStats(); err != nil {
		log.Errorf("cannot get system-health-stats, reason: %s", err)
		return 0
	} else {
		return info.CPUPercent
	}
}

func CPUAverage() float64 {
	if ret := utils.Atof(yakit.GetKey(consts.GetGormProfileDatabase(), LASTCPUPERCENT_KEY)); ret > 0 {
		return (CPUPercent() + ret) / 2.0
	}
	return CPUPercent()
}
