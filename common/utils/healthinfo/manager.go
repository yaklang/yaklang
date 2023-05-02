package healthinfo

import (
	"context"
	"github.com/pkg/errors"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec/health"
	"github.com/yaklang/yaklang/common/utils"
)

type Manager struct {
	interval time.Duration
	cancel   context.CancelFunc

	startTime    time.Time
	maxInfoCount int
	infos        []*health.HealthInfo
}

func (m *Manager) start(interval time.Duration) context.CancelFunc {
	m.interval = interval
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.Tick(interval)
		for {
			select {
			case <-ticker:
				m.updateInfos()
			case <-ctx.Done():
			}
		}
	}()
	return cancel
}

func (m *Manager) updateInfos() {
	ctx := utils.TimeoutContext(m.interval)
	if m.maxInfoCount <= len(m.infos) {
		info, err := NewHealthInfo(ctx)
		if err != nil {
			log.Errorf("update health info failed: %s", err)
			return
		}
		m.infos = append(m.infos[1:], info)
	} else {
		info, err := NewHealthInfo(ctx)
		if err != nil {
			log.Errorf("update health info failed: %s", err)
			return
		}
		m.infos = append(m.infos, info)
	}
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

	m.maxInfoCount = int(maxCacheTime / interval)
	log.Infof("health info: cache %v infos", m.maxInfoCount)
	if m.maxInfoCount <= 0 {
		return nil, errors.Errorf(
			"max health info count %v, maxCacheTime should larger than interval", m.maxInfoCount,
		)
	}

	cancel := m.start(interval)
	m.cancel = cancel

	return m, nil
}
