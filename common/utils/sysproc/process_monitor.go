package sysproc

import (
	"context"
	"github.com/gobwas/glob"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"sync"
)

type GlobalProcessMonitor struct {
	patternLock sync.Mutex
	globPattern map[string]*glob.Glob

	processLock       sync.Mutex
	watchingProcesses map[int32]context.CancelFunc

	ctx context.Context
}

func (pm *GlobalProcessMonitor) Start() {

}

func (pm *GlobalProcessMonitor) Stop() {}

func (pm *GlobalProcessMonitor) AddGlobPattern(pattern string) error {
	pm.patternLock.Lock()
	defer pm.patternLock.Unlock()
	if _, exists := pm.globPattern[pattern]; exists {
		return nil
	}
	g, err := glob.Compile(pattern)
	if err != nil {
		return err
	}
	pm.globPattern[pattern] = &g
	return nil
}

func (pm *GlobalProcessMonitor) WatchProcesses(ctx context.Context, pid int32) error {
	pm.processLock.Lock()
	defer pm.processLock.Unlock()
	if _, exists := pm.watchingProcesses[pid]; exists {
		return nil
	}
	_, cancel := context.WithCancel(ctx)
	pm.watchingProcesses[pid] = cancel

	return nil
}

func (pm *GlobalProcessMonitor) DetectProcessConnections(pid int32, limit int) ([]net.ConnectionStat, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	conns, err := p.ConnectionsMax(limit)
}
