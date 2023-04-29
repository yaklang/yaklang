package hybridscan

import (
	"fmt"
	"github.com/pkg/errors"
	"net"
)

func oPToStr(ip net.IP, port int) string {
	return fmt.Sprintf("%v:%v", ip.String(), port)
}

func (c *HyperScanCenter) onOpenPort(ip net.IP, port int) {
	_, ok := c.config.OpenPortTTLCache.Get(oPToStr(ip, port))
	if ok {
		return
	}
	c.config.OpenPortTTLCache.Set(oPToStr(ip, port), 1)

	for _, h := range c.openPortHandlers {
		h(ip, port)
	}
}

func (c *HyperScanCenter) RegisterSynScanOpenPortHandler(tag string, h func(ip net.IP, port int)) error {
	c.openPortHandlerMutex.Lock()
	defer c.openPortHandlerMutex.Unlock()

	if _, ok := c.openPortHandlers[tag]; ok {
		return errors.Errorf("existed handler: %v", tag)
	}

	c.openPortHandlers[tag] = h
	return nil
}

func (c *HyperScanCenter) UnregisterSynScanOpenPortHandler(tag string) {
	c.openPortHandlerMutex.Lock()
	defer c.openPortHandlerMutex.Unlock()

	delete(c.openPortHandlers, tag)
}
