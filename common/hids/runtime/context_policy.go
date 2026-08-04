//go:build hids && linux

package runtime

import (
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	defaultShortTermContextMaxProcesses = 4096
	defaultShortTermContextMaxNetworks  = 4096
	defaultShortTermContextMaxFiles     = 4096
	shortTermContextPruneInterval       = time.Minute
)

type shortTermContextConfig struct {
	window       time.Duration
	maxProcesses int
	maxNetworks  int
	maxFiles     int
}

func shortTermContextConfigFromSpec(spec model.DesiredSpec) shortTermContextConfig {
	window := spec.ContextPolicy.ShortTermWindow()
	if window <= 0 {
		window = time.Duration(model.DefaultShortTermWindowMinutes) * time.Minute
	}
	return shortTermContextConfig{
		window:       window,
		maxProcesses: defaultShortTermContextMaxProcesses,
		maxNetworks:  defaultShortTermContextMaxNetworks,
		maxFiles:     defaultShortTermContextMaxFiles,
	}
}
