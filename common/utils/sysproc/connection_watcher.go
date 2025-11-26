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
type NewRemoteIPCallback func(pid int32, remoteIP string, domain string)

// ProcessWatcher 封装了针对单个进程的监控逻辑
type ProcessWatcher struct {
	Pid          int32
	Proc         *process.Process
	seenIPs      sync.Map // 已发现的 IP 地址 (key: IP string, value: bool true)
	callback     NewRemoteIPCallback
	pollInterval time.Duration // 轮询连接的频率
}

// NewWatcher 创建一个新的进程监控器实例
func NewWatcher(pid int32, cb NewRemoteIPCallback, interval time.Duration) (*ProcessWatcher, error) {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("无法找到或创建进程对象 (PID %d): %v", pid, err)
	}

	return &ProcessWatcher{
		Pid:          pid,
		Proc:         proc,
		callback:     cb,
		pollInterval: interval,
	}, nil
}

// Start 启动监控循环，直到 Context 被取消或进程退出
func (w *ProcessWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	log.Printf("[PID %d] 监控器启动，轮询间隔: %v", w.Pid, w.pollInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[PID %d] 监控器因 Context 取消而退出。", w.Pid)
			return
		case <-ticker.C:
			// 1. 检查进程是否仍然存活
			isRunning, _ := w.Proc.IsRunning()
			if !isRunning {
				log.Printf("[PID %d] 进程已退出，停止监控。", w.Pid)
				return
			}

			// 2. 获取进程连接
			conns, err := w.Proc.Connections()
			if err != nil {
				// 权限不足或瞬时错误是常见的，继续即可
				if strings.Contains(err.Error(), "permission denied") {
					log.Printf("[PID %d] 权限不足，无法获取连接信息。请以管理员身份运行。", w.Pid)
					continue
				}
				// 忽略其他错误，但记录重要的
				log.Printf("[PID %d] 获取连接失败: %v", w.Pid, err)
				continue
			}

			// 3. 遍历连接并去重
			w.processConnections(conns)
		}
	}
}

// processConnections 检查新连接并触发回调
func (w *ProcessWatcher) processConnections(conns []net.ConnectionStat) {
	for _, c := range conns {
		remoteIP := c.Raddr.IP

		// 过滤不必要的连接 (本地回环、监听状态)
		if remoteIP == "127.0.0.1" || remoteIP == "::1" || c.Status == "LISTEN" || remoteIP == "" {
			continue
		}

		// 检查 IP 是否已经记录 (去重)
		if _, loaded := w.seenIPs.LoadOrStore(remoteIP, true); !loaded {
			// 这是一个新的远端 IP

			// 尝试从 DNS 缓存中获取域名
			domain := w.resolveIPToDomain(remoteIP)

			// 触发回调
			w.callback(w.Pid, remoteIP, domain)
		}
	}
}

// resolveIPToDomain 模拟 IP -> 域名的解析
func (w *ProcessWatcher) resolveIPToDomain(ip string) string {
	return "N/A (未命中 DNS 缓存)"
}
