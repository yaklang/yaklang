package aicommon

// SwapEmitter temporarily replaces the config emitter (e.g. during an isolated sub-loop)
// and returns a restore function that must be deferred.
func (c *Config) SwapEmitter(emitter *Emitter) (restore func()) {
	if c == nil {
		return func() {}
	}
	c.m.Lock()
	prev := c.Emitter
	c.Emitter = emitter
	c.m.Unlock()
	return func() {
		c.m.Lock()
		c.Emitter = prev
		c.m.Unlock()
	}
}
