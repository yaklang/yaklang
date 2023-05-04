package arptable

import (
	"sync"
	"time"
)

type cache struct {
	sync.RWMutex
	table ArpTable

	Updated      time.Time
	UpdatedCount int
}

func (c *cache) Refresh() {
	c.Lock()
	defer c.Unlock()

	c.table = Table()
	c.Updated = time.Now()
	c.UpdatedCount += 1
}

func (c *cache) Search(ip string) string {
	c.RLock()
	defer c.RUnlock()

	mac, ok := c.table[ip]

	if !ok {
		c.RUnlock()
		c.Refresh()
		c.RLock()
		mac = c.table[ip]
	}

	return mac
}
