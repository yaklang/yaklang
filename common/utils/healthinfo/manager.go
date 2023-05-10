package healthinfo

import (
	"context"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec/health"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"time"
)

const HealthManagerPersistantKey = "7b14dc58c8e9f5b39ef92ee4ce7c0d93fa7d8836aa3d6b67c106608c2bd6ddb1-HealthManagerPersistantKey"

type Manager struct {
	interval time.Duration
	cancel   context.CancelFunc

	startTime    time.Time
	maxInfoCount int
	infos        []*health.HealthInfo

	onChange     []func(info []*health.HealthInfo)
	onCPUPercent []func(percent float64)
	onCPUAverage []func(percent float64)
	onMemPercent []func(percent float64)
	onMemAverage []func(percent float64)
}

func (m *Manager) RegisterInfoChangedCallback(cb func(info []*health.HealthInfo)) {
	if m == nil {
		log.Warnf("cannot register callback on nil health manager")
		return
	}
	if cb == nil {
		return
	}
	m.onChange = append(m.onChange, cb)
}

func (m *Manager) RegisterCPUPercentCallback(cb func(percent float64)) {
	if m == nil {
		log.Warnf("cannot register callback on nil health manager")
		return
	}
	if cb == nil {
		return
	}
	m.onCPUPercent = append(m.onCPUPercent, cb)
}

func (m *Manager) RegisterCPUAverageCallback(cb func(percent float64)) {
	if m == nil {
		log.Warnf("cannot register callback on nil health manager")
		return
	}
	if cb == nil {
		return
	}
	m.onCPUAverage = append(m.onCPUAverage, cb)
}

func (m *Manager) RegisterMemPercentCallback(cb func(percent float64)) {
	if m == nil {
		log.Warnf("cannot register callback on nil health manager")
		return
	}
	if cb == nil {
		return
	}
	m.onMemPercent = append(m.onMemPercent, cb)
}

func (m *Manager) RegisterMemAverageCallback(cb func(percent float64)) {
	if m == nil {
		log.Warnf("cannot register callback on nil health manager")
		return
	}
	if cb == nil {
		return
	}
	m.onMemAverage = append(m.onMemAverage, cb)
}

func (m *Manager) start(interval time.Duration) context.CancelFunc {
	m.interval = interval
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		var cpuavg float64
		var memavg float64

		ticker := time.Tick(interval)
		data := yakit.Get(HealthManagerPersistantKey)
		if data != "" {
			var infos []*health.HealthInfo
			log.Info("start to use old cache for health info")
			err := json.Unmarshal([]byte(data), &infos)
			if err != nil {
				spew.Dump(data)
				log.Warnf("unmarshal health info failed: %s", err)
			}
			if len(infos) > 0 {
				m.infos = infos
				for _, i := range infos {
					cpuavg += i.CPUPercent
					memavg += i.MemoryPercent
				}
				cpuavg = cpuavg / float64(len(infos))
				memavg = memavg / float64(len(infos))
				log.Infof("load cached health infos total: %v, initial cpu avg: %v, mem avg: %v", len(infos), cpuavg, memavg)
			}
		}
		for {
			select {
			case <-ticker:
				cpuavg, memavg = m.updateInfos(cpuavg, memavg)
				var raw, _ = json.Marshal(m.infos)
				yakit.Set(HealthManagerPersistantKey, string(raw))

				if m.onChange != nil {
					var infos = make([]*health.HealthInfo, len(m.infos))
					copy(infos, m.infos)
					for _, cb := range m.onChange {
						cb(infos)
					}
				}
				for _, cb := range m.onCPUAverage {
					cb(cpuavg)
				}
				for _, cb := range m.onMemAverage {
					cb(memavg)
				}
			case <-ctx.Done():
			}
		}
	}()
	return cancel
}

func (m *Manager) updateInfos(cpuavg, memavg float64) (float64, float64) {
	ctx := utils.TimeoutContext(m.interval)
	info, err := NewHealthInfo(ctx)
	if err != nil {
		log.Errorf("update health info failed: %s", err)
		return 0, 0
	}

	if info == nil {
		return 0, 0
	}

	for _, cb := range m.onCPUPercent {
		cb(info.CPUPercent)
	}
	for _, cb := range m.onMemPercent {
		cb(info.MemoryPercent)
	}

	cpuavg = (info.CPUPercent + cpuavg) / 2.0
	memavg = (info.MemoryPercent + memavg) / 2.0

	if m.maxInfoCount <= len(m.infos) {
		m.infos = append(m.infos[1:], info)
	} else {
		m.infos = append(m.infos, info)
	}
	return cpuavg, memavg
}

func (m *Manager) GetHealthInfos() []*health.HealthInfo {
	return m.infos[:]
}

func (m *Manager) GetAliveDuration() time.Duration {
	return time.Now().Sub(m.startTime)
}

func NewHealthInfoManager(interval, maxCacheTime time.Duration) (*Manager, error) {
	m := &Manager{
		startTime: time.Now(),
	}

	if interval.Seconds() < 1 {
		log.Warnf("interval %v too small, set(auto) to 1 second", interval)
		interval = 1 * time.Second
	}

	m.maxInfoCount = int(maxCacheTime / interval)
	log.Debugf("health info: cache %v infos", m.maxInfoCount)
	if m.maxInfoCount <= 0 {
		return nil, errors.Errorf(
			"max health info count %v, maxCacheTime should larger than interval", m.maxInfoCount,
		)
	}

	m.cancel = m.start(interval)
	return m, nil
}

func (m *Manager) Cancel() {
	if m == nil {
		return
	}
	if m.cancel != nil {
		m.cancel()
	}
}
