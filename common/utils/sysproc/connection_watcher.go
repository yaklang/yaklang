package sysproc

import (
	"context"
	"fmt"
	"github.com/shirou/gopsutil/v4/net"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// NewRemoteIPCallback 是发现新外联 IP 时的回调函数签名
type NewRemoteIPCallback func(pid int32, remoteIP string)

// ConnectionsWatcher 封装了针对单个进程的监控逻辑
type ConnectionsWatcher struct {
	Pid          int32
	Proc         *process.Process
	seenIPs      sync.Map // 已发现的 IP 地址 (key: IP string, value: bool true)
	callback     NewRemoteIPCallback
	pollInterval time.Duration // 轮询连接的频率
}

// NewWatcher 创建一个新的进程监控器实例
func NewWatcher(pid int32, cb NewRemoteIPCallback, interval time.Duration) (*ConnectionsWatcher, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("can not found process (PID %d): %v", pid, err)
	}

	return &ConnectionsWatcher{
		Pid:          pid,
		Proc:         proc,
		callback:     cb,
		pollInterval: interval,
	}, nil
}

// Start 启动监控循环，直到 Context 被取消或进程退出
func (w *ConnectionsWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	log.Printf("[PID %d] start watching, ticker duration: %v", w.Pid, w.pollInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[PID %d] context cancel", w.Pid)
			return
		case <-ticker.C:

			if w.Proc == nil {
				log.Printf("[PID %d] process exit , stop watching", w.Pid)
				return
			}

			isRunning, _ := w.Proc.IsRunning()
			if !isRunning {
				log.Printf("[PID %d] process exit , stop watching", w.Pid)
				return
			}

			conns, err := w.Proc.Connections()
			if err != nil {
				if strings.Contains(err.Error(), "permission denied") {
					log.Printf("[PID %d] permission denied , can not get connections information", w.Pid)
					continue
				}
				log.Printf("[PID %d] get connections information fail: %v", w.Pid, err)
				continue
			}

			w.processConnections(conns)
		}
	}
}

// processConnections 检查新连接并触发回调
func (w *ConnectionsWatcher) processConnections(conns []net.ConnectionStat) {
	for _, c := range conns {
		remoteIP := c.Raddr.IP

		if remoteIP == "127.0.0.1" || remoteIP == "::1" || c.Status == "LISTEN" || remoteIP == "" {
			continue
		}

		if _, loaded := w.seenIPs.LoadOrStore(remoteIP, true); !loaded {
			w.callback(w.Pid, remoteIP)
		}
	}
}
