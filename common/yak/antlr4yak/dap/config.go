package dap

import (
	"net"
	"sync"
)

type DAPServerConfig struct {
	listener  net.Listener
	stopped   chan struct{}
	stopOnce  sync.Once
	extraLibs map[string]interface{}
}

// triggerServerStop 关闭 stopped 通道以通知服务停止。
// 使用 sync.Once 保证只 close 一次，并且不再把 stopped 置 nil，
// 避免与 DAPServer.Start 中对 stopped 的并发读取产生数据竞争或重复 close panic。
func (c *DAPServerConfig) triggerServerStop() {
	c.stopOnce.Do(func() {
		if c.stopped != nil {
			close(c.stopped)
		}
	})
}
