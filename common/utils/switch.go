package utils

import "sync"

type Switch struct {
	op bool
	*sync.Cond
}

func NewSwitch(b ...bool) *Switch {
	op := false
	if len(b) > 0 {
		op = b[0]
	}
	return &Switch{op, sync.NewCond(&sync.Mutex{})}
}

func (c *Switch) SwitchTo(b bool) {
	c.L.Lock()
	c.op = b
	c.L.Unlock()
	if b {
		c.Broadcast()
	}
}

func (c *Switch) Switch() {
	c.L.Lock()
	c.op = !c.op
	c.L.Unlock()
	if c.op {
		c.Broadcast()
	}
}

func (c *Switch) Condition() bool {
	return c.op
}

func (c *Switch) WaitUntilOpen() {
	c.L.Lock()
	for !c.op {
		c.Wait()
	}
	c.L.Unlock()
}
