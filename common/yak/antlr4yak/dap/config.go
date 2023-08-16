package dap

import "net"

type DAPServerConfig struct {
	listener  net.Listener
	stopped   chan struct{}
	extraLibs map[string]interface{}
}

func (c *DAPServerConfig) triggerServerStop() {
	if c.stopped != nil {
		close(c.stopped)
		c.stopped = nil
	}
}
