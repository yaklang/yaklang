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

	// 简单地返回结果，不做自动刷新
	return c.table[ip]
}
